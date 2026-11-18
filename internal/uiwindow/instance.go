package uiwindow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// One execai ui per project folder. The running one holds an OS file lock
// on <config>/ui/<key>.lock (flock on Unix, LockFileEx on Windows) and keeps
// <config>/ui/<key>.json next to it: where it listens, its pid, what shows
// the page. A second execai ui for the same folder finds the lock taken,
// reads the state and shows the window that is already there.
//
// The lock, not the pid in the file, says whether the first one is alive:
// the kernel drops it when the process ends, however it ends, so a state
// file left by a killed process is never in the way, and a pid reused by
// another program cannot pass for a running panel. After a clean stop the
// file stays with the port alone, so the next run of the project asks for
// the same address.
//
// Both files are 0600 in the 0700 config directory, like credentials.json:
// the state carries the one-time sign-in code. They are this computer's
// only — sync takes the named settings files and the memory folder, not
// this one.

const (
	// PortBase and PortCount — the ports execai ui takes when none is asked
	// for: every folder has its own in this range.
	//
	// 23750–23769 lies in a block the IANA registry leaves unassigned
	// (23547–23999, TCP and UDP), below the ranges systems hand out to
	// outgoing connections (Linux 32768–60999, Windows and macOS
	// 49152–65535) and below the Kubernetes NodePort range (30000–32767),
	// and is not a customary port of development tools (3000, 5173, 8000,
	// 8080, 8765, 9000…).
	PortBase  = 23750
	PortCount = 20
)

// Ports — the ports a run without --port tries for the folder with this
// key, in order: the folder's own — worked out from the key, so it is the
// same on every run, whatever ran before — then on up the range and round.
// (Remembering the last port instead made the address drift: one clash
// moved a folder for good.)
func Ports(key string) []int {
	first := 0
	if len(key) >= 8 {
		if n, err := strconv.ParseUint(key[:8], 16, 32); err == nil {
			first = int(n % PortCount)
		}
	}
	out := make([]int, PortCount)
	for i := range out {
		out[i] = PortBase + (first+i)%PortCount
	}
	return out
}

// What shows the page of a running execai ui.
const (
	ModeWindow = "window" // an app window of State.Browser with State.Profile
	ModeTab    = "tab"    // a tab of the default browser
	ModeNone   = "none"   // nothing was opened (--no-open)
)

// State — the state file of one project's execai ui.
type State struct {
	Cwd  string `json:"cwd"`
	Port int    `json:"port"`
	// LastRun — when a panel last started for the folder (RFC 3339), kept
	// after it stopped: what the sweep of folders that are gone goes by.
	LastRun string `json:"last_run,omitempty"`
	// PID, Started, Mode, Browser, Profile, Code — of the running process;
	// cleared when it stops.
	PID     int      `json:"pid,omitempty"`
	Started string   `json:"started,omitempty"`
	Mode    string   `json:"mode,omitempty"`
	Browser *Browser `json:"browser,omitempty"`
	Profile string   `json:"profile,omitempty"`
	// WindowUp — the window has fetched the page. Until then a second run
	// leaves the browser alone: two starts of one profile at the same
	// moment race for it.
	WindowUp bool `json:"window_up,omitempty"`
	// Control — the secret the running panel takes its orders with:
	// execai ui --stop and --code send it to the panel's own address.
	Control string `json:"control,omitempty"`
}

// Key — the name of a project folder's instance: the same for every
// spelling of the folder.
func Key(cwd string) string {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = filepath.Clean(cwd)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:8])
}

// Instance — this process's hold (or look) on a project folder.
type Instance struct {
	key   string
	state string
	lock  *os.File
	held  bool
}

// Acquire takes the folder for this process. held=false: another execai ui
// has it — Read tells where. A file system that cannot lock does not stop
// the panel: the folder counts as taken by us, as before there was a lock.
func Acquire(configDir, cwd string) (in *Instance, held bool, err error) {
	return AcquireKey(configDir, Key(cwd))
}

