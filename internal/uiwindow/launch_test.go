package uiwindow

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestArgs(t *testing.T) {
	b := Browser{ID: "chrome", Name: "Google Chrome", Path: "/usr/bin/google-chrome"}
	const url = "http://127.0.0.1:23750/?t=abc"
	args := Args(b, "/home/u/.config/execai/ui-window/chrome-0123", url, "linux")
	for _, want := range []string{
		"--user-data-dir=/home/u/.config/execai/ui-window/chrome-0123",
		"--no-first-run",
		"--no-default-browser-check",
		"--class=execai-ui",
		"--app=" + url,
	} {
		if !slices.Contains(args, want) {
			t.Errorf("no %s in %q", want, args)
		}
	}
	if args[len(args)-1] != "--app="+url {
		t.Errorf("the address is not the last argument: %q", args)
	}
	for _, a := range args {
		if strings.HasPrefix(a, "--remote-debugging") || a == "--headless" || strings.HasPrefix(a, "--headless=") {
			t.Errorf("the window is an ordinary one, without %s", a)
		}
	}
	// The window class is an X11/Wayland notion.
	for _, goos := range []string{"darwin", "windows"} {
		for _, a := range Args(b, "p", url, goos) {
			if strings.HasPrefix(a, "--class=") {
				t.Errorf("%s: %s", goos, a)
			}
		}
	}
}

// The profile of the window lives in the agent's config directory, one per
// browser and project; a browser in a sandbox gets it where its sandbox lets
// it write.
func TestProfileDir(t *testing.T) {
	const cfg, home, key = "/home/u/.config/execai", "/home/u", "0123456789abcdef"
	cases := []struct {
		b    Browser
		want string
	}{
		{Browser{ID: "chrome"}, "/home/u/.config/execai/ui-window/chrome-0123456789abcdef"},
		{Browser{ID: "edge"}, "/home/u/.config/execai/ui-window/edge-0123456789abcdef"},
		{Browser{ID: "chromium", Kind: "snap", Sandbox: "chromium"}, "/home/u/snap/chromium/common/execai-ui-window/0123456789abcdef"},
		{Browser{ID: "chrome", Kind: "flatpak", Sandbox: "com.google.Chrome"}, "/home/u/.var/app/com.google.Chrome/execai-ui-window/0123456789abcdef"},
		{Browser{ID: "My Browser/1"}, "/home/u/.config/execai/ui-window/my-browser-1-0123456789abcdef"},
	}
	for _, c := range cases {
		if got := ProfileDir(cfg, home, c.b, key, "linux"); got != c.want {
			t.Errorf("%+v: %s, want %s", c.b, got, c.want)
		}
	}
	if got := ProfileDir(`C:\Users\u\AppData\Roaming\execai`, `C:\Users\u`, Browser{ID: "edge"}, key, "windows"); got != `C:\Users\u\AppData\Roaming\execai\ui-window\edge-0123456789abcdef` {
		t.Errorf("windows: %s", got)
	}
}

// fakeProc — a browser process that ends when the test says so.
type fakeProc struct {
	exit    chan error
	stopped atomic.Int32
	once    sync.Once
}

func newFakeProc() *fakeProc { return &fakeProc{exit: make(chan error, 1)} }

func (p *fakeProc) Wait() error { return <-p.exit }
func (p *fakeProc) Pid() int    { return 4242 }
func (p *fakeProc) Stop() {
	p.stopped.Add(1)
	p.end(nil)
}
func (p *fakeProc) end(err error) { p.once.Do(func() { p.exit <- err }) }

func outcomeOf(t *testing.T, w *Watch) Outcome {
	t.Helper()
	select {
	case o := <-w.Done():
		return o
	case <-time.After(5 * time.Second):
		t.Fatal("the watch said nothing")
		return Outcome{}
	}
}

