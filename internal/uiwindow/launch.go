package uiwindow

import (
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

// WindowClass — the X11/Wayland class of the window; the launcher shortcut
// names it (StartupWMClass), so the desktop shows the window under the
// shortcut's icon.
const WindowClass = "execai-ui"

// Args — the command line that shows url as an app window with its own
// profile. The profile makes the window a browser process of its own: it
// does not join the person's running browser (so its end is the window's
// end), has none of their extensions and gets its own button on the taskbar.
//
// The switches after the first three keep that profile small and quiet: it
// only ever shows a page of this computer, so the browser's own downloads
// (component updates, prediction models, lists for sites it will not visit)
// are left out — with them the profile grows to 150–200 MB in minutes, each
// project having one. Links in the panel leave for the person's own browser
// (uiserve, POST /open), so nothing else is loaded in this window.
func Args(b Browser, profile, url, goos string) []string {
	args := []string{
		"--user-data-dir=" + profile,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-mode", // the process ends with its last window
		"--disable-component-update",
		"--disable-background-networking",
		"--disable-features=OptimizationGuideModelDownloading,OptimizationHints,OptimizationHintsFetching,OptimizationTargetPrediction",
	}
	if goos == "linux" {
		args = append(args, "--class="+WindowClass)
	}
	return append(args, "--app="+url)
}

// ProfileDir — where the window's browser profile lives: in the agent's
// config directory, one per browser (their profiles are not interchangeable)
// and per project (key — the instance key; each project's window is its own
// process). A browser in a sandbox cannot write there, so its profile goes
// where the sandbox lets it. Sync never looks into either place: it takes
// the named settings files and the memory folder only.
func ProfileDir(configDir, home string, b Browser, key, goos string) string {
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	switch b.Kind {
	case KindSnap:
		return strings.Join([]string{home, "snap", b.Sandbox, "common", "execai-ui-window", key}, sep)
	case KindFlatpak:
		return strings.Join([]string{home, ".var", "app", b.Sandbox, "execai-ui-window", key}, sep)
	}
	return strings.Join([]string{strings.TrimRight(configDir, `/\`), "ui-window", slug(b.ID) + "-" + key}, sep)
}

// slug — a name safe as a part of a file name.
func slug(s string) string {
	var out strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			dash = false
		case !dash && out.Len() > 0:
			out.WriteByte('-')
			dash = true
		}
	}
	if res := strings.TrimRight(out.String(), "-"); res != "" {
		return res
	}
	return "browser"
}

// Proc — a started browser.
type Proc interface {
	// Wait blocks until the process has ended and returns how it ended.
	Wait() error
	// Stop asks the browser to close and makes sure it has ended.
	Stop()
	Pid() int
}

// Starter starts a program; tests give one that starts nothing.
type Starter func(path string, args []string) (Proc, error)

// Open starts the browser with url as an app window.
func Open(start Starter, b Browser, profile, url, goos string) (Proc, error) {
	return start(b.Path, Args(b, profile, url, goos))
}

// stopWait — how long a browser asked to close may take before it is killed.
const stopWait = 3 * time.Second

type osProc struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

// StartProcess — the Starter of the real system. The browser's own output
// is dropped: it writes a lot there and none of it is for the person.
func StartProcess(path string, args []string) (Proc, error) {
	cmd := exec.Command(path, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &osProc{cmd: cmd, done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

func (p *osProc) Wait() error {
	<-p.done
	return p.err
}

func (p *osProc) Pid() int { return p.cmd.Process.Pid }

func (p *osProc) Stop() {
	select {
	case <-p.done:
		return
	default:
	}
	askToClose(p.cmd.Process)
	select {
	case <-p.done:
		return
	case <-time.After(stopWait):
	}
	_ = p.cmd.Process.Kill()
	select {
	case <-p.done:
	case <-time.After(stopWait):
	}
}

// Kind — how a watched window ended.
type Kind int

const (
	// Closed — the window showed the page and the browser has ended: the
	// person closed the window.
	Closed Kind = iota + 1
	// Failed — the browser ended without ever showing the page.
	Failed
	// Handed — the started program ended at once but the page is shown: a
	// browser already running with this profile took the window. Its end
	// is not seen from here; Gone tells when that window is closed.
	Handed
	// Stopped — Stop was called.
	Stopped
)

// Outcome — what Watch found out.
type Outcome struct {
	Kind Kind
	Err  error // how the process ended, for Failed
}

// Timing — the moments Watch tells the cases apart by.
type Timing struct {
	// Quick — a process that ends sooner than this after its start was not
	// closed by a person.
	Quick time.Duration
	// Grace — how long after the process ended the page may still be
	// fetched by the browser that took the window over.
	Grace time.Duration
	// Gone — a page that said goodbye and has been away this long is a
	// closed window (Gone). A reload is away for a second or two.
	Gone time.Duration
	// Jump — how long the file that carries the one-time key to the window
	// may wait for a browser that does not fetch it.
	Jump time.Duration
	// Silent — a browser that runs this long without fetching the page is
	// showing something else; the person is told the other ways in.
	Silent time.Duration
	Poll   time.Duration
	// Leave — when the panel stops, how long the browser is given to close
	// its window and end by itself before it is asked to (Stop).
	Leave time.Duration
}

// DefaultTiming — for real browsers.
var DefaultTiming = Timing{Quick: 2 * time.Second, Grace: 4 * time.Second, Gone: 10 * time.Second,
	Jump: time.Minute, Silent: 20 * time.Second, Poll: 100 * time.Millisecond, Leave: 1500 * time.Millisecond}

// Watch waits for the end of the window's browser and says what it meant.
type Watch struct {
	proc     Proc
	done     chan Outcome
	stopping atomic.Bool
	// ended — closed when the browser's process has ended.
	ended chan struct{}
	leave time.Duration
}

// NewWatch starts watching p. served reports whether a browser has fetched
// the page with the one-time link: that, not the exit code, tells a window
// that was shown and closed from a browser that never came up.
func NewWatch(p Proc, served func() bool, t Timing) *Watch {
	w := &Watch{proc: p, done: make(chan Outcome, 1), ended: make(chan struct{}), leave: t.Leave}
	started := time.Now()
	go func() {
		err := p.Wait()
		close(w.ended)
		quick := time.Since(started) < t.Quick
		was := served()
		if !was {
			deadline := time.Now().Add(t.Grace)
			for !w.stopping.Load() && !served() && time.Now().Before(deadline) {
				time.Sleep(t.Poll)
			}
		}
		switch {
		case w.stopping.Load():
			w.done <- Outcome{Kind: Stopped}
		case !served():
			w.done <- Outcome{Kind: Failed, Err: err}
		case quick || !was:
			w.done <- Outcome{Kind: Handed}
		default:
			w.done <- Outcome{Kind: Closed}
		}
	}()
	return w
}

// Done delivers the one outcome.
func (w *Watch) Done() <-chan Outcome { return w.done }

// Stop closes the window (Ctrl+C in the terminal); the outcome is Stopped.
//
// The page has just been told to close its window (uiserve, sayClose), and
// a browser that closes its last window by itself writes what the page
// keeps — the look the person chose, the side panel put away — to disk on
// its way out. Asked to end at once, it did not: a look chosen within the
// last minute was gone at the next start. So the browser gets a moment
// (Timing.Leave) to go by itself; one that stays — a window that does not
// close, macOS, where a browser outlives its windows — is asked as before.
func (w *Watch) Stop() {
	w.stopping.Store(true)
	if w.leave > 0 {
		select {
		case <-w.ended:
			return
		case <-time.After(w.leave):
		}
	}
	w.proc.Stop()
}

// Gone tells that the window is closed by its page, for the cases where
// the end of the browser cannot: macOS keeps an application running after
// its last window is closed, and a window taken over by a browser that was
// already running (Handed) is not a process of ours at all.
//
// left — no page is connected, and the last one said goodbye when it went
// (uiserve.Server.PageLeft): it was closed or reloaded. A page that only
// vanished has crashed — the window is still open, showing the browser's
// own sad page — and a page that never came has nothing to close; neither
// stops anything. The returned channel is closed once left has held for
// t.Gone (a reload is back sooner); never after stop is closed.
func Gone(left func() bool, t Timing, stop <-chan struct{}) <-chan struct{} {
	gone := make(chan struct{})
	go func() {
		tick := time.NewTicker(t.Poll)
		defer tick.Stop()
		var since time.Time // since when left holds
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			switch {
			case !left():
				since = time.Time{}
			case since.IsZero():
				since = time.Now()
			case time.Since(since) >= t.Gone:
				close(gone)
				return
			}
		}
	}()
	return gone
}
