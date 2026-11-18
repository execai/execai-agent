package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/writelock"
)

// The one place where sync changes a person's files on this computer.
//
// Everything an exchange writes over or removes — a personal memory file, a
// settings file, a file in a bound project's .execai/ — goes through a
// keeper. Right before it replaces a file it reads it again and compares
// it with the hash the decision was taken on (threeway.go): a file that
// changed meanwhile — the model saves memory while an exchange runs — is
// not touched, errChanged says so and the caller leaves that file, and its
// base, for the next exchange. What a file held is kept first, unless the
// new content keeps all of it (the same bytes; for the memory index every
// entry line; for a settings file all but what stays this computer's own,
// configLoses), in one of two places (threeway.go, keepKind):
//
//   - a permanent copy — content the server never saw (here ≠ base), a
//     newer version a return to an earlier one replaced (row 2b), what
//     differs here when there is nothing to compare with: next to a project
//     file as <name>.local-<time>, or in <config>/sync-unsent/<date>/ for
//     memory and settings. The person is told about every one, also when
//     the same copy is there already; sync never removes them.
//   - history — a version the server had (here = base), replaced by a newer
//     one or removed by another computer: in <config>/sync-history
//     (history.go), for 30 days.
//
// Copies of older agents (any *.local-*, *.server-*, the contents of
// sync-conflicts) are never removed by this agent.
//
// A keeper also never writes through a symbolic link or into something that
// is not a plain folder or file, and never replaces what it cannot read.
//
// The order of a replacement: the copy is written and flushed to disk; the
// new content is written into a temporary file next to the target and
// flushed; then, holding the short lock every writer of the agent takes in
// these folders (internal/writelock), the target is read again and compared
// with what the decision was taken on, and only if it is still that, the
// temporary file is renamed over it. A half-written file is never there,
// and a copy is on disk before what it saves is replaced.
//
// The window that remains is between that last comparison and the rename:
// microseconds, and closed for the agent's own writers (the write and edit
// tools, the memory tools, /project memory, the index template, an import),
// which wait for the lock. A program that is not the agent — the person's
// editor, a script — does not take it: if it writes the file in exactly
// that moment, its write is replaced by the version from the server and is
// lost unless it is that version. Its earlier writes are safe: whatever the
// file held at the comparison is what the copy holds, or the replacement
// does not happen.
//
// TestKeeper_OnlyPlaceThatTouchesFiles parses this package and fails on any
// other call that writes, renames or removes a file.

// place — one file sync may change: kind says which area it is in.
type place struct {
	kind string // KindMemory, KindConfig or KindProjectMemory
	root string // the memory folder, the config folder, or a project's .execai
	rel  string // slash path under root
	// scope — for the history: the project's workspace id.
	scope string
}

func (p place) file() string { return filepath.Join(p.root, filepath.FromSlash(p.rel)) }

// expectAny — put/remove without comparing with a decision (sync's own
// writes of content nobody else edits).
const expectAny = "\x00any"

// errChanged — the file changed while the exchange decided about it.
var errChanged = errors.New("sync: the file changed while sync was working on it")

// keeper — see the top of the file.
type keeper struct {
	paths Paths
	now   func() time.Time
	rep   *Report
	// hist — the history folders read in this exchange (history.go).
	hist *historyIndex
}

func (e *Engine) keeper(rep *Report) *keeper {
	if e.hist == nil {
		e.hist = &historyIndex{}
	}
	return &keeper{paths: e.Paths, now: e.now, rep: rep, hist: e.hist}
}

var (
	errLinkedPlace = errors.New("sync: a symbolic link is in the way; sync does not write through links")
	errNotAFolder  = errors.New("sync: not a folder")
	errNotAFile    = errors.New("sync: not a regular file")
)

// checkPlace refuses a place sync must not write to: a link anywhere from
// the root down, a folder that is a file, a file that is something else. The
// config folder itself is where the agent lives and is not checked; a
// memory folder or a .execai/ is.
func checkPlace(pl place) error {
	if pl.kind != KindConfig {
		fi, err := os.Lstat(pl.root)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return nil // made on write, with nothing in it yet
		case err != nil:
			return err
		case fi.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("%s: %w", pl.root, errLinkedPlace)
		case !fi.IsDir():
			return fmt.Errorf("%s: %w", pl.root, errNotAFolder)
		}
	}
	cur := pl.root
	parts := strings.Split(pl.rel, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		last := i == len(parts)-1
		switch {
		case fi.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("%s: %w", cur, errLinkedPlace)
		case !last && !fi.IsDir():
			return fmt.Errorf("%s: %w", cur, errNotAFolder)
		case last && !fi.Mode().IsRegular():
			return fmt.Errorf("%s: %w", cur, errNotAFile)
		}
	}
	return nil
}

