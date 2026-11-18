package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/ide"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
	"github.com/velesbsdllc/agent-vbai/internal/uiserve"
	"github.com/velesbsdllc/agent-vbai/internal/uiwindow"
)

// uiProc — a browser process the test ends by hand.
type uiProc struct {
	exit    chan error
	once    sync.Once
	stopped atomic.Int32
}

func newUIProc() *uiProc           { return &uiProc{exit: make(chan error, 1)} }
func (p *uiProc) Wait() error      { return <-p.exit }
func (p *uiProc) Pid() int         { return 4242 }
func (p *uiProc) end(err error)    { p.once.Do(func() { p.exit <- err }) }
func (p *uiProc) Stop()            { p.stopped.Add(1); p.end(nil) }
func (p *uiProc) wasStopped() bool { return p.stopped.Load() > 0 }

type uiStart struct {
	path string
	args []string
	proc *uiProc
}

// uiRig — execai ui with nothing real behind it: no browser is started, no
// tab opened, the agent is a function that waits for the end.
type uiRig struct {
	t      *testing.T
	cfg    *config.Config
	deps   uiDeps
	dir    string // the project folder
	out    *lockedBuffer
	chrome uiwindow.Browser

	mu       sync.Mutex
	finds    []string
	findErr  map[string]error // by the explicit name; "" — the search
	starts   []uiStart
	startErr error
	onStart  func(p *uiProc) // called with each started process
	tabs     []string
	ides     int
	notices  []string // desktop notifications: "title | body"
	ports    []int    // where this rig's panels listen: free ports outside the range of real panels
}

// freePorts — n ports nobody listens on now, picked by the system.
func freePorts(t *testing.T, n int) []int {
	t.Helper()
	var ports []int
	var held []net.Listener
	for i := 0; i < n; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, ln)
		ports = append(ports, ln.Addr().(*net.TCPAddr).Port)
	}
	for _, ln := range held {
		ln.Close()
	}
	return ports
}

func newUIRig(t *testing.T) *uiRig {
	t.Helper()
	prev := i18n.Locale()
	i18n.SetLocale("en")
	oldSync := startIDESync
	startIDESync = func(context.Context, *config.Config, agentsync.Hooks) func() { return func() {} }
	t.Cleanup(func() {
		startIDESync = oldSync
		i18n.SetLocale(prev)
	})
	home := t.TempDir()
	r := &uiRig{t: t, cfg: config.Defaults(), dir: t.TempDir(), out: &lockedBuffer{}, findErr: map[string]error{},
		chrome: uiwindow.Browser{ID: "chrome", Name: "Google Chrome", Path: "/usr/bin/google-chrome"}}
	r.cfg.Locale = "en"
	r.ports = freePorts(t, 3)
	r.deps = uiDeps{
		ports: func(string) []int { return r.ports },
		notify: func(title, body string) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.notices = append(r.notices, title+" | "+body)
		},
		goos:      "linux",
		home:      home,
		configDir: filepath.Join(home, "cfg"),
		getenv:    screenEnv,
		exe:       func() (string, error) { return "/usr/local/bin/execai", nil },
		find: func(explicit string) (uiwindow.Browser, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.finds = append(r.finds, explicit)
			if err := r.findErr[explicit]; err != nil {
				return uiwindow.Browser{}, err
			}
			return r.chrome, nil
		},
		start: func(path string, args []string) (uiwindow.Proc, error) {
			r.mu.Lock()
			if r.startErr != nil {
				err := r.startErr
				r.mu.Unlock()
				return nil, err
			}
			p := newUIProc()
			r.starts = append(r.starts, uiStart{path, args, p})
			hook := r.onStart
			r.mu.Unlock()
			if hook != nil {
				hook(p)
			}
			return p, nil
		},
		openTab: func(url string) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.tabs = append(r.tabs, url)
			return nil
		},
		ide: func(*config.Config, string, *ide.SyncNotices) uiserve.IDE {
			return func(ctx context.Context, in io.Reader, out io.Writer) error {
				r.mu.Lock()
				r.ides++
				r.mu.Unlock()
				<-ctx.Done()
				return nil
			}
		},
		timing: uiwindow.Timing{Quick: 60 * time.Millisecond, Grace: 150 * time.Millisecond, Gone: 150 * time.Millisecond,
			Jump: 10 * time.Second, Poll: 5 * time.Millisecond},
		stateWait: 2 * time.Second,
		stopWait:  5 * time.Second,
	}
	return r
}

// screenEnv — the environment of a desktop session.
func screenEnv(k string) string {
	if k == "DISPLAY" {
		return ":0"
	}
	return ""
}

// noFolder — for run: leave --cwd out (run puts the project folder there
// otherwise).
const noFolder = "\x00none"

// run starts execai ui in the background; the returned channel gives its
// result, cancel is Ctrl+C.
func (r *uiRig) run(f uiFlags) (done chan error, cancel func()) {
	switch {
	case f.cwd == noFolder:
		f.cwd = ""
	case f.cwd == "" && !f.last:
		f.cwd = r.dir
	}
	ctx, cancel := context.WithCancel(context.Background())
	done = make(chan error, 1)
	ended := make(chan struct{})
	go func() {
		done <- runUI(ctx, r.out, r.cfg, f, r.deps)
		close(ended)
	}()
	r.t.Cleanup(func() {
		cancel()
		select {
		case <-ended:
		case <-time.After(5 * time.Second):
		}
	})
	return done, cancel
}

func (r *uiRig) wait(what string, cond func() bool) {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.t.Fatalf("timed out waiting for %s; output:\n%s", what, r.out.String())
}

// waitPrinted — sub is in the output n times. The address line is printed
// after the folder is taken, so only then may a test look at the state
// (looking takes the lock for a moment).
func (r *uiRig) waitPrinted(sub string, n int) {
	r.t.Helper()
	r.wait(strconv.Itoa(n)+" × "+sub, func() bool { return strings.Count(r.out.String(), sub) == n })
}

func (r *uiRig) nStarts() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.starts) }
func (r *uiRig) nTabs() int   { r.mu.Lock(); defer r.mu.Unlock(); return len(r.tabs) }
func (r *uiRig) startN(i int) uiStart {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts[i]
}
func (r *uiRig) tab(i int) string { r.mu.Lock(); defer r.mu.Unlock(); return r.tabs[i] }

var jumpTarget = regexp.MustCompile(`location\.replace\("(http://127\.0\.0\.1:\d+/\?t=[0-9a-f]{64})"\)`)

// jumpFile — the file the i-th started browser was pointed at.
func (r *uiRig) jumpFile(i int) string {
	r.t.Helper()
	app := argOf(r.startN(i).args, "--app=")
	path, ok := strings.CutPrefix(app, "file://")
	if !ok {
		r.t.Fatalf("the browser was started with %q, not with a file", app)
	}
	return path
}

// keyURL — the address with the one-time key the i-th started browser is
// sent to. It is not among the browser's arguments: it is in the file the
// arguments name.
func (r *uiRig) keyURL(i int) string {
	r.t.Helper()
	data, err := os.ReadFile(r.jumpFile(i))
	if err != nil {
		r.t.Fatal(err)
	}
	m := jumpTarget.FindSubmatch(data)
	if m == nil {
		r.t.Fatalf("the file does not send the window to the panel:\n%s", data)
	}
	return string(m[1])
}