func TestWatch(t *testing.T) {
	timing := Timing{Quick: 80 * time.Millisecond, Grace: 120 * time.Millisecond, Gone: 100 * time.Millisecond, Poll: 5 * time.Millisecond}

	t.Run("the browser ends at once with an error: the start failed", func(t *testing.T) {
		p := newFakeProc()
		w := NewWatch(p, func() bool { return false }, timing)
		p.end(errors.New("exit status 1"))
		if o := outcomeOf(t, w); o.Kind != Failed || o.Err == nil {
			t.Fatalf("%+v", o)
		}
	})
	t.Run("the browser ends later without ever showing the page: the start failed", func(t *testing.T) {
		p := newFakeProc()
		w := NewWatch(p, func() bool { return false }, timing)
		time.Sleep(timing.Quick + 40*time.Millisecond)
		p.end(nil)
		if o := outcomeOf(t, w); o.Kind != Failed {
			t.Fatalf("%+v", o)
		}
	})
	t.Run("the page was shown and the browser ended: the window is closed", func(t *testing.T) {
		p := newFakeProc()
		var served atomic.Bool
		w := NewWatch(p, served.Load, timing)
		served.Store(true)
		time.Sleep(timing.Quick + 40*time.Millisecond)
		p.end(nil)
		if o := outcomeOf(t, w); o.Kind != Closed {
			t.Fatalf("%+v", o)
		}
	})
	t.Run("the launcher ends at once and the page is shown after it: a running copy took the window", func(t *testing.T) {
		p := newFakeProc()
		var served atomic.Bool
		w := NewWatch(p, served.Load, timing)
		p.end(nil)
		time.AfterFunc(40*time.Millisecond, func() { served.Store(true) })
		if o := outcomeOf(t, w); o.Kind != Handed {
			t.Fatalf("%+v", o)
		}
	})
	t.Run("the page is shown and the launcher ends within a moment: a running copy took the window", func(t *testing.T) {
		p := newFakeProc()
		w := NewWatch(p, func() bool { return true }, timing)
		p.end(nil)
		if o := outcomeOf(t, w); o.Kind != Handed {
			t.Fatalf("%+v", o)
		}
	})
	t.Run("Stop closes the window and says nothing more", func(t *testing.T) {
		p := newFakeProc()
		w := NewWatch(p, func() bool { return true }, timing)
		time.Sleep(timing.Quick + 40*time.Millisecond)
		w.Stop()
		if p.stopped.Load() != 1 {
			t.Fatal("the browser was not asked to close")
		}
		if o := outcomeOf(t, w); o.Kind != Stopped {
			t.Fatalf("%+v", o)
		}
	})
	// The page was told to close the window: a browser that goes by itself
	// writes what the page keeps (the chosen look) on its way out. Asked to
	// end at once, it lost what was chosen in the last minute.
	t.Run("Stop gives the browser a moment to go by itself and does not ask it then", func(t *testing.T) {
		leaving := timing
		leaving.Leave = 2 * time.Second
		p := newFakeProc()
		w := NewWatch(p, func() bool { return true }, leaving)
		time.Sleep(timing.Quick + 40*time.Millisecond)
		go func() {
			time.Sleep(60 * time.Millisecond)
			p.end(nil) // the window closed, the browser ended by itself
		}()
		start := time.Now()
		w.Stop()
		if d := time.Since(start); d > time.Second {
			t.Errorf("Stop waited %v for a browser that had already gone", d)
		}
		if n := p.stopped.Load(); n != 0 {
			t.Fatalf("the browser was asked to end %d time(s) though it went by itself", n)
		}
		if o := outcomeOf(t, w); o.Kind != Stopped {
			t.Fatalf("%+v", o)
		}
	})
	t.Run("a browser that stays is asked to end after that moment", func(t *testing.T) {
		leaving := timing
		leaving.Leave = 150 * time.Millisecond
		p := newFakeProc()
		w := NewWatch(p, func() bool { return true }, leaving)
		time.Sleep(timing.Quick + 40*time.Millisecond)
		start := time.Now()
		w.Stop()
		if d := time.Since(start); d < leaving.Leave {
			t.Errorf("Stop asked after %v, before the moment (%v) was over", d, leaving.Leave)
		}
		if p.stopped.Load() != 1 {
			t.Fatal("the browser that stayed was not asked to end")
		}
		if o := outcomeOf(t, w); o.Kind != Stopped {
			t.Fatalf("%+v", o)
		}
	})
	t.Run("a window that is already closed costs Stop nothing", func(t *testing.T) {
		leaving := timing
		leaving.Leave = 2 * time.Second
		p := newFakeProc()
		w := NewWatch(p, func() bool { return true }, leaving)
		time.Sleep(timing.Quick + 40*time.Millisecond)
		p.end(nil)
		if o := outcomeOf(t, w); o.Kind != Closed {
			t.Fatalf("%+v", o)
		}
		start := time.Now()
		w.Stop()
		if d := time.Since(start); d > 500*time.Millisecond {
			t.Errorf("Stop waited %v for a browser that had ended before", d)
		}
	})
}