// AcquireKey — Acquire for a folder known by its key (Keys).
func AcquireKey(configDir, key string) (in *Instance, held bool, err error) {
	if !isKey(key) {
		return nil, false, fmt.Errorf("not an instance key: %q", key)
	}
	dir := filepath.Join(configDir, "ui")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(filepath.Join(dir, key+".lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, err
	}
	in = &Instance{key: key, state: filepath.Join(dir, key+".json"), lock: f}
	ok, lerr := tryLock(f)
	if ok {
		in.dropStale()
	}
	in.held = ok || lerr != nil
	return in, in.held, nil
}

// isKey — 16 hex digits, what Key makes.
func isKey(key string) bool {
	if len(key) != 16 {
		return false
	}
	for _, r := range key {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// Keys — the keys of all folders a panel was started for on this computer
// (running or not), sorted.
func Keys(configDir string) []string {
	entries, err := os.ReadDir(filepath.Join(configDir, "ui"))
	if err != nil {
		return nil
	}
	var keys []string
	for _, e := range entries {
		if key, ok := strings.CutSuffix(e.Name(), ".lock"); ok && isKey(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// dropStale — the lock was just taken: whatever the state file says about a
// process is about one that is gone (a killed panel leaves its pid, code and
// control secret behind). It is cut down to the folder and the port at
// once, so that nobody who reads the state while this holder is still
// starting takes the dead process's details for the holder's.
func (in *Instance) dropStale() {
	if st, err := in.Read(); err == nil && st.live() {
		_ = in.Write(st.rest())
	}
}

// live — the state says something about a process.
func (st State) live() bool {
	return st.PID != 0 || st.Started != "" || st.Mode != "" || st.Browser != nil ||
		st.Profile != "" || st.WindowUp || st.Control != ""
}

// rest — the state of a folder nobody runs a panel for: where it was and
// when it last ran.
func (st State) rest() State {
	last := st.Started
	if last == "" {
		last = st.LastRun
	}
	return State{Cwd: st.Cwd, Port: st.Port, LastRun: last}
}

// Retry tries to take a folder that another execai ui held when Acquire
// looked: true once this process holds it — the other one has ended. This,
// and nothing the state file says, is how the end of a panel is known: the
// kernel drops the lock with the process, on every system, and a pid proves
// nothing.
func (in *Instance) Retry() bool {
	if in.held {
		return true
	}
	if in.lock == nil {
		return false
	}
	if ok, err := tryLock(in.lock); err == nil && ok {
		in.dropStale()
		in.held = true
	}
	return in.held
}

// Key — the instance key (part of the window profile's name).
func (in *Instance) Key() string { return in.key }

// StateFile — the path of the state file.
func (in *Instance) StateFile() string { return in.state }

// Read returns the state; a missing or broken file is the empty state.
func (in *Instance) Read() (State, error) {
	var st State
	data, err := os.ReadFile(in.state)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if json.Unmarshal(data, &st) != nil {
		return State{}, nil
	}
	return st, nil
}

// Write replaces the state file: whole or as it was, never half.
func (in *Instance) Write(st State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(in.state, data, 0o600)
}

// Release ends the hold: the state keeps the folder and the port for the
// next run, the lock is dropped. For an Instance that never held the folder
// it only closes the file.
func (in *Instance) Release() {
	if in == nil || in.lock == nil {
		return
	}
	if in.held {
		// Only what a process left is rewritten. A file that says nothing
		// of a process is left as it lies: a look at an idle folder by
		// another command must not make its state file new.
		if st, err := in.Read(); err == nil && st.live() {
			_ = in.Write(st.rest())
		}
		unlock(in.lock)
		in.held = false
	}
	_ = in.lock.Close()
	in.lock = nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp, mode)
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		_ = os.Remove(tmp)
	}
	return werr
}

// lastFile — the folder of the last execai ui run, for a launcher shortcut
// that names none.
func lastFile(configDir string) string { return filepath.Join(configDir, "ui", "last.json") }

// SaveLastDir remembers the folder execai ui runs in.
func SaveLastDir(configDir, cwd string) error {
	if err := os.MkdirAll(filepath.Join(configDir, "ui"), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(struct {
		Cwd string `json:"cwd"`
	}{cwd})
	if err != nil {
		return err
	}
	return writeAtomic(lastFile(configDir), data, 0o600)
}

// LastDir — the folder of the last run, "" when there was none or it is
// gone.
func LastDir(configDir string) string {
	data, err := os.ReadFile(lastFile(configDir))
	if err != nil {
		return ""
	}
	var v struct {
		Cwd string `json:"cwd"`
	}
	if json.Unmarshal(data, &v) != nil || v.Cwd == "" {
		return ""
	}
	if st, err := os.Stat(v.Cwd); err != nil || !st.IsDir() {
		return ""
	}
	return v.Cwd
}

// sweepAge — how long a folder must have been unused before what is left of
// it is removed once it is gone: a project on a disk that is only unplugged
// today keeps its window.
const sweepAge = 30 * 24 * time.Hour

// Sweep removes what panels left for folders that no longer exist and were
// last used long ago: the state file and the window profiles (a profile is
// some megabytes per folder). The lock files stay — empty, and removing a
// lock file is how two processes come to hold "the same" lock.
func Sweep(configDir, home string) {
	for _, key := range Keys(configDir) {
		in, held, err := AcquireKey(configDir, key)
		if err != nil {
			continue
		}
		if held && in.abandoned() {
			_ = os.Remove(in.state)
			for _, p := range profilesOf(configDir, home, key) {
				_ = os.RemoveAll(p)
			}
		}
		in.Release()
	}
}

// abandoned — the folder of this instance is gone and no panel has run for
// it for sweepAge. The age is the date of the last run, kept in the state
// file; a file from before that date was kept there is as old as the file
// itself.
func (in *Instance) abandoned() bool {
	st, err := in.Read()
	if err != nil || st.Cwd == "" || st.live() {
		return false
	}
	last, err := time.Parse(time.RFC3339, st.LastRun)
	if err != nil {
		fi, serr := os.Stat(in.state)
		if serr != nil {
			return false
		}
		last = fi.ModTime()
	}
	if time.Since(last) < sweepAge {
		return false
	}
	_, err = os.Stat(st.Cwd)
	return os.IsNotExist(err)
}

// profilesOf — the window profiles of a folder, of every browser: the ones
// in the config directory and the ones in the folders of sandboxed
// browsers (ProfileDir).
func profilesOf(configDir, home, key string) []string {
	var out []string
	if entries, err := os.ReadDir(filepath.Join(configDir, "ui-window")); err == nil {
		for _, e := range entries {
			if e.IsDir() && strings.HasSuffix(e.Name(), "-"+key) {
				out = append(out, filepath.Join(configDir, "ui-window", e.Name()))
			}
		}
	}
	for _, sandbox := range []struct{ root, rest string }{
		{filepath.Join(home, "snap"), filepath.Join("common", "execai-ui-window", key)},
		{filepath.Join(home, ".var", "app"), filepath.Join("execai-ui-window", key)},
	} {
		entries, err := os.ReadDir(sandbox.root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			p := filepath.Join(sandbox.root, e.Name(), sandbox.rest)
			if fi, err := os.Lstat(p); err == nil && fi.IsDir() {
				out = append(out, p)
			}
		}
	}
	return out
}