// windowComesUp — the i-th started browser does what a browser does: goes
// to the address in its file. The returned client holds the session; base
// is the panel's address without the key.
func (r *uiRig) windowComesUp(i int) (client *http.Client, base string) {
	r.t.Helper()
	jar, _ := cookiejar.New(nil)
	client = &http.Client{Jar: jar}
	key := r.keyURL(i)
	resp, err := client.Get(key)
	if err != nil {
		r.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		r.t.Fatalf("the page: %d", resp.StatusCode)
	}
	r.wait("the window to be marked as up", func() bool { s, _ := r.state(); return s.WindowUp })
	return client, strings.SplitN(key, "?", 2)[0]
}

// stream opens the page's event stream; the returned function ends it.
func (r *uiRig) stream(client *http.Client, base string) (end func()) {
	r.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"events", nil)
	resp, err := client.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		r.t.Fatalf("event stream: %d", resp.StatusCode)
	}
	go func() { _, _ = io.Copy(io.Discard, resp.Body) }()
	return func() {
		cancel()
		resp.Body.Close()
	}
}

// goodbye — what the window's page sends when it is closed or reloaded.
func (r *uiRig) goodbye(client *http.Client, base string) {
	r.t.Helper()
	req, _ := http.NewRequest("POST", base+"bye", strings.NewReader(""))
	req.Header.Set("Origin", strings.TrimSuffix(base, "/"))
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	resp, err := client.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		r.t.Fatalf("goodbye: %d", resp.StatusCode)
	}
}

func argOf(args []string, prefix string) string {
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, prefix); ok {
			return v
		}
	}
	return ""
}

// state — what a second execai ui would read about the folder.
func (r *uiRig) state() (st uiwindow.State, running bool) {
	r.t.Helper()
	in, held, err := uiwindow.Acquire(r.deps.configDir, r.dir)
	if err != nil {
		r.t.Fatal(err)
	}
	st, _ = in.Read()
	if held {
		// Not running: give the folder back as it was found.
		in.Release()
		return st, false
	}
	in.Release()
	return st, true
}

func finished(t *testing.T, done chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("execai ui did not stop")
		return nil
	}
}

func stillRunning(t *testing.T, done chan error, d time.Duration) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("execai ui stopped: %v", err)
	case <-time.After(d):
	}
}

// With a browser of the family the panel opens as an app window; when the
// person closes it, execai ui stops by itself and says why.
func TestUI_WindowAndItsClosingStopsThePanel(t *testing.T) {
	r := newUIRig(t)
	done, _ := r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	st := r.startN(0)
	if st.path != r.chrome.Path {
		t.Fatalf("started %s", st.path)
	}
	url := r.keyURL(0)
	profile := argOf(st.args, "--user-data-dir=")
	wantProfile := filepath.Join(r.deps.configDir, "ui-window", "chrome-"+uiwindow.Key(r.dir))
	if profile != wantProfile || r.jumpFile(0) != filepath.Join(wantProfile, "execai-open.html") {
		t.Fatalf("args %q", st.args)
	}
	if r.nTabs() != 0 {
		t.Fatalf("a tab was opened too: %q", r.tabs)
	}
	r.wait("the state file", func() bool { s, _ := r.state(); return s.Mode == uiwindow.ModeWindow })
	state, running := r.state()
	if !running || state.PID != os.Getpid() || state.Cwd != r.dir || state.Profile != wantProfile ||
		state.Browser == nil || state.Browser.Path != r.chrome.Path || state.Control == "" || state.WindowUp {
		t.Fatalf("state %+v running=%v", state, running)
	}
	stable := "http://127.0.0.1:" + strconv.Itoa(state.Port) + "/"
	if !strings.HasPrefix(url, stable+"?t=") {
		t.Fatalf("the window's address %s is not on the port of the state %d", url, state.Port)
	}
	out := r.out.String()
	if !strings.Contains(out, "window of its own (Google Chrome)") || !strings.Contains(out, stable) {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(out, "?t=") || strings.Contains(out, "One-time code") || strings.Contains(out, state.Control) {
		t.Errorf("the terminal shows the key, a code nobody asked for or the control secret:\n%s", out)
	}

	// The browser fetches the page; later the person closes the window.
	jar, _ := cookiejar.New(nil)
	resp, err := (&http.Client{Jar: jar}).Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the page: %d", resp.StatusCode)
	}
	time.Sleep(r.deps.timing.Quick + 40*time.Millisecond)
	stillRunning(t, done, 50*time.Millisecond)
	st.proc.end(nil)
	if err := finished(t, done); err != nil {
		t.Fatalf("execai ui after the window closed: %v", err)
	}
	if out := r.out.String(); !strings.Contains(out, "The window is closed") || !strings.Contains(out, "The panel is stopped.") {
		t.Errorf("output:\n%s", out)
	}
	// The folder is free again; the address is remembered, the rest is gone.
	state, running = r.state()
	if running || state.PID != 0 || state.Control != "" || state.WindowUp || stable != "http://127.0.0.1:"+strconv.Itoa(state.Port)+"/" {
		t.Fatalf("state after the stop %+v running=%v", state, running)
	}

	// The next run of the project asks for the same address.
	done, cancel := r.run(uiFlags{})
	r.wait("the second browser start", func() bool { return r.nStarts() == 2 })
	if got := r.keyURL(1); !strings.HasPrefix(got, stable+"?t=") {
		t.Errorf("the next run is at %s, the last one was at %s", got, stable)
	}
	// Ctrl+C in the terminal closes the window.
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	if !r.startN(1).proc.wasStopped() {
		t.Error("Ctrl+C left the window open")
	}
}

// Where the browser goes on running after its last window is closed (macOS
// keeps applications without windows), its end tells nothing. The window is
// then known to be closed by its page: it says goodbye and stays away. A
// reload says goodbye and is back; a page that crashed says nothing — the
// window is still there, a turn may be running, and nothing stops.
func TestUI_WindowClosedWhileItsBrowserRunsOn(t *testing.T) {
	r := newUIRig(t)
	r.deps.goos = "darwin"
	done, _ := r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	st := r.startN(0)
	page, base := r.windowComesUp(0)
	// The page connects, reloads twice, stays.
	end := r.stream(page, base)
	for i := 0; i < 2; i++ {
		time.Sleep(r.deps.timing.Gone / 2)
		r.goodbye(page, base)
		end()
		time.Sleep(r.deps.timing.Gone / 3)
		end = r.stream(page, base)
	}
	// It crashes: gone without a word, and back when the person reloads.
	end()
	stillRunning(t, done, 3*r.deps.timing.Gone)
	if st.proc.wasStopped() {
		t.Fatal("the browser was closed under an open window")
	}
	end = r.stream(page, base)
	// The window is closed; the browser process lives on.
	r.goodbye(page, base)
	end()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	if out := r.out.String(); !strings.Contains(out, "The window is closed") {
		t.Errorf("output:\n%s", out)
	}
	if !st.proc.wasStopped() {
		t.Error("the browser without windows was left running")
	}
}