func TestOpen(t *testing.T) {
	b := Browser{ID: "chrome", Path: "/usr/bin/google-chrome"}
	var gotPath string
	var gotArgs []string
	p := newFakeProc()
	start := func(path string, args []string) (Proc, error) {
		gotPath, gotArgs = path, args
		return p, nil
	}
	proc, err := Open(start, b, "/cfg/ui-window/chrome-k", "http://127.0.0.1:23750/", "linux")
	if err != nil || proc != Proc(p) {
		t.Fatalf("%v %v", proc, err)
	}
	if gotPath != b.Path || !slices.Contains(gotArgs, "--app=http://127.0.0.1:23750/") ||
		!slices.Contains(gotArgs, "--user-data-dir=/cfg/ui-window/chrome-k") {
		t.Fatalf("%s %q", gotPath, gotArgs)
	}
	boom := errors.New("exec: not found")
	if _, err := Open(func(string, []string) (Proc, error) { return nil, boom }, b, "p", "u", "linux"); !errors.Is(err, boom) {
		t.Fatalf("a start that fails: %v", err)
	}
}

// Where the browser outlives its last window (macOS), or the window lives
// in a browser the panel did not start, the end of a process tells nothing.
// There the window is known to be closed by its page: it said goodbye and
// stays away. A page that is merely not there — it never came, or it
// crashed — is not a closed window.
func TestGone(t *testing.T) {
	timing := Timing{Gone: 100 * time.Millisecond, Poll: 5 * time.Millisecond}
	closed := func(ch <-chan struct{}, within time.Duration) bool {
		select {
		case <-ch:
			return true
		case <-time.After(within):
			return false
		}
	}

	t.Run("a page that never left is not a closed window", func(t *testing.T) {
		stop := make(chan struct{})
		defer close(stop)
		if closed(Gone(func() bool { return false }, timing, stop), 3*timing.Gone) {
			t.Fatal("gone without having left")
		}
	})
	t.Run("left for good", func(t *testing.T) {
		var left atomic.Bool
		stop := make(chan struct{})
		defer close(stop)
		gone := Gone(left.Load, timing, stop)
		if closed(gone, 2*timing.Gone) {
			t.Fatal("gone while the page is there")
		}
		left.Store(true)
		started := time.Now()
		if !closed(gone, 5*time.Second) {
			t.Fatal("not noticed")
		}
		if d := time.Since(started); d < timing.Gone {
			t.Fatalf("gone after %v, sooner than the wait of %v", d, timing.Gone)
		}
	})
	t.Run("a reload — left for a moment — is not a closed window", func(t *testing.T) {
		var left atomic.Bool
		stop := make(chan struct{})
		defer close(stop)
		gone := Gone(left.Load, timing, stop)
		for i := 0; i < 4; i++ {
			left.Store(true)
			time.Sleep(timing.Gone / 2)
			left.Store(false)
			time.Sleep(4 * timing.Poll)
		}
		if closed(gone, 2*timing.Gone) {
			t.Fatal("a short absence was taken for a closed window")
		}
	})
	t.Run("stop ends the watch", func(t *testing.T) {
		var left atomic.Bool
		stop := make(chan struct{})
		gone := Gone(left.Load, timing, stop)
		close(stop)
		time.Sleep(4 * timing.Poll)
		left.Store(true)
		if closed(gone, 3*timing.Gone) {
			t.Fatal("a stopped watch spoke")
		}
	})
}

