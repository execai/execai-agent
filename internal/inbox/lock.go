package inbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// The inbox has exactly one owner per machine: either the background
// `execai serve` or one editor window (`execai ide`). Two owners would not
// corrupt anything — ack pins a task to one of them — but they would split the
// stream of tasks at random, and the person would not know why the output
// showed up where they are not looking. Cheaper to keep the second one out.
//
// The lock is a pid file, ~/.config/execai/serve.pid (the name predates the
// editor owner and is kept so that older `execai serve` binaries respect it):
//
//	<pid> <unix start time> <api base> <kind> <cwd>
//
// kind is "serve" or "ide"; cwd — the directory an editor owner works in (it
// may contain spaces, so it goes last). Older files carry only the first
// three fields and mean "serve".
//
// Liveness is checked with signal 0, not by the file alone: the owner may
// have been killed with -9 or the machine rebooted, and a leftover file must
// not block the inbox forever.
//
// The file is created atomically: the content is written to a temporary file
// and hard-linked into place, which fails if the lock already exists. Two
// processes starting at once can no longer both "win" by overwriting each
// other's file, and a reader never sees a half-written one.

// Owner kinds.
const (
	KindServe = "serve"
	KindIDE   = "ide"
)

// Owner is what the lock file says about the inbox owner.
type Owner struct {
	PID     int
	Started time.Time
	APIBase string
	Kind    string // KindServe or KindIDE
	Cwd     string // the editor owner's directory; empty for serve
}

// LockedError — the inbox is already owned by a live process.
type LockedError struct {
	Owner Owner
}

func (e *LockedError) Error() string {
	if e.Owner.Kind == KindIDE {
		return i18n.Tf("inbox.lock.heldByIDE", e.Owner.PID, e.Owner.Cwd)
	}
	return i18n.Tf("inbox.lock.heldByServe", e.Owner.PID)
}

// LockPath is where the lock lives.
func LockPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "execai")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "serve.pid"), nil
}

// ReadOwner reads the lock file. No file is not an error but "no owner";
// garbage in it reads as "no owner" too.
func ReadOwner() (*Owner, error) {
	p, err := LockPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseOwner(string(data)), nil
}

func parseOwner(s string) *Owner {
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(s), "\n", 2)[0])
	f := strings.SplitN(line, " ", 5)
	if len(f) < 1 {
		return nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(f[0]))
	if err != nil {
		return nil
	}
	o := &Owner{PID: pid, Kind: KindServe}
	if len(f) > 1 {
		if sec, err := strconv.ParseInt(f[1], 10, 64); err == nil {
			o.Started = time.Unix(sec, 0)
		}
	}
	if len(f) > 2 && f[2] != "-" {
		o.APIBase = f[2]
	}
	if len(f) > 3 && f[3] == KindIDE {
		o.Kind = KindIDE
	}
	if len(f) > 4 {
		o.Cwd = f[4]
	}
	return o
}

func formatOwner(o Owner) string {
	base := o.APIBase
	if base == "" || strings.ContainsAny(base, " \n") {
		base = "-"
	}
	kind := o.Kind
	if kind != KindIDE {
		kind = KindServe
	}
	cwd := strings.ReplaceAll(o.Cwd, "\n", " ")
	return fmt.Sprintf("%d %d %s %s %s\n", o.PID, o.Started.Unix(), base, kind, cwd)
}