// While the panel's own browser runs (Linux, Windows: it ends with its last
// window), a page that goes away — goodbye or not — stops nothing: the end
// of the browser is the sign, and a page can leave a living window.
func TestUI_OwnBrowserRuns_PageAwayStopsNothing(t *testing.T) {
	r := newUIRig(t)
	done, cancel := r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	page, base := r.windowComesUp(0)
	end := r.stream(page, base)
	time.Sleep(r.deps.timing.Gone / 2)
	r.goodbye(page, base)
	end()
	stillRunning(t, done, 3*r.deps.timing.Gone)
	if r.startN(0).proc.wasStopped() || strings.Contains(r.out.String(), "The window is closed") {
		t.Fatalf("the panel took a page that went away for a closed window:\n%s", r.out.String())
	}
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}

// The started program ends at once and the page comes up all the same: a
// browser that was already running with the profile — left by a panel that
// was killed — took the window. Its end is not the panel's to see, so this
// window is known to be closed by its page.
func TestUI_WindowInABrowserLeftBehind(t *testing.T) {
	r := newUIRig(t)
	r.onStart = func(p *uiProc) { p.end(nil) }
	done, _ := r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	page, base := r.windowComesUp(0)
	end := r.stream(page, base)
	stillRunning(t, done, r.deps.timing.Grace+3*r.deps.timing.Gone)
	r.goodbye(page, base)
	end()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	if out := r.out.String(); !strings.Contains(out, "The window is closed") || strings.Contains(out, "did not open") {
		t.Errorf("output:\n%s", out)
	}
}

// The browser runs and never comes for the page — it shows something of its
// own. After a while the person is told the other ways in: the code and the
// link. The panel goes on.
func TestUI_BrowserShowsNothing(t *testing.T) {
	r := newUIRig(t)
	r.deps.timing.Silent = 80 * time.Millisecond
	done, cancel := r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	r.waitPrinted("never showed the panel", 1)
	out := r.out.String()
	if !strings.Contains(out, "execai ui --code") || strings.Count(out, "ExecAI panel: http://127.0.0.1:") != 1 || !strings.Contains(out, "/?t=") {
		t.Errorf("output:\n%s", out)
	}
	stillRunning(t, done, 100*time.Millisecond)
	if r.nTabs() != 0 || r.startN(0).proc.wasStopped() {
		t.Error("the panel opened a tab or closed the browser by itself")
	}
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}

	// A window that came up is told nothing.
	r = newUIRig(t)
	r.deps.timing.Silent = 80 * time.Millisecond
	done, cancel = r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	r.windowComesUp(0)
	time.Sleep(200 * time.Millisecond)
	if strings.Contains(r.out.String(), "never showed the panel") {
		t.Errorf("output:\n%s", r.out.String())
	}
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}

// A tab is the person's: closing it stops nothing, whatever opened it.
func TestUI_ClosedTabStopsNothing(t *testing.T) {
	r := newUIRig(t)
	r.onStart = func(p *uiProc) { p.end(errors.New("exit status 1")) } // the window fails, a tab takes over
	done, cancel := r.run(uiFlags{})
	r.wait("the tab", func() bool { return r.nTabs() == 1 })
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	resp, err := browser.Get(r.tab(0))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	ctx, closeTab := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", strings.SplitN(r.tab(0), "?", 2)[0]+"events", nil)
	events, err := browser.Do(req)
	if err != nil || events.StatusCode != http.StatusOK {
		t.Fatalf("event stream: %v", err)
	}
	time.Sleep(r.deps.timing.Gone / 2)
	closeTab()
	events.Body.Close()
	stillRunning(t, done, 3*r.deps.timing.Gone)
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}

// No browser of the family (only Firefox, only Safari): a tab of the
// default browser, as before, and one line about what would give a window.
func TestUI_NoBrowser_TabAndOneLine(t *testing.T) {
	r := newUIRig(t)
	r.findErr[""] = uiwindow.ErrNoBrowser
	done, cancel := r.run(uiFlags{})
	r.wait("the tab", func() bool { return r.nTabs() == 1 })
	if !strings.Contains(r.tab(0), "/?t=") {
		t.Fatalf("the tab got %s", r.tab(0))
	}
	if r.nStarts() != 0 {
		t.Fatal("a browser was started")
	}
	r.wait("the state file", func() bool { s, _ := r.state(); return s.Mode == uiwindow.ModeTab })
	out := r.out.String()
	supported := uiwindow.Supported("linux")
	if n := strings.Count(out, supported); n != 1 {
		t.Errorf("the supported browsers are named %d times:\n%s", n, out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, supported) && !strings.Contains(line, "opening a tab") {
			t.Errorf("line %q", line)
		}
	}
	if !strings.Contains(out, "ExecAI panel: "+r.tab(0)) {
		t.Errorf("the link is not printed:\n%s", out)
	}
	stillRunning(t, done, 100*time.Millisecond)
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}

