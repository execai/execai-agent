package main

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/uiserve"
	"github.com/velesbsdllc/agent-vbai/internal/uiwindow"
)

// uiMaxReopen — how many times the window is opened anew after an alarm
// about the one-time key; after that the person signs in by the code.
const uiMaxReopen = 2

// uiPanel — one running execai ui: what shows its page and what the state
// file says about it.
type uiPanel struct {
	out    io.Writer
	d      uiDeps
	inst   *uiwindow.Instance
	cancel func() // stops the panel
	show   uiShow // how the page is shown
	// noScreen — nothing is opened because there is no graphical session,
	// not because the person asked for nothing: they are told.
	noScreen bool

	mu     sync.Mutex
	state  uiwindow.State
	server *uiserve.Server
	win    *uiWin // the window this panel opened and watches; nil — none
	alarms int    // alarms about the one-time key in this run
	failed error  // why the panel ends with an error

	closedOnce sync.Once
}

// fail ends the panel with an error (--window, and no window came up).
func (p *uiPanel) fail(err error) {
	p.mu.Lock()
	if p.failed == nil {
		p.failed = err
	}
	p.mu.Unlock()
	p.cancel()
}

// failure — the error the panel ends with, if any.
func (p *uiPanel) failure() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.failed
}

// uiWin — one app window the panel started.
type uiWin struct {
	browser uiwindow.Browser
	watch   *uiwindow.Watch
	// jump — the file that carries the one-time key to the window; removed
	// when the key is used, after a while, and with the window.
	jump  string
	timer *time.Timer
	// silent — fires when the browser runs and shows nothing for too long.
	silent *time.Timer
	// done — closed when nobody watches the window any more: it is closed,
	// it never opened, another one took its place, or the panel stops.
	done chan struct{}
	once sync.Once
}

// end — the window is no longer the panel's concern: its file with the key
// goes, the watch for its page ends. The browser itself is left alone.
func (w *uiWin) end() {
	w.once.Do(func() {
		for _, t := range []*time.Timer{w.timer, w.silent} {
			if t != nil {
				t.Stop()
			}
		}
		uiwindow.RemoveJump(w.jump)
		close(w.done)
	})
}

// started — the server listens: show the page and write the state file.
func (p *uiPanel) started(s *uiserve.Server, show uiShow, browser *uiwindow.Browser) {
	p.mu.Lock()
	p.server = s
	p.state.Port = s.Port()
	p.state.Control = s.Control()
	p.mu.Unlock()

	switch {
	case show == showNone:
		if p.noScreen {
			fmt.Fprintln(p.out, i18n.T("ui.noScreen"))
		}
		fmt.Fprintln(p.out, i18n.Tf("ui.address", s.URL()))
		fmt.Fprintln(p.out, i18n.T("ui.hint"))
	case browser != nil:
		p.openWindow(s, *browser)
	default:
		if show == showAuto {
			fmt.Fprintln(p.out, i18n.Tf("ui.window.tabFallback", uiwindow.Supported(p.d.goos)))
		}
		p.openTab(s)
	}
	p.save()
}

// openWindow starts the browser with the page as an app window and watches
// it; a browser that cannot be started gives way to a tab.
//
// The address with the one-time key is not an argument of the browser:
// arguments can be read by every program on the computer. The window is
// started with a file of the person's own (uiwindow.WriteJump) that sends
// it on to that address.
func (p *uiPanel) openWindow(s *uiserve.Server, b uiwindow.Browser) {
	profile := uiwindow.ProfileDir(p.d.configDir, p.d.home, b, p.inst.Key(), p.d.goos)
	jump, err := uiwindow.WriteJump(profile, s.URL())
	var proc uiwindow.Proc
	if err == nil {
		if proc, err = uiwindow.Open(p.d.start, b, profile, uiwindow.FileURL(jump, p.d.goos), p.d.goos); err != nil {
			uiwindow.RemoveJump(jump)
		}
	}
	if err != nil {
		p.noWindow(s, b, err)
		return
	}
	// The key is the window's: what the terminal shows is the address that
	// stays.
	fmt.Fprintln(p.out, i18n.Tf("ui.window.opened", b.Name, s.StableURL()))
	fmt.Fprintln(p.out, i18n.T("ui.window.hint"))
	win := &uiWin{browser: b, jump: jump, done: make(chan struct{}),
		watch: uiwindow.NewWatch(proc, s.TokenUsed, p.d.timing)}
	// A browser that never fetches the file must not leave the key lying.
	win.timer = time.AfterFunc(orTiming(p.d.timing.Jump, uiwindow.DefaultTiming.Jump), func() { uiwindow.RemoveJump(win.jump) })
	// A browser that runs and shows nothing — a window that says the
	// profile is in use, a first-run screen of a browser that has one after
	// all — leaves the person with no panel and no word. After a while they
	// are told the other ways in.
	win.silent = time.AfterFunc(orTiming(p.d.timing.Silent, uiwindow.DefaultTiming.Silent), func() {
		if p.current(win) && !s.TokenUsed() {
			fmt.Fprintln(p.out, i18n.Tf("ui.window.silent", b.Name))
			fmt.Fprintln(p.out, i18n.Tf("ui.address", s.URL()))
		}
	})
	p.mu.Lock()
	p.state.Mode, p.state.Browser, p.state.Profile, p.state.WindowUp = uiwindow.ModeWindow, &b, profile, false
	p.win = win
	p.mu.Unlock()
	go p.follow(win, s)
	if p.d.goos == "darwin" {
		// macOS keeps an application running after its last window is
		// closed: there the end of the browser does not tell.
		p.watchPage(win, s)
	}
}

