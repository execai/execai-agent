package inbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	return dir
}

// writeLockFile writes a lock as another process would.
func writeLockFile(t *testing.T, line string) {
	t.Helper()
	p, err := LockPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
}

// liveProcess starts a process that lives until the test ends.
func liveProcess(t *testing.T, script string) int {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	if err := cmd.Start(); err != nil {
		t.Skipf("no sh: %v", err)
	}
	done := make(chan struct{})
	go func() { _, _ = cmd.Process.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
	})
	return cmd.Process.Pid
}

func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Skipf("no true: %v", err)
	}
	return cmd.Process.Pid
}

// A free inbox is taken; the file says who holds it (kind and folder, with
// spaces); release gives it back.
func TestAcquire_FreeTakesAndReleases(t *testing.T) {
	isolate(t)
	release, err := Acquire(KindIDE, "https://api.example", "/home/me/my project")
	if err != nil {
		t.Fatalf("free inbox not taken: %v", err)
	}
	o, _ := ReadOwner()
	if o == nil || o.PID != os.Getpid() || o.Kind != KindIDE || o.Cwd != "/home/me/my project" ||
		o.APIBase != "https://api.example" {
		t.Fatalf("lock says %+v", o)
	}
	p, _ := LockPath()
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}
	release()
	if o, _ := ReadOwner(); o != nil {
		t.Errorf("lock left after release: %+v", o)
	}
}

// A live serve holds the inbox: the window gets LockedError naming it.
func TestAcquire_LiveServeRefuses(t *testing.T) {
	isolate(t)
	pid := liveProcess(t, "sleep 30")
	writeLockFile(t, strconv.Itoa(pid)+" 1780000000 https://x")

	_, err := Acquire(KindIDE, "https://x", "/p")
	var le *LockedError
	if !errors.As(err, &le) {
		t.Fatalf("want LockedError, got %v", err)
	}
	if le.Owner.PID != pid || le.Owner.Kind != KindServe {
		t.Errorf("owner %+v — an old three-field file means serve", le.Owner)
	}
	if o, _ := ReadOwner(); o == nil || o.PID != pid {
		t.Error("the serve's lock was touched")
	}
}

// Another live editor window holds it: kind ide and its folder.
func TestAcquire_OtherWindowRefuses(t *testing.T) {
	isolate(t)
	pid := liveProcess(t, "sleep 30")
	writeLockFile(t, strconv.Itoa(pid)+" 1780000000 https://x ide /work/other repo\n")

	_, err := Acquire(KindServe, "https://x", "")
	var le *LockedError
	if !errors.As(err, &le) || le.Owner.Kind != KindIDE || le.Owner.Cwd != "/work/other repo" {
		t.Fatalf("got %v (%+v)", err, le)
	}
	if !strings.Contains(err.Error(), "/work/other repo") {
		t.Errorf("message does not name the window's folder: %v", err)
	}
}

// The owner was killed (-9, reboot): its file must not block the inbox.
func TestAcquire_DeadOwnerIsReplaced(t *testing.T) {
	isolate(t)
	writeLockFile(t, strconv.Itoa(deadPID(t))+" 1780000000 https://old ide /old")
	release, err := Acquire(KindIDE, "https://x", "/new")
	if err != nil {
		t.Fatalf("stale lock blocked the inbox: %v", err)
	}
	defer release()
	if o, _ := ReadOwner(); o == nil || o.PID != os.Getpid() || o.Cwd != "/new" {
		t.Errorf("lock now %+v", o)
	}
}

// Garbage in the file is "no owner".
func TestAcquire_GarbageIsReplaced(t *testing.T) {
	isolate(t)
	writeLockFile(t, "not a pid at all")
	release, err := Acquire(KindServe, "https://x", "")
	if err != nil {
		t.Fatalf("garbage blocked the inbox: %v", err)
	}
	release()
}