// WriteOwner overwrites the lock file with this process as the owner,
// without checking who held it. Acquire is the way to take the lock; this is
// for callers that already know the file is theirs (and for tests).
func WriteOwner(kind, apiBase, cwd string) error {
	p, err := LockPath()
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", p, os.Getpid())
	o := Owner{PID: os.Getpid(), Started: time.Now(), APIBase: apiBase, Kind: kind, Cwd: cwd}
	if err := os.WriteFile(tmp, []byte(formatOwner(o)), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// RemoveLock deletes the lock file unconditionally — for a caller that has
// just seen its owner die (Stop).
func RemoveLock() {
	if p, err := LockPath(); err == nil {
		_ = os.Remove(p)
	}
}

// ProcessAlive reports whether a process with this pid exists. On Unix
// os.FindProcess always succeeds, so signal 0 decides; on Windows FindProcess
// itself fails for a dead process.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// CurrentOwner returns the live owner of the inbox, nil when there is none
// (no file, garbage, or the process is gone).
func CurrentOwner() *Owner {
	o, err := ReadOwner()
	if err != nil || o == nil || !ProcessAlive(o.PID) {
		return nil
	}
	return o
}

// Acquire takes the inbox for this process. A live owner gives *LockedError;
// a stale file (its process is gone) is replaced. The returned release
// removes the file — only while it is still ours.
func Acquire(kind, apiBase, cwd string) (release func(), err error) {
	p, err := LockPath()
	if err != nil {
		return nil, err
	}
	me := Owner{PID: os.Getpid(), Started: time.Now(), APIBase: apiBase, Kind: kind, Cwd: cwd}
	content := formatOwner(me)
	for attempt := 0; attempt < 5; attempt++ {
		err := createExclusive(p, content)
		if err == nil {
			return func() { releaseIfOurs(p, me) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		cur, rerr := ReadOwner()
		if rerr != nil {
			return nil, rerr
		}
		if cur != nil && ProcessAlive(cur.PID) {
			return nil, &LockedError{Owner: *cur}
		}
		// A leftover of a killed process (or garbage): normal after kill -9 or
		// a reboot. Move it away and try again.
		breakStale(p, cur)
	}
	return nil, fmt.Errorf("%s: %s", i18n.T("inbox.lock.busy"), p)
}

// createExclusive makes path with content only if it does not exist yet.
// The content goes to a temporary file first and is hard-linked into place,
// so the lock is never visible half-written. Where hard links are not
// supported, O_EXCL is the fallback.
func createExclusive(path, content string) error {
	tmp := fmt.Sprintf("%s.%d.%d.tmp", path, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	err := os.Link(tmp, path)
	if err == nil || errors.Is(err, fs.ErrExist) {
		return err
	}
	f, ferr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if ferr != nil {
		return ferr
	}
	_, werr := f.WriteString(content)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// breakStale moves a stale lock out of the way. Rename is atomic, so of two
// processes breaking the same stale file only one gets it; if what we moved
// turns out to be a fresh lock taken meanwhile (its pid differs from the stale
// one we saw), it is put back.
func breakStale(path string, stale *Owner) {
	aside := fmt.Sprintf("%s.%d.%d.stale", path, os.Getpid(), time.Now().UnixNano())
	if err := os.Rename(path, aside); err != nil {
		return // someone else moved it first
	}
	data, _ := os.ReadFile(aside)
	moved := parseOwner(string(data))
	samePID := (moved == nil && stale == nil) || (moved != nil && stale != nil && moved.PID == stale.PID)
	if !samePID && moved != nil && ProcessAlive(moved.PID) {
		// A live owner's fresh lock: put it back where it belongs (unless the
		// path has been taken again — then the newer lock stands).
		if err := os.Link(aside, path); err != nil && !errors.Is(err, fs.ErrExist) {
			_ = os.Rename(aside, path)
			return
		}
	}
	_ = os.Remove(aside)
}

// releaseIfOurs removes the lock only while it still names this process: a
// lock broken and retaken by someone else must not be deleted from under them.
func releaseIfOurs(path string, me Owner) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	cur := parseOwner(string(data))
	if cur == nil || cur.PID != me.PID || cur.Started.Unix() != me.Started.Unix() {
		return
	}
	_ = os.Remove(path)
}

// WaitGone waits for a process to exit; false when it is still alive after d.
func WaitGone(pid int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if !ProcessAlive(pid) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !ProcessAlive(pid)
}

// StopServe asks the background `execai serve` that owns the inbox to exit
// (SIGTERM) and waits up to wait for it — the "Accept here" button of the
// editor panel. It never touches an editor owner: closing another window's
// agent from here would kill that window's chats. A task the daemon is
// running gets cancelled and reports an error to the web chat itself.
func StopServe(wait time.Duration) error {
	o := CurrentOwner()
	if o == nil {
		return nil // nobody to stop
	}
	if o.Kind != KindServe {
		return &LockedError{Owner: *o}
	}
	p, err := os.FindProcess(o.PID)
	if err != nil {
		return err
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return errors.New(i18n.Tf("inbox.lock.signalFailed", o.PID, err))
	}
	if !WaitGone(o.PID, wait) {
		return errors.New(i18n.Tf("inbox.lock.stopTimeout", o.PID, wait))
	}
	// The daemon removes its file on a clean exit; after a crash it is ours
	// to clean up.
	if cur, _ := ReadOwner(); cur != nil && cur.PID == o.PID {
		RemoveLock()
	}
	return nil
}