func orTiming(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

// watchPage — the window is known to be closed by its page: it said
// goodbye and stays away. Used only where the end of the browser cannot
// tell (macOS; a window in a browser the panel did not start). While the
// panel's own browser runs, a page that is merely gone — it crashed, the
// window still shows the browser's sad page — stops nothing: a turn may be
// running, and the person reloads.
func (p *uiPanel) watchPage(win *uiWin, s *uiserve.Server) {
	go func() {
		select {
		case <-uiwindow.Gone(s.PageLeft, p.d.timing, win.done):
			if p.current(win) {
				p.windowClosed()
			}
		case <-win.done:
		}
	}()
}

// noWindow — the browser did not give a window. For a person who asked for
// a window and nothing else (--window) that is an error; otherwise the page
// goes to a tab, and a tab is the person's to close without stopping
// anything.
func (p *uiPanel) noWindow(s *uiserve.Server, b uiwindow.Browser, why error) {
	if why == nil {
		why = errors.New("exit status 0")
	}
	if p.show == showWindow {
		p.fail(errors.New(i18n.Tf("ui.window.failed", b.Name, why)))
		return
	}
	fmt.Fprintln(p.out, i18n.Tf("ui.window.startFailed", b.Name, why))
	p.mu.Lock()
	p.state.Browser, p.state.Profile, p.state.WindowUp = nil, "", false
	p.mu.Unlock()
	p.openTab(s)
}

// signedIn — a browser came in with the one-time key: the file that
// carried it has done its work.
func (p *uiPanel) signedIn() {
	p.mu.Lock()
	win := p.win
	if win != nil {
		// The window is up: a second run may now ask its browser for it.
		p.state.WindowUp = true
	}
	p.mu.Unlock()
	if win != nil {
		uiwindow.RemoveJump(win.jump)
		p.save()
	}
}

// intruder — the server found that the one-time key was in other hands and
// has thrown everybody out. The person is told; a window is opened anew
// (its key travels in a file nobody else can read). A tab is not: its link
// is handed to the system's opener as an argument, where it was read once
// already — there the way back in is the one-time code.
func (p *uiPanel) intruder(why uiserve.Intrusion) {
	line := i18n.T("ui.alarm.usedTwice")
	if why == uiserve.KeyUsedNoPage {
		line = i18n.T("ui.alarm.noPage")
	}
	p.mu.Lock()
	p.alarms++
	n, win, s := p.alarms, p.win, p.server
	p.win = nil
	p.state.WindowUp = false
	p.mu.Unlock()
	fmt.Fprintln(p.out, line)
	if win == nil || n > uiMaxReopen {
		fmt.Fprintln(p.out, i18n.T("ui.alarm.useCode"))
	} else {
		fmt.Fprintln(p.out, i18n.T("ui.alarm.reopen"))
	}
	if win == nil {
		return
	}
	// Off the server's goroutine: the browser being closed may be the very
	// one whose request rang the alarm.
	go func() {
		win.end()
		win.watch.Stop()
		if n <= uiMaxReopen {
			p.openWindow(s, win.browser)
		} else {
			p.mu.Lock()
			p.state.Mode, p.state.Browser, p.state.Profile = uiwindow.ModeNone, nil, ""
			p.mu.Unlock()
		}
		p.save()
	}()
}

// windowClosed — the person closed the window: the panel stops.
func (p *uiPanel) windowClosed() {
	p.closedOnce.Do(func() {
		fmt.Fprintln(p.out, i18n.T("ui.window.closed"))
		p.cancel()
	})
}

// current — win is the window the panel watches now.
func (p *uiPanel) current(win *uiWin) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.win == win
}

// follow — what the end of the window's browser means for the panel.
func (p *uiPanel) follow(win *uiWin, s *uiserve.Server) {
	o := <-win.watch.Done()
	if !p.current(win) {
		return // another window took its place
	}
	switch o.Kind {
	case uiwindow.Closed:
		p.windowClosed()
	case uiwindow.Failed:
		// The browser never showed the page: the key is still good.
		win.end()
		p.mu.Lock()
		p.win = nil
		p.mu.Unlock()
		p.noWindow(s, win.browser, o.Err)
		p.save()
	case uiwindow.Handed:
		// The window lives in a browser that was already running with this
		// profile (left by a panel that was killed). It is not a process of
		// ours: its closing is known by its page, and when the panel stops
		// the page is told to close the window (uiserve, sayClose).
		p.watchPage(win, s)
	}
}

// openTab — the page as a tab of the default browser, with the link in the
// terminal for when the browser does not come up.
func (p *uiPanel) openTab(s *uiserve.Server) {
	fmt.Fprintln(p.out, i18n.Tf("ui.address", s.URL()))
	fmt.Fprintln(p.out, i18n.T("ui.hint"))
	p.mu.Lock()
	p.state.Mode = uiwindow.ModeTab
	p.mu.Unlock()
	if err := p.d.openTab(s.URL()); err != nil {
		fmt.Fprintln(p.out, i18n.Tf("ui.openFailed", err))
	}
}

// save writes the state file.
func (p *uiPanel) save() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state.Port != 0 {
		_ = p.inst.Write(p.state)
	}
}

// askedToStop — execai ui --stop asked this panel to end: the same as
// Ctrl+C in its terminal.
func (p *uiPanel) askedToStop() {
	fmt.Fprintln(p.out, i18n.T("ui.stop.asked"))
	p.cancel()
}

// stop — the panel ends (Ctrl+C, the agent stopped, the window closed): the
// window's browser goes with it.
func (p *uiPanel) stop() {
	p.mu.Lock()
	win := p.win
	p.win = nil
	p.mu.Unlock()
	if win != nil {
		win.end()
		win.watch.Stop()
	}
}