// --window: no browser is an error that lists the suitable ones, before
// anything starts.
func TestUI_WindowFlag_NoBrowserIsAnError(t *testing.T) {
	r := newUIRig(t)
	r.findErr[""] = uiwindow.ErrNoBrowser
	done, _ := r.run(uiFlags{window: true})
	err := finished(t, done)
	if err == nil || !strings.Contains(err.Error(), uiwindow.Supported("linux")) {
		t.Fatalf("error %v", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ides != 0 || len(r.tabs) != 0 || len(r.starts) != 0 {
		t.Fatalf("something started: ides=%d tabs=%q starts=%d", r.ides, r.tabs, len(r.starts))
	}
}

// --window asked for a window and nothing else: a browser that is found
// and does not give one is an error, not a tab and a success.
func TestUI_WindowFlag_BrowserFails(t *testing.T) {
	r := newUIRig(t)
	r.startErr = errors.New("exec format error")
	done, _ := r.run(uiFlags{window: true})
	if err := finished(t, done); err == nil || !strings.Contains(err.Error(), "did not open in Google Chrome: exec format error") {
		t.Fatalf("--window, the browser cannot be started: %v", err)
	}
	r = newUIRig(t)
	r.onStart = func(p *uiProc) { p.end(errors.New("exit status 1")) }
	done, _ = r.run(uiFlags{window: true})
	if err := finished(t, done); err == nil || !strings.Contains(err.Error(), "did not open in Google Chrome: exit status 1") {
		t.Fatalf("--window, the browser ends at once: %v", err)
	}
	if r.nTabs() != 0 {
		t.Errorf("a tab was opened: %q", r.tabs)
	}
}

// The browser cannot be started: a tab instead, the command goes on.
func TestUI_StartFails_Tab(t *testing.T) {
	r := newUIRig(t)
	r.startErr = errors.New("exec format error")
	done, cancel := r.run(uiFlags{})
	r.wait("the tab", func() bool { return r.nTabs() == 1 })
	if !strings.Contains(r.tab(0), "/?t=") {
		t.Fatalf("the tab got %s", r.tab(0))
	}
	out := r.out.String()
	if !strings.Contains(out, "did not open in Google Chrome (exec format error)") {
		t.Errorf("output:\n%s", out)
	}
	stillRunning(t, done, 100*time.Millisecond)
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}

// The browser starts and ends at once with an error, the page never
// fetched: a tab instead, the command goes on.
func TestUI_BrowserDiesAtOnce_Tab(t *testing.T) {
	r := newUIRig(t)
	r.onStart = func(p *uiProc) { p.end(errors.New("exit status 1")) }
	done, cancel := r.run(uiFlags{})
	r.wait("the tab", func() bool { return r.nTabs() == 1 })
	if !strings.Contains(r.tab(0), "/?t=") {
		t.Fatalf("the tab got %s", r.tab(0))
	}
	r.wait("the state file", func() bool { s, _ := r.state(); return s.Mode == uiwindow.ModeTab })
	if out := r.out.String(); !strings.Contains(out, "did not open in Google Chrome (exit status 1)") || strings.Contains(out, "The window is closed") {
		t.Errorf("output:\n%s", out)
	}
	stillRunning(t, done, 100*time.Millisecond)
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}

// Linux without a graphical session (a server reached over SSH — where
// other people have accounts too): nothing is started at all. Not a
// browser, and not the system's opener either: it would get the address
// with the key as an argument, for every program of the computer to read,
// and has no screen to open anything on. The link is printed for the person
// to open themselves. --window says why it cannot be.
func TestUI_NoScreen(t *testing.T) {
	r := newUIRig(t)
	r.deps.getenv = func(string) string { return "" }
	for i, f := range []uiFlags{{}, {browser: true}, {noOpen: true}} {
		done, cancel := r.run(f)
		r.waitPrinted("ExecAI panel: http://127.0.0.1:", i+1)
		r.wait("the state file", func() bool { s, running := r.state(); return running && s.Mode == uiwindow.ModeNone })
		// A second run does not call the opener either.
		second, _ := r.run(uiFlags{})
		if err := finished(t, second); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
		cancel()
		if err := finished(t, done); err != nil {
			t.Fatal(err)
		}
	}
	r.mu.Lock()
	if len(r.tabs) != 0 || len(r.starts) != 0 || len(r.finds) != 0 {
		t.Errorf("without a screen: the opener was called %d times (%q), browsers started %d, looked for %q",
			len(r.tabs), r.tabs, len(r.starts), r.finds)
	}
	r.mu.Unlock()
	out := r.out.String()
	// Said where a browser would have been opened; not where the person
	// asked for nothing to be opened anyway.
	if n := strings.Count(out, "No graphical session"); n != 2 {
		t.Errorf("the missing screen is named %d times, want 2:\n%s", n, out)
	}
	if strings.Contains(out, "Google Chrome") || strings.Count(out, "/?t=") != 3 {
		t.Errorf("output:\n%s", out)
	}
	done, _ := r.run(uiFlags{window: true})
	if err := finished(t, done); err == nil || !strings.Contains(err.Error(), "DISPLAY") {
		t.Fatalf("--window without a screen: %v", err)
	}

	// A panel that was started at the screen, as a tab; then a second run
	// from a session without one (the same person, over SSH): it tells the
	// address and starts nothing.
	r.deps.getenv = screenEnv
	atScreen, stop := r.run(uiFlags{browser: true})
	r.wait("the tab", func() bool { return r.nTabs() == 1 })
	r.wait("the state file", func() bool { s, _ := r.state(); return s.Mode == uiwindow.ModeTab })
	r.deps.getenv = func(string) string { return "" }
	before := len(r.out.String())
	for _, f := range []uiFlags{{}, {browser: true}} {
		second, _ := r.run(f)
		if err := finished(t, second); err != nil {
			t.Fatal(err)
		}
	}
	if r.nTabs() != 1 || strings.Count(r.out.String()[before:], "already open: http://127.0.0.1:") != 2 {
		t.Errorf("a second run without a screen: opener calls %d, output:\n%s", r.nTabs(), r.out.String()[before:])
	}
	stop()
	if err := finished(t, atScreen); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	r.tabs = nil
	r.mu.Unlock()

	// A Wayland session is a screen too; macOS and Windows have no such
	// variable to look at.
	r.deps.getenv = func(k string) string {
		if k == "WAYLAND_DISPLAY" {
			return "wayland-0"
		}
		return ""
	}
	done, cancel := r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	r.deps.getenv = func(string) string { return "" }
	r.deps.goos = "darwin"
	done, cancel = r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 2 })
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}

// The window's profile and the state of the running panel are this
// computer's: none of them is a file sync sends or lies in a folder it
// walks.
func TestUI_SyncLeavesTheWindowAlone(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "execai")
	paths := agentsync.Paths{ConfigDir: cfg}
	synced := []string{paths.MemoryDir()}
	for _, name := range agentsync.ConfigFiles {
		synced = append(synced, filepath.Join(cfg, name))
	}
	in, _, err := uiwindow.Acquire(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer in.Release()
	if err := uiwindow.SaveLastDir(cfg, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ours := []string{
		uiwindow.ProfileDir(cfg, "/home/u", uiwindow.Browser{ID: "chrome"}, in.Key(), "linux"),
		in.StateFile(),
		filepath.Join(cfg, "ui", "last.json"),
	}
	within := func(path, dir string) bool {
		rel, err := filepath.Rel(dir, path)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	for _, o := range ours {
		for _, s := range synced {
			if within(o, s) || within(s, o) {
				t.Errorf("%s and the synced %s overlap", o, s)
			}
		}
	}
}

// --browser and --no-open do not look for a browser at all.
func TestUI_BrowserAndNoOpenFlags(t *testing.T) {
	r := newUIRig(t)
	done, cancel := r.run(uiFlags{browser: true})
	r.wait("the tab", func() bool { return r.nTabs() == 1 })
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}

	done, cancel = r.run(uiFlags{noOpen: true})
	r.waitPrinted("ExecAI panel: ", 2)
	r.wait("the state file", func() bool { s, running := r.state(); return running && s.Mode == uiwindow.ModeNone })
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.finds) != 0 || len(r.starts) != 0 || len(r.tabs) != 1 {
		t.Fatalf("finds=%q starts=%d tabs=%q", r.finds, len(r.starts), r.tabs)
	}
}

