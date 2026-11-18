package uiwindow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

// One execai ui per project folder: the second one finds the first through
// the lock and reads where it listens.
func TestInstance_SecondFindsFirst(t *testing.T) {
	cfg := t.TempDir()
	proj := t.TempDir()

	first, held, err := Acquire(cfg, proj)
	if err != nil || !held {
		t.Fatalf("first: held=%v err=%v", held, err)
	}
	st := State{Cwd: proj, Port: 23751, PID: os.Getpid(), Mode: ModeWindow, Started: "2026-01-02T03:04:05Z",
		Browser: &Browser{ID: "chrome", Name: "Google Chrome", Path: "/usr/bin/google-chrome"},
		Profile: filepath.Join(cfg, "ui-window", "chrome-x"), Control: "secret", WindowUp: true}
	if err := first.Write(st); err != nil {
		t.Fatal(err)
	}

	second, held, err := Acquire(cfg, proj)
	if err != nil {
		t.Fatal(err)
	}
	if held {
		t.Fatal("the second run took the folder while the first one holds it")
	}
	got, err := second.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != 23751 || got.PID != os.Getpid() || got.Mode != ModeWindow || got.Browser == nil ||
		got.Browser.Path != "/usr/bin/google-chrome" || got.Profile != st.Profile || got.Control != "secret" || !got.WindowUp {
		t.Fatalf("state %+v", got)
	}
	second.Release() // releasing what is not held changes nothing
	if got, _ := first.Read(); got.PID != os.Getpid() || got.Control == "" {
		t.Fatalf("the second run's release touched the first one's state: %+v", got)
	}

	// The state file is private — it carries the control secret — and so
	// is the folder it lies in.
	fi, err := os.Stat(first.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("state file mode %v", fi.Mode().Perm())
	}
	if di, err := os.Stat(filepath.Dir(first.StateFile())); err != nil || di.Mode().Perm() != 0o700 {
		t.Errorf("the folder of the state files: %v %v", di.Mode().Perm(), err)
	}

	// Another folder is another instance.
	other, held, err := Acquire(cfg, t.TempDir())
	if err != nil || !held {
		t.Fatalf("another folder: held=%v err=%v", held, err)
	}
	if other.StateFile() == first.StateFile() {
		t.Fatal("two folders share a state file")
	}
	other.Release()

	// The first one stops: what the file says about the process is gone
	// with it — read as it lies on the disk, before anybody takes the
	// folder again.
	first.Release()
	raw, err := os.ReadFile(first.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	var left State
	if err := json.Unmarshal(raw, &left); err != nil {
		t.Fatal(err)
	}
	if left.PID != 0 || left.Control != "" || left.Mode != "" || left.Browser != nil || left.Profile != "" ||
		left.WindowUp || left.Started != "" || left.Port != 23751 || left.Cwd != proj {
		t.Fatalf("the state file after a clean stop: %s", raw)
	}
	// …but for when it last ran: the sweep of folders that are gone goes by
	// that date.
	if left.LastRun != "2026-01-02T03:04:05Z" {
		t.Fatalf("the date of the last run after a clean stop: %s", raw)
	}
	third, held, err := Acquire(cfg, proj)
	if err != nil || !held {
		t.Fatalf("after the first one stopped: held=%v err=%v", held, err)
	}
	defer third.Release()
	got, _ = third.Read()
	if got.Port != 23751 || got.PID != 0 || got.Control != "" {
		t.Fatalf("state after a clean stop %+v", got)
	}
}

// A state file left by a killed process (its pid and code still in it) is
// not in the way: nobody holds the lock.
func TestInstance_StaleState(t *testing.T) {
	cfg := t.TempDir()
	proj := t.TempDir()
	probe, held, err := Acquire(cfg, proj)
	if err != nil || !held {
		t.Fatal(err)
	}
	file := probe.StateFile()
	probe.Release()

	stale, _ := json.Marshal(State{Cwd: proj, Port: 23753, PID: 1 << 30, Mode: ModeWindow, Control: "dead", WindowUp: true})
	if err := os.WriteFile(file, stale, 0o600); err != nil {
		t.Fatal(err)
	}
	in, held, err := Acquire(cfg, proj)
	if err != nil || !held {
		t.Fatalf("a stale state file blocks the folder: held=%v err=%v", held, err)
	}
	defer in.Release()
	got, _ := in.Read()
	if got.Port != 23753 {
		t.Fatalf("the port of the last run is not remembered: %+v", got)
	}
	// What the dead process left about itself is dropped the moment the
	// folder is taken: whoever reads the state while the new holder is
	// still starting must not take the old pid, code or control secret for
	// the holder's.
	if got.PID != 0 || got.Control != "" || got.Mode != "" || got.WindowUp {
		t.Fatalf("the state of a killed process outlived it: %+v", got)
	}
	// Garbage in the file is no state at all.
	if err := os.WriteFile(file, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := in.Read(); err != nil || got.Port != 0 {
		t.Fatalf("a broken state file: %+v %v", got, err)
	}
}

// The same folder by another spelling is the same instance.
func TestKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := Key(filepath.Join(dir, "p"))
	if b := Key(filepath.Join(dir, "p") + string(filepath.Separator)); a != b {
		t.Error("a trailing separator changes the key")
	}
	if b := Key(filepath.Join(dir, "x", "..", "p")); a != b {
		t.Error("an unclean path changes the key")
	}
	if err := os.Symlink(filepath.Join(dir, "p"), filepath.Join(dir, "link")); err == nil {
		if b := Key(filepath.Join(dir, "link")); a != b {
			t.Error("a link to the folder is another key")
		}
	}
	if b := Key(dir); a == b {
		t.Error("two folders, one key")
	}
	if len(a) != 16 {
		t.Errorf("key %q", a)
	}
}

// Every folder has a port of its own in a small range, worked out from the
// folder's key: the same from run to run, whatever else ran in between. A
// taken one gives way to the next ones of the range.
func TestPorts(t *testing.T) {
	seen := map[int]bool{}
	for i := 0; i < 200; i++ {
		key := Key(filepath.Join("/projects", strconv.Itoa(i)))
		got := Ports(key)
		if len(got) != PortCount {
			t.Fatalf("%s: %v", key, got)
		}
		all := map[int]bool{}
		for _, p := range got {
			if p < PortBase || p >= PortBase+PortCount || all[p] {
				t.Fatalf("%s: %v is not the range, each port once", key, got)
			}
			all[p] = true
		}
		if again := Ports(key); !slices.Equal(got, again) {
			t.Fatalf("%s: %v, then %v", key, got, again)
		}
		// After its own port the folder goes on up the range and round.
		for j := 1; j < len(got); j++ {
			if want := PortBase + (got[0]-PortBase+j)%PortCount; got[j] != want {
				t.Fatalf("%s: %v", key, got)
			}
		}
		seen[got[0]] = true
	}
	if len(seen) < PortCount*3/4 {
		t.Errorf("200 folders share %d first ports of %d", len(seen), PortCount)
	}
	// A key that is not one starts at the bottom of the range.
	if got := Ports(""); got[0] != PortBase || len(got) != PortCount {
		t.Fatalf("%v", got)
	}
	// The range is below the ports systems hand out to outgoing connections.
	if PortBase+PortCount > 32768 || PortBase < 1024 {
		t.Fatalf("range %d..%d", PortBase, PortBase+PortCount-1)
	}
}

// The folder of the last run, for a launcher shortcut that names none.
func TestLastDir(t *testing.T) {
	cfg := t.TempDir()
	if got := LastDir(cfg); got != "" {
		t.Fatalf("no last folder yet: %q", got)
	}
	proj := t.TempDir()
	if err := SaveLastDir(cfg, proj); err != nil {
		t.Fatal(err)
	}
	if got := LastDir(cfg); got != proj {
		t.Fatalf("%q", got)
	}
	// A folder that is gone is not offered.
	if err := os.Remove(proj); err != nil {
		t.Fatal(err)
	}
	if got := LastDir(cfg); got != "" {
		t.Fatalf("a removed folder: %q", got)
	}
}

// Retry — the look of a second execai ui at the folder turns into a hold
// once the first one has ended: how `execai ui --stop` knows the panel is
// gone, whatever its pid was.
func TestInstance_Retry(t *testing.T) {
	cfg, proj := t.TempDir(), t.TempDir()
	first, held, err := Acquire(cfg, proj)
	if err != nil || !held {
		t.Fatal(err)
	}
	if err := first.Write(State{Cwd: proj, Port: 23751, PID: os.Getpid(), Control: "secret"}); err != nil {
		t.Fatal(err)
	}
	second, held, err := Acquire(cfg, proj)
	if err != nil || held {
		t.Fatalf("held=%v err=%v", held, err)
	}
	defer second.Release()
	if second.Retry() {
		t.Fatal("took the folder from under its holder")
	}
	if st, _ := first.Read(); st.Control != "secret" {
		t.Fatalf("a failed retry touched the state: %+v", st)
	}
	first.Release()
	if !second.Retry() {
		t.Fatal("the folder is free and was not taken")
	}
	if !second.Retry() {
		t.Fatal("a held folder is held")
	}
	third, held, _ := Acquire(cfg, proj)
	if held {
		t.Fatal("two holders")
	}
	third.Release()
}

// Keys — every folder a panel was ever started for on this computer, for
// `execai ui --stop --all`.
func TestInstance_Keys(t *testing.T) {
	cfg := t.TempDir()
	if got := Keys(cfg); len(got) != 0 {
		t.Fatalf("no panels yet: %q", got)
	}
	a, b := t.TempDir(), t.TempDir()
	ia, _, err := Acquire(cfg, a)
	if err != nil {
		t.Fatal(err)
	}
	defer ia.Release()
	ib, _, err := Acquire(cfg, b)
	if err != nil {
		t.Fatal(err)
	}
	ib.Release()
	if err := SaveLastDir(cfg, a); err != nil { // last.json is not an instance
		t.Fatal(err)
	}
	// Neither is a lock file of some other name.
	for _, name := range []string{"notakey.lock", "0123.lock", "0123456789ABCDEF.lock", "0123456789abcdef0.lock", "0123456789abcdeg.lock"} {
		if err := os.WriteFile(filepath.Join(cfg, "ui", name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := Keys(cfg)
	want := []string{Key(a), Key(b)}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("keys %q, want %q", got, want)
	}
	// By its key the folder of a running panel is seen as taken, a stopped
	// one as free.
	in, held, err := AcquireKey(cfg, Key(a))
	if err != nil || held {
		t.Fatalf("running: held=%v err=%v", held, err)
	}
	in.Release()
	in, held, err = AcquireKey(cfg, Key(b))
	if err != nil || !held {
		t.Fatalf("stopped: held=%v err=%v", held, err)
	}
	in.Release()
	if _, _, err := AcquireKey(cfg, "../escape"); err == nil {
		t.Fatal("a key that is not a key")
	}
}

// Sweep removes what is left of folders that are gone and were last used
// long ago: their state files and window profiles. "Long ago" is the date of
// the last run, kept in the state file — not the age of the file, which
// every look at the folder used to make young again. A folder that exists,
// a folder that vanished only lately (a disk that is unplugged today), a
// panel that runs and anything that is not ours stay.
func TestSweep(t *testing.T) {
	cfg, home := t.TempDir(), t.TempDir()
	long := time.Now().Add(-2 * sweepAge)
	lately := time.Now().Add(-24 * time.Hour)
	profiles := map[string][]string{}
	// ran — a panel ran in dir at the given time and stopped (or, held, is
	// still there).
	ran := func(dir string, when time.Time, held bool) *Instance {
		t.Helper()
		in, _, err := Acquire(cfg, dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := in.Write(State{Cwd: dir, Port: PortBase, PID: os.Getpid(), Started: when.UTC().Format(time.RFC3339),
			Mode: ModeWindow, Control: "secret"}); err != nil {
			t.Fatal(err)
		}
		key := in.Key()
		for _, p := range []string{
			ProfileDir(cfg, home, Browser{ID: "chrome"}, key, "linux"),
			ProfileDir(cfg, home, Browser{ID: "edge"}, key, "linux"),
			ProfileDir(cfg, home, Browser{ID: "chromium", Kind: KindSnap, Sandbox: "chromium"}, key, "linux"),
			ProfileDir(cfg, home, Browser{ID: "chrome", Kind: KindFlatpak, Sandbox: "com.google.Chrome"}, key, "linux"),
		} {
			if err := os.MkdirAll(filepath.Join(p, "Default"), 0o700); err != nil {
				t.Fatal(err)
			}
			profiles[dir] = append(profiles[dir], p)
		}
		if !held {
			in.Release()
		}
		return in
	}
	mk := func(name string) string {
		t.Helper()
		d := filepath.Join(t.TempDir(), name)
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	gone, goneLately, kept, running := mk("gone"), mk("gone-lately"), mk("kept"), mk("running-then-gone")
	ran(gone, long, false)
	ran(goneLately, lately, false)
	ran(kept, long, false)
	defer ran(running, long, true).Release()
	// Files written before the date was kept inside: their age is the
	// file's.
	oldForm, oldFormFresh := mk("old-form"), mk("old-form-fresh")
	for dir, mtime := range map[string]time.Time{oldForm: long, oldFormFresh: lately} {
		file := filepath.Join(cfg, "ui", Key(dir)+".json")
		data, _ := json.Marshal(map[string]any{"cwd": dir, "port": PortBase})
		if err := os.WriteFile(file, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg, "ui", Key(dir)+".lock"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(file, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	stranger := filepath.Join(cfg, "ui-window", "not-a-profile")
	if err := os.MkdirAll(stranger, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{gone, goneLately, running, oldForm, oldFormFresh} {
		if err := os.Remove(d); err != nil {
			t.Fatal(err)
		}
	}

	// Looks at the folders in between — other commands take each lock for a
	// moment — change nothing about their age.
	for i := 0; i < 3; i++ {
		for _, key := range Keys(cfg) {
			if in, _, err := AcquireKey(cfg, key); err == nil {
				in.Release()
			}
		}
	}
	if fi, err := os.Stat(filepath.Join(cfg, "ui", Key(oldFormFresh)+".json")); err != nil || time.Since(fi.ModTime()) < 23*time.Hour {
		t.Fatalf("a look at an idle folder made its state file young: %v %v", fi.ModTime(), err)
	}

	Sweep(cfg, home)

	exists := func(p string) bool { _, err := os.Lstat(p); return err == nil }
	state := func(dir string) string { return filepath.Join(cfg, "ui", Key(dir)+".json") }
	for _, p := range profiles[gone] {
		if exists(p) {
			t.Errorf("a profile of a folder that is gone stays: %s", p)
		}
	}
	for _, dir := range []string{gone, oldForm} {
		if exists(state(dir)) {
			t.Errorf("the state file of %s, gone for long, stays", filepath.Base(dir))
		}
	}
	for _, dir := range []string{kept, goneLately, running} {
		for _, p := range profiles[dir] {
			if !exists(p) {
				t.Errorf("a profile of %s was removed: %s", filepath.Base(dir), p)
			}
		}
	}
	for _, dir := range []string{kept, goneLately, running, oldFormFresh} {
		if !exists(state(dir)) {
			t.Errorf("the state file of %s was removed", filepath.Base(dir))
		}
	}
	if !exists(stranger) {
		t.Error("a folder that is no profile of ours was removed")
	}

	// The date of the last run survives a run that is cut short too: a
	// panel that was killed leaves its start in the file, and the next one
	// to take the folder keeps it.
	killed := mk("killed")
	in := ran(killed, long, true)
	unlock(in.lock) // the process is gone: the kernel drops its lock
	next, held, err := Acquire(cfg, killed)
	if err != nil || !held {
		t.Fatalf("held=%v err=%v", held, err)
	}
	if st, _ := next.Read(); st.LastRun != long.UTC().Format(time.RFC3339) || st.PID != 0 {
		t.Errorf("the state after a killed panel: %+v", st)
	}
	next.Release()
}