// The race the old code had: check-then-write let two starters both "win".
// Of many concurrent takers exactly one gets the lock now.
func TestAcquire_ExactlyOneWinner(t *testing.T) {
	isolate(t)
	const n = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, locked := 0, 0
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := Acquire(KindIDE, "https://x", "/p"+strconv.Itoa(i))
			mu.Lock()
			defer mu.Unlock()
			var le *LockedError
			switch {
			case err == nil:
				wins++
			case errors.As(err, &le):
				locked++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if wins != 1 || locked != n-1 {
		t.Errorf("wins %d, refused %d — want exactly one owner", wins, locked)
	}
}

// release removes the file only while it is ours: a lock broken and retaken
// by someone else stays.
func TestRelease_KeepsSomeoneElsesLock(t *testing.T) {
	isolate(t)
	release, err := Acquire(KindIDE, "https://x", "/p")
	if err != nil {
		t.Fatal(err)
	}
	pid := liveProcess(t, "sleep 30")
	writeLockFile(t, strconv.Itoa(pid)+" 1780000001 https://x serve -")
	release()
	if o, _ := ReadOwner(); o == nil || o.PID != pid {
		t.Errorf("someone else's lock removed: %+v", o)
	}
}

// "Accept here": the daemon gets SIGTERM, exits, the lock is gone.
func TestStopServe_StopsDaemon(t *testing.T) {
	isolate(t)
	pid := liveProcess(t, "sleep 30")
	writeLockFile(t, strconv.Itoa(pid)+" 1780000000 https://x serve -")
	if err := StopServe(5 * time.Second); err != nil {
		t.Fatalf("StopServe: %v", err)
	}
	if ProcessAlive(pid) {
		t.Error("serve still alive")
	}
	if o, _ := ReadOwner(); o != nil {
		t.Errorf("lock left behind: %+v", o)
	}
}

// Another editor window is never stopped from here: its chats would die.
func TestStopServe_NeverStopsAWindow(t *testing.T) {
	isolate(t)
	pid := liveProcess(t, "sleep 30")
	writeLockFile(t, strconv.Itoa(pid)+" 1780000000 https://x ide /w")
	err := StopServe(time.Second)
	var le *LockedError
	if !errors.As(err, &le) {
		t.Fatalf("want LockedError, got %v", err)
	}
	if !ProcessAlive(pid) {
		t.Error("an editor window was killed")
	}
}

// Nothing to stop is not an error.
func TestStopServe_Nobody(t *testing.T) {
	isolate(t)
	if err := StopServe(time.Second); err != nil {
		t.Errorf("no owner: %v", err)
	}
}

func TestParseOwner_Formats(t *testing.T) {
	cases := []struct {
		in   string
		want *Owner
	}{
		{"", nil},
		{"abc", nil},
		{"42", &Owner{PID: 42, Kind: KindServe}},
		{"42 1780000000 https://a", &Owner{PID: 42, Started: time.Unix(1780000000, 0), APIBase: "https://a", Kind: KindServe}},
		{"42 1780000000 - ide /a b", &Owner{PID: 42, Started: time.Unix(1780000000, 0), Kind: KindIDE, Cwd: "/a b"}},
	}
	for _, c := range cases {
		got := parseOwner(c.in)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("parseOwner(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
	// Round trip, including an empty API base and a folder with spaces.
	o := Owner{PID: 7, Started: time.Unix(1780000000, 0), Kind: KindIDE, Cwd: "/x y/z"}
	if got := parseOwner(formatOwner(o)); got == nil || *got != o {
		t.Errorf("round trip %+v → %+v", o, got)
	}
}

// The lock lives where older execai serve binaries look for it.
func TestLockPath_IsServePid(t *testing.T) {
	dir := isolate(t)
	p, err := LockPath()
	if err != nil || p != filepath.Join(dir, "execai", "serve.pid") {
		t.Errorf("LockPath = %q, %v", p, err)
	}
}