func TestUI_FlagsThatDoNotGoTogether(t *testing.T) {
	r := newUIRig(t)
	for _, f := range []uiFlags{
		{window: true, browser: true}, {window: true, noOpen: true}, {browser: true, noOpen: true},
		{installShortcut: true, removeShortcut: true}, {code: true, installShortcut: true},
		{stop: true, code: true}, {stop: true, removeShortcut: true},
	} {
		done, _ := r.run(f)
		if err := finished(t, done); err == nil || !strings.Contains(err.Error(), "do not go together") {
			t.Errorf("%+v: %v", f, err)
		}
	}
	// --all is a way to stop, nothing else.
	done, _ := r.run(uiFlags{all: true})
	if err := finished(t, done); err == nil || !strings.Contains(err.Error(), "--stop") {
		t.Errorf("--all alone: %v", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ides != 0 {
		t.Fatal("the panel started")
	}
}

// execai ui --stop ends the panel running for the folder the way Ctrl+C in
// its terminal would: the window closes, the state is cleared. With nothing
// running it succeeds and says so.
func TestUI_Stop(t *testing.T) {
	r := newUIRig(t)
	done, _ := r.run(uiFlags{stop: true})
	if err := finished(t, done); err != nil {
		t.Fatalf("--stop with no panel: %v", err)
	}
	if out := r.out.String(); !strings.Contains(out, "No panel is running for this folder.") {
		t.Errorf("output:\n%s", out)
	}

	first, _ := r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	r.wait("the state file", func() bool { s, running := r.state(); return running && s.Control != "" })
	before := len(r.out.String())
	done, _ = r.run(uiFlags{stop: true})
	if err := finished(t, done); err != nil {
		t.Fatalf("--stop: %v", err)
	}
	// By the time --stop returns the panel has let the folder go, not merely
	// been asked: its command returns within the same moment.
	select {
	case err := <-first:
		if err != nil {
			t.Fatalf("the stopped panel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("--stop returned while the panel still runs")
	}
	if !r.startN(0).proc.wasStopped() {
		t.Error("the window was left open")
	}
	out := r.out.String()[before:]
	if !strings.Contains(out, "The panel is stopped: "+r.dir) || !strings.Contains(out, "Asked to stop by execai ui --stop.") {
		t.Errorf("output:\n%s", out)
	}
	if st, running := r.state(); running || st.PID != 0 || st.Control != "" || st.Port == 0 {
		t.Errorf("state after --stop: %+v running=%v", st, running)
	}

	// A panel without a window (a tab; started from a shortcut, it has no
	// terminal to press Ctrl+C in) is stopped the same way.
	first, _ = r.run(uiFlags{browser: true})
	r.wait("the tab", func() bool { return r.nTabs() == 1 })
	r.wait("the state file", func() bool { s, running := r.state(); return running && s.Control != "" })
	done, _ = r.run(uiFlags{stop: true})
	if err := finished(t, done); err != nil {
		t.Fatalf("--stop of a tab panel: %v", err)
	}
	if err := finished(t, first); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ides != 2 {
		t.Errorf("agents started: %d (--stop must start none)", r.ides)
	}
}

// What the state file says about a process proves nothing. --stop sends no
// signal to any pid: it asks the panel at its own address and believes it is
// gone only when the folder's lock is free. Here the pid in the file is this
// very test — a signal to it would end the test run.
func TestUI_Stop_NeverTrustsThePid(t *testing.T) {
	r := newUIRig(t)
	var asked atomic.Int32
	decoy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		w.WriteHeader(http.StatusNoContent) // says yes to anything
	}))
	defer decoy.Close()
	port := decoy.Listener.Addr().(*net.TCPAddr).Port
	stale := uiwindow.State{Cwd: r.dir, Port: port, PID: os.Getpid(), Mode: uiwindow.ModeTab, Control: "left-behind"}

	// A killed panel left its state; nobody holds the folder. Another
	// program listens on its old port, another process has its old pid.
	in, _, err := uiwindow.Acquire(r.deps.configDir, r.dir)
	if err != nil {
		t.Fatal(err)
	}
	file := in.StateFile()
	in.Release()
	data, _ := json.Marshal(stale)
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	done, _ := r.run(uiFlags{stop: true})
	if err := finished(t, done); err != nil {
		t.Fatalf("--stop over a stale state: %v", err)
	}
	if out := r.out.String(); !strings.Contains(out, "No panel is running for this folder.") {
		t.Errorf("output:\n%s", out)
	}
	if n := asked.Load(); n != 0 {
		t.Errorf("the program on the stale port was asked to stop (%d times)", n)
	}

	// Somebody does hold the folder, and the address in the state answers
	// yes — but the folder is not let go: that is not "stopped".
	holder, held, err := uiwindow.Acquire(r.deps.configDir, r.dir)
	if err != nil || !held {
		t.Fatalf("held=%v err=%v", held, err)
	}
	defer holder.Release()
	if err := holder.Write(stale); err != nil {
		t.Fatal(err)
	}
	r.deps.stopWait = 300 * time.Millisecond
	done, _ = r.run(uiFlags{stop: true})
	if err := finished(t, done); err == nil || !strings.Contains(err.Error(), "did not stop") || !strings.Contains(err.Error(), r.dir) {
		t.Fatalf("--stop of a panel that does not end: %v", err)
	}
	if asked.Load() == 0 {
		t.Error("the panel was never asked")
	}
	if strings.Contains(r.out.String(), "The panel is stopped: ") {
		t.Errorf("a panel that still holds its folder was called stopped:\n%s", r.out.String())
	}
	if again, held, _ := uiwindow.Acquire(r.deps.configDir, r.dir); held {
		t.Fatal("--stop took the folder from its holder")
	} else {
		again.Release()
	}
}

// A panel that does not stop is reported once — as the command's error.
func TestUI_StopAll_FailureIsSaidOnce(t *testing.T) {
	r := newUIRig(t)
	holder, held, err := uiwindow.Acquire(r.deps.configDir, r.dir)
	if err != nil || !held {
		t.Fatalf("held=%v err=%v", held, err)
	}
	defer holder.Release()
	if err := holder.Write(uiwindow.State{Cwd: r.dir, Port: r.ports[0], PID: os.Getpid(), Control: "x"}); err != nil {
		t.Fatal(err)
	}
	r.deps.stopWait = 200 * time.Millisecond
	done, _ := r.run(uiFlags{cwd: noFolder, stop: true, all: true})
	err = finished(t, done)
	if err == nil || strings.Count(err.Error(), r.dir) != 1 {
		t.Fatalf("--stop --all over a panel that does not stop: %v", err)
	}
	if strings.Contains(r.out.String(), r.dir) {
		t.Errorf("the failure is printed as well as returned:\n%s", r.out.String())
	}
}

// --stop --all: the panels of all folders.
func TestUI_StopAll(t *testing.T) {
	a := newUIRig(t)
	b := newUIRig(t)
	b.deps.configDir, b.deps.home = a.deps.configDir, a.deps.home
	doneA, _ := a.run(uiFlags{browser: true})
	a.wait("the tab", func() bool { return a.nTabs() == 1 })
	doneB, _ := b.run(uiFlags{})
	b.wait("the window", func() bool { return b.nStarts() == 1 })
	b.wait("the state file", func() bool { s, running := b.state(); return running && s.Control != "" })
	a.wait("the state file", func() bool { s, running := a.state(); return running && s.Control != "" })
	// A third folder whose panel ended long ago.
	gone := t.TempDir()
	in, _, err := uiwindow.Acquire(a.deps.configDir, gone)
	if err != nil {
		t.Fatal(err)
	}
	_ = in.Write(uiwindow.State{Cwd: gone, Port: uiwindow.PortBase + 7})
	in.Release()

	before := len(a.out.String())
	done, _ := a.run(uiFlags{cwd: noFolder, stop: true, all: true})
	if err := finished(t, done); err != nil {
		t.Fatalf("--stop --all: %v", err)
	}
	for name, d := range map[string]chan error{"a": doneA, "b": doneB} {
		select {
		case err := <-d:
			if err != nil {
				t.Errorf("panel %s: %v", name, err)
			}
		case <-time.After(time.Second):
			t.Errorf("panel %s still runs after --stop --all returned", name)
		}
	}
	out := a.out.String()[before:]
	if !strings.Contains(out, "The panel is stopped: "+a.dir) || !strings.Contains(out, "The panel is stopped: "+b.dir) || strings.Contains(out, gone) {
		t.Errorf("output:\n%s", out)
	}
	if !b.startN(0).proc.wasStopped() {
		t.Error("a window was left open")
	}
	before = len(a.out.String())
	done, _ = a.run(uiFlags{cwd: noFolder, stop: true, all: true})
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	if out := a.out.String()[before:]; !strings.Contains(out, "No panel is running.") {
		t.Errorf("output:\n%s", out)
	}
}

// A second execai ui for the same folder starts no server: it shows the
// window of the first one — the same address in the same profile — says so
// and ends with success. Another folder is another panel.
func TestUI_SecondRunShowsTheOpenWindow(t *testing.T) {
	r := newUIRig(t)
	first, cancel := r.run(uiFlags{})
	r.wait("the first window", func() bool { return r.nStarts() == 1 })
	r.wait("the state file", func() bool { s, _ := r.state(); return s.Mode == uiwindow.ModeWindow })
	state, _ := r.state()
	stable := "http://127.0.0.1:" + strconv.Itoa(state.Port) + "/"

	// The first one's window is not up yet: a second run leaves the browser
	// alone — two starts of one profile at once would race — and says the
	// panel is starting.
	r.deps.stateWait = 150 * time.Millisecond
	before := len(r.out.String())
	early, _ := r.run(uiFlags{})
	if err := finished(t, early); err != nil {
		t.Fatalf("a second run before the window is up: %v", err)
	}
	if r.nStarts() != 1 || r.nTabs() != 0 {
		t.Fatalf("a second run before the window is up touched the browser: starts %d, tabs %d", r.nStarts(), r.nTabs())
	}
	if out := r.out.String()[before:]; !strings.Contains(out, "already starting") {
		t.Errorf("a second run before the window is up said:\n%s", out)
	}
	r.deps.stateWait = 2 * time.Second
	r.windowComesUp(0)

	// The launcher of the second run hands the window to the running
	// browser and ends at once.
	r.mu.Lock()
	r.onStart = func(p *uiProc) { p.end(nil) }
	r.mu.Unlock()
	before = len(r.out.String())
	second, _ := r.run(uiFlags{})
	if err := finished(t, second); err != nil {
		t.Fatalf("the second run: %v", err)
	}
	if r.nStarts() != 2 {
		t.Fatalf("browser starts: %d", r.nStarts())
	}
	again := r.startN(1)
	if got := argOf(again.args, "--app="); got != stable {
		t.Errorf("the second run opens %s, want the address of the first one without a key %s", got, stable)
	}
	if argOf(again.args, "--user-data-dir=") != argOf(r.startN(0).args, "--user-data-dir=") || again.path != r.chrome.Path {
		t.Errorf("another profile or browser: %q", again.args)
	}
	if again.proc.wasStopped() {
		t.Error("the second run closed the window it asked for")
	}
	if out := r.out.String()[before:]; !strings.Contains(out, "already open") || strings.Contains(out, "Opening the ExecAI panel") {
		t.Errorf("the second run said:\n%s", out)
	}
	r.mu.Lock()
	if r.ides != 1 {
		t.Errorf("agents started: %d", r.ides)
	}
	r.mu.Unlock()
	stillRunning(t, first, 50*time.Millisecond)
	if s, running := r.state(); !running || s.Port != state.Port || s.PID != os.Getpid() {
		t.Errorf("the second run changed the state: %+v", s)
	}

	// --browser on a second run: the stable address in the default browser.
	// No code is printed for it: a code is made when it is asked for.
	before = len(r.out.String())
	third, _ := r.run(uiFlags{browser: true})
	if err := finished(t, third); err != nil {
		t.Fatal(err)
	}
	if r.nTabs() != 1 || r.tab(0) != stable || r.nStarts() != 2 {
		t.Fatalf("tabs %q, browser starts %d", r.tabs, r.nStarts())
	}
	if out := r.out.String()[before:]; strings.Contains(out, "One-time code") || !strings.Contains(out, "already open: "+stable) {
		t.Errorf("the second run with --browser said:\n%s", out)
	}

	// Another folder: its own server, address and window profile.
	other := newUIRig(t)
	other.deps.configDir, other.deps.home = r.deps.configDir, r.deps.home
	otherDone, otherCancel := other.run(uiFlags{})
	other.wait("the other folder's window", func() bool { return other.nStarts() == 1 })
	o := other.startN(0)
	if got := other.keyURL(0); strings.HasPrefix(got, stable) {
		t.Errorf("another folder got %s", got)
	}
	if argOf(o.args, "--user-data-dir=") == argOf(again.args, "--user-data-dir=") {
		t.Error("two folders share a window profile")
	}
	otherCancel()
	if err := finished(t, otherDone); err != nil {
		t.Fatal(err)
	}

	cancel()
	if err := finished(t, first); err != nil {
		t.Fatal(err)
	}
}

// The first one runs as a tab: the second run opens the stable address in
// the default browser.
func TestUI_SecondRun_Tab(t *testing.T) {
	r := newUIRig(t)
	first, cancel := r.run(uiFlags{browser: true})
	r.wait("the tab", func() bool { return r.nTabs() == 1 })
	r.wait("the state file", func() bool { s, _ := r.state(); return s.Mode == uiwindow.ModeTab })
	state, _ := r.state()
	second, _ := r.run(uiFlags{})
	if err := finished(t, second); err != nil {
		t.Fatal(err)
	}
	if r.nTabs() != 2 || r.tab(1) != "http://127.0.0.1:"+strconv.Itoa(state.Port)+"/" || r.nStarts() != 0 {
		t.Fatalf("tabs %q starts %d", r.tabs, r.nStarts())
	}
	if strings.Contains(r.out.String(), "One-time code") {
		t.Error("a code is printed for a tab of the browser that holds the session")
	}
	// --no-open on the second run only tells the address.
	third, _ := r.run(uiFlags{noOpen: true})
	if err := finished(t, third); err != nil {
		t.Fatal(err)
	}
	if r.nTabs() != 2 {
		t.Fatalf("--no-open opened a tab: %q", r.tabs)
	}
	cancel()
	if err := finished(t, first); err != nil {
		t.Fatal(err)
	}
}

// The browser can be named: EXECAI_UI_BROWSER, then ui_browser in
// config.json. One that is not on this computer (removed since, a mistyped
// path) is said and the search goes on.
func TestUI_NamedBrowser(t *testing.T) {
	r := newUIRig(t)
	r.cfg.UIBrowser = "edge"
	r.findErr["edge"] = uiwindow.ErrNotHere
	done, cancel := r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	if !slices.Equal(r.finds, []string{"edge", ""}) {
		t.Errorf("searches %q", r.finds)
	}
	r.finds = nil
	r.mu.Unlock()
	if out := r.out.String(); !strings.Contains(out, "(edge) is not on this computer") {
		t.Errorf("output:\n%s", out)
	}

	// The environment wins over the file.
	r.deps.getenv = func(k string) string {
		if k == "EXECAI_UI_BROWSER" {
			return "/opt/pw/chrome"
		}
		return screenEnv(k)
	}
	done, cancel = r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 2 })
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !slices.Equal(r.finds, []string{"/opt/pw/chrome"}) {
		t.Errorf("searches %q", r.finds)
	}
}