// read — what the place holds now; ok=false when there is no file.
func (k *keeper) read(pl place) (data []byte, ok bool, err error) {
	if err := checkPlace(pl); err != nil {
		return nil, false, err
	}
	data, err = os.ReadFile(pl.file())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// current reads the place and checks it against the decision: expect —
// the hash the decision saw, "" for no file, expectAny for no check.
func (k *keeper) current(pl place, expect string) ([]byte, bool, error) {
	old, exists, err := k.read(pl)
	if err != nil {
		return nil, false, err
	}
	if expect != expectAny {
		if (expect == "") == exists || (exists && hashOf(old) != expect) {
			return nil, false, fmt.Errorf("%s: %w", pl.file(), errChanged)
		}
	}
	return old, exists, nil
}

// put makes the file at pl hold data. What it held goes to a copy of the
// kind keep unless nothing of it is lost; why — what the person is told
// about a copy not sent. mtime — the time to give the file (zero leaves the
// time of writing; it never decides anything). join — the settings arrive
// on joining sync (configLoses). Returns whether the file changed.
func (k *keeper) put(pl place, data []byte, mtime time.Time, expect string, keep keepKind, why reason, join bool) (bool, error) {
	old, exists, err := k.current(pl, expect)
	if err != nil {
		return false, err
	}
	if exists && bytes.Equal(old, data) {
		return false, nil
	}
	crashPoint("file")
	var made madeCopy
	if exists && (keep == keepAlways || keep == keepHistoryAlways || !nothingLost(pl, old, data, join)) {
		if made, err = k.keepCopy(pl, old, keep, why); err != nil {
			return false, err
		}
	}
	tmp, err := prepareFile(pl.file(), data)
	if err != nil {
		k.undo(made)
		return false, err
	}
	crashPoint("prepared")
	release := writelock.Hold(k.paths.ConfigDir)
	_, _, err = k.current(pl, expect)
	if err == nil {
		crashPoint("rename")
		err = commitFile(tmp, pl.file())
	}
	release()
	if err != nil {
		_ = os.Remove(tmp)
		k.undo(made)
		return false, err
	}
	k.record(made)
	if !mtime.IsZero() {
		_ = os.Chtimes(pl.file(), mtime, mtime)
	}
	return true, nil
}

// remove deletes the file at pl after keeping a copy of it (keep, why as
// for put).
func (k *keeper) remove(pl place, expect string, keep keepKind, why reason) error {
	old, exists, err := k.current(pl, expect)
	if err != nil || !exists {
		return err
	}
	crashPoint("file")
	made, err := k.keepCopy(pl, old, keep, why)
	if err != nil {
		return err
	}
	release := writelock.Hold(k.paths.ConfigDir)
	_, _, err = k.current(pl, expect)
	if err == nil {
		if err = os.Remove(pl.file()); errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
		syncDir(filepath.Dir(pl.file()))
	}
	release()
	if err != nil {
		k.undo(made)
		return err
	}
	k.record(made)
	return nil
}

// nothingLost reports whether writing new over old keeps everything old
// held: the same bytes; for the memory index every entry line of old; for a
// settings file all of it but what stays this computer's own (configLoses).
func nothingLost(pl place, old, new []byte, join bool) bool {
	switch {
	case bytes.Equal(old, new):
		return true
	case pl.kind == KindMemory && pl.rel == "MEMORY.md":
		return entriesKept(old, new)
	case pl.kind == KindConfig:
		var c struct {
			ModelAuto string `json:"model_auto"`
		}
		auto := ""
		if pl.rel == "config.json" && json.Unmarshal(old, &c) == nil {
			auto = c.ModelAuto
		}
		return !configLoses(pl.rel, old, new, join, auto)
	}
	return false
}

// madeCopy — a copy keepCopy made (or found) for a replacement: recorded in
// the report once the replacement is done, taken back if it is not.
type madeCopy struct {
	kind    keepKind
	pl      place
	why     reason
	path    string
	created bool // written now, not found there already
}

// keepCopy writes one copy of data for pl, of the kind keep, flushed to
// disk; the report learns of it only when the replacement it saves is done
// (record).
func (k *keeper) keepCopy(pl place, data []byte, keep keepKind, why reason) (madeCopy, error) {
	switch keep {
	case keepNothing:
		return madeCopy{}, nil
	case keepAlways:
		keep = keepUnsent
	case keepHistory, keepHistoryAlways:
		p, err := keepHistoryCopy(k.paths, k.hist, k.now(), pl, data)
		m := madeCopy{kind: keepHistory, pl: pl, path: p, created: p != ""}
		if p == "" && err == nil {
			m.path = newestHistory(k.paths, k.hist, pl) // holds this already
		}
		return m, err
	case keepNone:
		// Nothing of value: a placeholder index, an empty file.
		if len(bytes.TrimSpace(data)) == 0 || (pl.kind == KindMemory && pl.rel == "MEMORY.md" && len(indexEntries(data)) == 0) {
			return madeCopy{}, nil
		}
		keep = keepUnsent // what is not known to be on the server stays
	}
	var p string
	var created bool
	var err error
	if pl.kind == KindProjectMemory {
		p, created, err = k.projectCopy(pl.file(), data)
	} else {
		p, created, err = keepUnsentCopy(k.paths.UnsentDir(), k.now().Local(), pl.rel, data)
	}
	return madeCopy{kind: keepUnsent, pl: pl, why: why, path: p, created: created}, err
}

// record tells the report about a copy whose replacement is done: every
// permanent copy, also one that was there already (the person hears of
// each displaced version); a history version is counted only when one was
// written, and named for its file either way.
func (k *keeper) record(m madeCopy) {
	if k.rep == nil || m.path == "" {
		return
	}
	switch m.kind {
	case keepHistory:
		if m.created {
			k.rep.History++
		}
		if k.rep.historyOf == nil {
			k.rep.historyOf = map[string]string{}
		}
		k.rep.historyOf[m.pl.file()] = m.path
	case keepUnsent:
		k.rep.Unsent = append(k.rep.Unsent, unsentCopy{kind: m.pl.kind, root: m.pl.root, path: m.path, why: m.why})
	}
}

// undo takes back a copy written for a replacement that did not happen
// (the file changed meanwhile, or could not be written): what it holds is
// still in the file. A copy that was there before stays.
func (k *keeper) undo(m madeCopy) {
	if m.created && m.path != "" {
		_ = os.Remove(m.path)
		if m.kind == keepHistory {
			k.hist.forget(m.path)
		}
	}
}

// unsentCopy — a copy of content the server never saw.
type unsentCopy struct {
	kind, root, path string
	why              reason
}

// copyStampLayout — the time in the name of a copy next to a project file.
const copyStampLayout = "20060102-150405"

// copyNameRe — <name>.<local|server>-<YYYYMMDD-HHMMSS>: the copies next to
// project files, made by this agent and by the older ones alike.
var copyNameRe = regexp.MustCompile(`^(.+)\.(local|server)-(\d{8}-\d{6})$`)

// projectCopy writes data next to file as <name>.local-<time>. A copy of
// the same file with exactly this content, of any day, is reused (sync
// never removes these, so it does not repeat them); a name taken already
// moves the time a second on, so the name stays one that every agent
// version keeps local.
func (k *keeper) projectCopy(file string, data []byte) (string, bool, error) {
	now := k.now().Local()
	dir, base := filepath.Split(file)
	if same := identicalProjectCopy(dir, base, data); same != "" {
		return same, false, nil
	}
	stamp := now
	for i := 0; i < 3600; i++ {
		target := filepath.Join(dir, base+".local-"+stamp.Format(copyStampLayout))
		if !fileExists(target) {
			if err := writeFileAtomic(target, data); err != nil {
				return "", false, err
			}
			return target, true, nil
		}
		stamp = stamp.Add(time.Second)
	}
	return "", false, fmt.Errorf("sync: no free name for a copy of %s", file)
}

// identicalProjectCopy — a copy of base in dir with exactly data, either
// side, any day.
func identicalProjectCopy(dir, base string, data []byte) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, de := range entries {
		m := copyNameRe.FindStringSubmatch(de.Name())
		if m == nil || m[1] != base || !de.Type().IsRegular() {
			continue
		}
		p := filepath.Join(dir, de.Name())
		if b, err := os.ReadFile(p); err == nil && bytes.Equal(b, data) {
			return p
		}
	}
	return ""
}