const jumpKeyURL = "http://127.0.0.1:23750/?t=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// The window is not started with the address that carries the one-time
// key: the arguments of a process can be read by every program on the
// computer. The key travels in a file only the person can read, which sends
// the window on to the address.
func TestJump(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "ui-window", "chrome-k")
	path, err := WriteJump(profile, jumpKeyURL)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != profile {
		t.Fatalf("the file is at %s, not in the window's profile", path)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v: others may read the key", fi.Mode().Perm())
	}
	if di, _ := os.Stat(profile); di.Mode().Perm() != 0o700 {
		t.Errorf("the profile directory is %v", di.Mode().Perm())
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `location.replace("`+jumpKeyURL+`")`) {
		t.Errorf("the file does not send the window on:\n%s", data)
	}

	// A file left by an earlier run is replaced, not added to; a link put
	// in its place is not written through.
	other := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(other, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, path); err == nil {
		next := strings.Replace(jumpKeyURL, "0123", "ffff", 1)
		if _, err := WriteJump(profile, next); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(other); string(b) != "untouched" {
			t.Fatalf("the key was written through a link: %q", b)
		}
		if li, err := os.Lstat(path); err != nil || li.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("the link is still there: %v %v", li, err)
		}
		if b, _ := os.ReadFile(path); !strings.Contains(string(b), next) || strings.Contains(string(b), jumpKeyURL) {
			t.Errorf("the file after a second write:\n%s", b)
		}
	}

	RemoveJump(path)
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("the file is still there")
	}
	RemoveJump(path) // gone already: nothing to do
	RemoveJump("")
}

// The window's argument is the file, and nothing in the command line is
// the key.
func TestJump_ArgumentsCarryNoKey(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "ui-window", "chrome-k")
	path, err := WriteJump(profile, jumpKeyURL)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	start := func(_ string, args []string) (Proc, error) {
		got = args
		return newFakeProc(), nil
	}
	if _, err := Open(start, Browser{ID: "chrome", Path: "/usr/bin/google-chrome"}, profile, FileURL(path, "linux"), "linux"); err != nil {
		t.Fatal(err)
	}
	if got[len(got)-1] != "--app=file://"+path {
		t.Errorf("the last argument is %q", got[len(got)-1])
	}
	for _, a := range got {
		if strings.Contains(a, "0123456789abcdef") || strings.Contains(a, "?t=") {
			t.Errorf("the key is in an argument: %q", a)
		}
	}
}

func TestFileURL(t *testing.T) {
	cases := []struct{ goos, path, want string }{
		{"linux", "/home/u/.config/execai/ui-window/chrome-k/execai-open.html", "file:///home/u/.config/execai/ui-window/chrome-k/execai-open.html"},
		{"linux", "/home/u/my dir/a#b?c%d/execai-open.html", "file:///home/u/my%20dir/a%23b%3Fc%25d/execai-open.html"},
		{"darwin", "/Users/u/Library/Application Support/execai/ui-window/edge-k/execai-open.html", "file:///Users/u/Library/Application%20Support/execai/ui-window/edge-k/execai-open.html"},
		{"windows", `C:\Users\u\AppData\Roaming\execai\ui-window\edge-k\execai-open.html`, "file:///C:/Users/u/AppData/Roaming/execai/ui-window/edge-k/execai-open.html"},
		{"windows", `C:\Users\Иван Петров\execai\execai-open.html`, "file:///C:/Users/%D0%98%D0%B2%D0%B0%D0%BD%20%D0%9F%D0%B5%D1%82%D1%80%D0%BE%D0%B2/execai/execai-open.html"},
	}
	for _, c := range cases {
		if got := FileURL(c.path, c.goos); got != c.want {
			t.Errorf("%s %s:\n got %s\nwant %s", c.goos, c.path, got, c.want)
		}
	}
}