// execai ui --code asks the running panel of the folder for a one-time
// code and prints it with the address it is for. Until it is asked for
// there is no code anywhere — not in the terminal, not in the state file.
func TestUI_Code(t *testing.T) {
	r := newUIRig(t)
	done, _ := r.run(uiFlags{code: true})
	if err := finished(t, done); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("--code with no panel: %v", err)
	}
	first, cancel := r.run(uiFlags{noOpen: true})
	r.waitPrinted("ExecAI panel: ", 1)
	r.wait("the state file", func() bool { s, running := r.state(); return running && s.Control != "" })
	state, _ := r.state()
	stable := "http://127.0.0.1:" + strconv.Itoa(state.Port) + "/"
	login := func(code string) int {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"code": code})
		req, _ := http.NewRequest("POST", stable+"login", strings.NewReader(string(body)))
		req.Header.Set("Origin", strings.TrimSuffix(stable, "/"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := login("AAAA-AAAA"); got != http.StatusForbidden {
		t.Fatalf("a try before any code was asked for: %d", got)
	}

	before := len(r.out.String())
	done, _ = r.run(uiFlags{code: true})
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`One-time code for the page ` + regexp.QuoteMeta(stable) + `: ([A-Z2-9]{4}-[A-Z2-9]{4}) \(good for 5 min\)`).
		FindStringSubmatch(r.out.String()[before:])
	if m == nil {
		t.Fatalf("output:\n%s", r.out.String()[before:])
	}
	raw, _ := os.ReadFile(filepath.Join(r.deps.configDir, "ui", uiwindow.Key(r.dir)+".json"))
	if strings.Contains(string(raw), m[1]) || strings.Contains(string(raw), strings.ReplaceAll(m[1], "-", "")) {
		t.Errorf("the code is kept in the state file:\n%s", raw)
	}
	if got := login(m[1]); got != http.StatusNoContent {
		t.Fatalf("the code from --code: %d", got)
	}
	if got := login(m[1]); got != http.StatusForbidden {
		t.Fatalf("the code a second time: %d", got)
	}
	stillRunning(t, first, 50*time.Millisecond)
	cancel()
	if err := finished(t, first); err != nil {
		t.Fatal(err)
	}
}