// pruneEmptyDirs removes the folders under pd that removing the files rels
// left empty, from the deepest up; pd itself stays (its identity is what
// the base of the project is tied to, bases.go), and so does any other
// empty folder — the person's own, or one standing where a file goes.
func pruneEmptyDirs(pd string, rels []string) {
	for _, rel := range rels {
		for d := path.Dir(rel); d != "." && d != "/"; d = path.Dir(d) {
			if os.Remove(filepath.Join(pd, filepath.FromSlash(d))) != nil {
				break // fails, as it should, on a folder with files
			}
		}
	}
}

// entriesKept — every entry line of the index old (not blank, not inside
// an HTML comment) is in new too.
func entriesKept(old, new []byte) bool {
	have := map[string]bool{}
	for _, l := range indexEntries(new) {
		have[l] = true
	}
	for _, l := range indexEntries(old) {
		if !have[l] {
			return false
		}
	}
	return true
}

// indexEntries — the lines of a memory index that say something: not
// blank, not inside <!-- … --> (the template the chat writes into a new
// memory folder is all comment), trailing blanks cut.
func indexEntries(data []byte) []string {
	var out []string
	in := false
	for _, l := range strings.Split(string(data), "\n") {
		t := strings.TrimRight(l, " \t\r")
		rest := t
		kept := ""
		for rest != "" {
			if in {
				i := strings.Index(rest, "-->")
				if i < 0 {
					rest = ""
					break
				}
				in = false
				rest = rest[i+3:]
				continue
			}
			i := strings.Index(rest, "<!--")
			if i < 0 {
				kept += rest
				break
			}
			kept += rest[:i]
			in = true
			rest = rest[i+4:]
		}
		if strings.TrimSpace(kept) != "" {
			out = append(out, t)
		}
	}
	return out
}