// --last (what a launcher shortcut runs): the folder of the last run. When
// there has been none, there is no project — and the home folder is not
// made one: the command says how to name a folder, in its error and, for a
// shortcut that has no terminal, by a desktop notification.
func TestUI_Last(t *testing.T) {
	r := newUIRig(t)
	done, _ := r.run(uiFlags{last: true})
	err := finished(t, done)
	if err == nil || !strings.Contains(err.Error(), "no project folder is chosen") || !strings.Contains(err.Error(), "--install-shortcut --cwd") {
		t.Fatalf("--last with no folder: %v", err)
	}
	r.mu.Lock()
	if len(r.notices) != 1 || !strings.Contains(r.notices[0], "no project folder") || r.ides != 0 || len(r.starts) != 0 || len(r.tabs) != 0 {
		t.Errorf("notifications %q, agents %d, browser starts %d, tabs %d", r.notices, r.ides, len(r.starts), len(r.tabs))
	}
	r.mu.Unlock()
	if in, held, _ := uiwindow.Acquire(r.deps.configDir, r.deps.home); !held {
		t.Error("a panel runs in the home folder")
	} else {
		if st, _ := in.Read(); st.Port != 0 {
			t.Errorf("a panel ran in the home folder: %+v", st)
		}
		in.Release()
	}

	// A run in the project makes it the last folder.
	done, cancel := r.run(uiFlags{noOpen: true})
	r.waitPrinted("ExecAI panel: ", 1)
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	done, cancel = r.run(uiFlags{last: true, noOpen: true})
	r.waitPrinted("ExecAI panel: ", 2)
	if _, running := r.state(); !running {
		t.Fatal("--last did not open the folder of the last run")
	}
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	// The folder is gone: again no project.
	if err := os.RemoveAll(r.dir); err != nil {
		t.Fatal(err)
	}
	done, _ = r.run(uiFlags{last: true})
	if err := finished(t, done); err == nil || !strings.Contains(err.Error(), "no project folder is chosen") {
		t.Fatalf("--last after the folder is gone: %v", err)
	}
}

// A look at the folder's lock by another command (--code, --stop --all, the
// sweep take it for an instant) at the moment a panel starts does not make
// the panel think the folder is taken.
func TestUI_StartsThroughAMomentaryLock(t *testing.T) {
	r := newUIRig(t)
	probe, held, err := uiwindow.Acquire(r.deps.configDir, r.dir)
	if err != nil || !held {
		t.Fatalf("held=%v err=%v", held, err)
	}
	time.AfterFunc(150*time.Millisecond, probe.Release)
	done, cancel := r.run(uiFlags{noOpen: true})
	r.waitPrinted("ExecAI panel: ", 1)
	if strings.Contains(r.out.String(), "already") {
		t.Errorf("output:\n%s", r.out.String())
	}
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}

// What a shortcut cannot do is said in the person's language, like the rest.
func TestUI_ShortcutErrorsAreTranslated(t *testing.T) {
	r := newUIRig(t)
	i18n.SetLocale("ru")
	foreign := filepath.Join(r.deps.home, ".local/share/applications/execai-ui.desktop")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("[Desktop Entry]\nName=Somebody else's\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range []uiFlags{{cwd: noFolder, installShortcut: true}, {cwd: noFolder, removeShortcut: true}} {
		done, _ := r.run(f)
		err := finished(t, done)
		if err == nil || !strings.Contains(err.Error(), foreign) || !strings.Contains(err.Error(), "не значок") ||
			strings.Contains(err.Error(), "not a shortcut") {
			t.Errorf("%+v over somebody else's file: %v", f, err)
		}
	}
	if b, _ := os.ReadFile(foreign); !strings.Contains(string(b), "Somebody else's") {
		t.Error("somebody else's file was touched")
	}
	r.deps.home = ""
	done, _ := r.run(uiFlags{cwd: noFolder, installShortcut: true})
	if err := finished(t, done); err == nil || !strings.Contains(err.Error(), "нет домашней папки") {
		t.Errorf("without a home folder: %v", err)
	}
	r.deps.home, r.deps.goos = "/home/u", "plan9"
	done, _ = r.run(uiFlags{cwd: noFolder, installShortcut: true})
	if err := finished(t, done); err == nil || !strings.Contains(err.Error(), "значка запуска нет") {
		t.Errorf("on a system without shortcuts: %v", err)
	}
}

// --install-shortcut and --remove-shortcut; without a suitable browser the
// person is told what the shortcut will do.
func TestUI_Shortcut(t *testing.T) {
	r := newUIRig(t)
	desktop := filepath.Join(r.deps.home, ".local/share/applications/execai-ui.desktop")
	done, _ := r.run(uiFlags{cwd: noFolder, installShortcut: true})
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(desktop)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `Exec="/usr/local/bin/execai" ui --last`+"\n") || !strings.Contains(string(data), "Comment=The ExecAI agent panel\n") {
		t.Errorf("desktop entry:\n%s", data)
	}
	if out := r.out.String(); !strings.Contains(out, "Launcher shortcut created: "+desktop) || strings.Contains(out, "opens a tab") {
		t.Errorf("output:\n%s", out)
	}
	// No panel has run yet: the shortcut has no folder to open, and says so.
	if out := r.out.String(); !strings.Contains(out, "There is none yet") {
		t.Errorf("output:\n%s", out)
	}

	// For a project, on a computer without a suitable browser.
	r.findErr[""] = uiwindow.ErrNoBrowser
	done, _ = r.run(uiFlags{installShortcut: true})
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(r.deps.home, ".local/share/applications/execai-ui-"+uiwindow.Key(r.dir)[:8]+".desktop")
	data, err = os.ReadFile(project)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), ` ui --cwd "`+r.dir+`"`) {
		t.Errorf("desktop entry:\n%s", data)
	}
	if out := r.out.String(); !strings.Contains(out, "Without one the shortcut opens a tab") {
		t.Errorf("output:\n%s", out)
	}

	done, _ = r.run(uiFlags{removeShortcut: true})
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(project); !os.IsNotExist(err) {
		t.Error("the project's shortcut is still there")
	}
	if _, err := os.Stat(desktop); err != nil {
		t.Error("removing a project's shortcut took the general one")
	}
	done, _ = r.run(uiFlags{cwd: noFolder, removeShortcut: true})
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(desktop); !os.IsNotExist(err) {
		t.Error("the shortcut is still there")
	}
	done, _ = r.run(uiFlags{cwd: noFolder, removeShortcut: true})
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	if out := r.out.String(); !strings.Contains(out, "Launcher shortcut removed: ") || !strings.Contains(out, "No launcher shortcut: ") {
		t.Errorf("output:\n%s", out)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ides != 0 {
		t.Fatal("the panel started")
	}
}

// The one-time key is in no argument of the browser: arguments are open to
// every program on the computer. It is in a file only the person can read,
// in the window's own profile directory, and the file is gone as soon as
// the key is used — or the panel stops, or the browser never comes for it.
func TestUI_TheKeyIsInNoArgument(t *testing.T) {
	hex64 := regexp.MustCompile(`[0-9a-f]{64}`)
	check := func(r *uiRig, i int) (key, jump string) {
		t.Helper()
		st := r.startN(i)
		key = r.keyURL(i)
		secret := key[strings.Index(key, "?t=")+3:]
		for _, a := range append([]string{st.path}, st.args...) {
			if strings.Contains(a, secret) || strings.Contains(a, "?t=") || hex64.MatchString(a) {
				t.Errorf("the key is in the command line: %q", a)
			}
		}
		jump = r.jumpFile(i)
		fi, err := os.Stat(jump)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("the file with the key is %v", fi.Mode().Perm())
		}
		if di, _ := os.Stat(filepath.Dir(jump)); di.Mode().Perm() != 0o700 {
			t.Errorf("the window's profile directory is %v", di.Mode().Perm())
		}
		if filepath.Dir(jump) != argOf(st.args, "--user-data-dir=") {
			t.Errorf("the file %s is outside the window's profile", jump)
		}
		return key, jump
	}
	gone := func(r *uiRig, jump, when string) {
		t.Helper()
		r.wait("the file with the key to go "+when, func() bool {
			_, err := os.Lstat(jump)
			return os.IsNotExist(err)
		})
	}

	// Used: gone at once — before the browser has its answer.
	r := newUIRig(t)
	done, cancel := r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	key, jump := check(r, 0)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the key: %d", resp.StatusCode)
	}
	if _, err := os.Lstat(jump); !os.IsNotExist(err) {
		t.Error("the file with the key outlived its use")
	}
	if strings.Contains(r.out.String(), key) || strings.Contains(r.out.String(), "?t=") {
		t.Errorf("the key is in the terminal:\n%s", r.out.String())
	}
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}

	// Never used, the panel stops: gone.
	r = newUIRig(t)
	done, cancel = r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	_, jump = check(r, 0)
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
	gone(r, jump, "with the panel")

	// Never used, the browser hangs: gone after a while.
	r = newUIRig(t)
	r.deps.timing.Jump = 80 * time.Millisecond
	done, cancel = r.run(uiFlags{})
	r.wait("the browser start", func() bool { return r.nStarts() == 1 })
	jump = r.jumpFile(0)
	gone(r, jump, "after its time")
	stillRunning(t, done, 50*time.Millisecond)
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}

// Somebody else used the key first; then the person's window comes with
// it. The server throws everybody out; the panel says so, closes the window
// and opens it anew with a new key — again in a file, not in an argument.
// After a few such alarms it stops opening windows and names the code.
func TestUI_KeyInOtherHands_WindowIsOpenedAnew(t *testing.T) {
	r := newUIRig(t)
	done, cancel := r.run(uiFlags{})
	get := func(key string) (*http.Response, http.CookieJar) {
		t.Helper()
		jar, _ := cookiejar.New(nil)
		resp, err := (&http.Client{Jar: jar}).Get(key)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp, jar
	}
	var keys []string
	for round := 0; round <= uiMaxReopen; round++ {
		r.wait("browser start "+strconv.Itoa(round+1), func() bool { return r.nStarts() == round+1 })
		key := r.keyURL(round)
		if slices.Contains(keys, key) {
			t.Fatalf("round %d: the key was not replaced", round)
		}
		keys = append(keys, key)
		first, thief := get(key)
		if first.StatusCode != http.StatusOK {
			t.Fatalf("round %d: the first comer: %d", round, first.StatusCode)
		}
		second, _ := get(key) // the person's own window
		if second.StatusCode != http.StatusForbidden {
			t.Fatalf("round %d: the second comer: %d", round, second.StatusCode)
		}
		// The first comer is out.
		req, _ := http.NewRequest("GET", strings.SplitN(key, "?", 2)[0], nil)
		resp, err := (&http.Client{Jar: thief}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("round %d: the first comer still has the page: %d", round, resp.StatusCode)
		}
		r.wait("the old window to be closed", func() bool { return r.startN(round).proc.wasStopped() })
		r.waitPrinted("Two came with the panel's one-time link", round+1)
	}
	if n := strings.Count(r.out.String(), "running turns are stopped and waiting permission requests refused"); n != uiMaxReopen+1 {
		t.Errorf("the alarm says what was done to the agent %d times:\n%s", n, r.out.String())
	}
	out := r.out.String()
	if n := strings.Count(out, "Opening the window anew"); n != uiMaxReopen {
		t.Errorf("the window was opened anew %d times, want %d:\n%s", n, uiMaxReopen, out)
	}
	if n := strings.Count(out, "run execai ui --code"); n != 1 {
		t.Errorf("the code is named %d times:\n%s", n, out)
	}
	time.Sleep(100 * time.Millisecond)
	if r.nStarts() != uiMaxReopen+1 || r.nTabs() != 0 {
		t.Errorf("browser starts %d, tabs %d", r.nStarts(), r.nTabs())
	}
	for _, k := range keys {
		if strings.Contains(out, k) {
			t.Error("a key is in the terminal")
		}
	}
	stillRunning(t, done, 50*time.Millisecond)
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}

// A tab's link goes to the system's opener as an argument: after an alarm
// it is not handed out again — the person signs in by the code.
func TestUI_KeyInOtherHands_Tab(t *testing.T) {
	r := newUIRig(t)
	done, cancel := r.run(uiFlags{browser: true})
	r.wait("the tab", func() bool { return r.nTabs() == 1 })
	for i := 0; i < 2; i++ {
		resp, err := http.Get(r.tab(0))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	r.waitPrinted("Two came with the panel's one-time link", 1)
	r.waitPrinted("run execai ui --code", 1)
	time.Sleep(100 * time.Millisecond)
	if r.nTabs() != 1 || r.nStarts() != 0 {
		t.Errorf("tabs %d, browser starts %d after the alarm", r.nTabs(), r.nStarts())
	}
	if out := r.out.String(); strings.Contains(out, "Opening the window anew") || strings.Count(out, "?t=") != 1 {
		t.Errorf("output:\n%s", out)
	}
	cancel()
	if err := finished(t, done); err != nil {
		t.Fatal(err)
	}
}
