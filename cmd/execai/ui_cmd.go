package main

// execai ui — the editor's chat panel on 127.0.0.1: the ide server runs in
// this process, internal/uiserve carries its protocol over HTTP (SSE out,
// POST in). Tasks from the site and the plan gate are the ide server's own,
// so this window takes the inbox like an editor window.
//
// The page is shown as a window of its own when a Chromium-family browser
// is installed (internal/uiwindow: app mode, a profile of its own), and as a
// tab of the default browser otherwise. One panel per project folder: a
// second run shows the window that is already open.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/velesbsdllc/agent-vbai/internal/auth"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/ide"
	"github.com/velesbsdllc/agent-vbai/internal/notify"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
	"github.com/velesbsdllc/agent-vbai/internal/uiserve"
	"github.com/velesbsdllc/agent-vbai/internal/uiwindow"
	agentver "github.com/velesbsdllc/agent-vbai/internal/version"
)

// uiBrowserEnv — names the browser of the window for this process; it wins
// over ui_browser in config.json.
const uiBrowserEnv = "EXECAI_UI_BROWSER"

// uiFlags — the flags of execai ui.
type uiFlags struct {
	cwd  string
	port int
	// How the page is shown; none set — a window, or a tab without a
	// suitable browser.
	window  bool // a window only
	browser bool // a tab only
	noOpen  bool // nothing: the address is printed
	// last — the folder of the last run (the home folder when there was
	// none) in place of the current one: launcher shortcuts start in no
	// folder of their own.
	last bool
	// Other things to do in place of running the panel.
	code            bool
	installShortcut bool
	removeShortcut  bool
	stop            bool
	all             bool // with stop: the panels of all folders
}

// uiDeps — the computer as execai ui sees it; tests put functions here that
// start no browser and open no tab.
type uiDeps struct {
	goos      string
	home      string
	configDir string
	getenv    func(string) string
	// exe — the path of this program, for a shortcut.
	exe func() (string, error)
	// find — the browser for the window (uiwindow.Find over the real system).
	find      func(explicit string) (uiwindow.Browser, error)
	start     uiwindow.Starter
	openTab   func(url string) error
	runScript func(argv, env []string) error
	// ide — the agent behind the page.
	ide    func(cfg *config.Config, dir string, notices *ide.SyncNotices) uiserve.IDE
	timing uiwindow.Timing
	// stateWait — how long a second run waits for the first one, still
	// starting, to say where it listens.
	stateWait time.Duration
	// stopWait — how long --stop waits for the panel it asked to end.
	stopWait time.Duration
	// ports — the ports a run without --port tries for a folder
	// (uiwindow.Ports; tests listen elsewhere).
	ports func(key string) []int
	// notify — a desktop notification: what a launcher shortcut, which has
	// no terminal, can still tell the person.
	notify func(title, body string)
}

func realUIDeps() (uiDeps, error) {
	dir, err := config.Dir()
	if err != nil {
		return uiDeps{}, err
	}
	sys := uiwindow.RealSystem()
	return uiDeps{
		goos:      runtime.GOOS,
		home:      sys.Home,
		configDir: dir,
		getenv:    os.Getenv,
		exe: func() (string, error) {
			exe, err := os.Executable()
			if err != nil {
				return "", err
			}
			if real, err := filepath.EvalSymlinks(exe); err == nil {
				exe = real
			}
			return exe, nil
		},
		find:    func(explicit string) (uiwindow.Browser, error) { return uiwindow.Find(sys, explicit) },
		start:   uiwindow.StartProcess,
		openTab: auth.OpenBrowser,
		runScript: func(argv, env []string) error {
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Env = append(os.Environ(), env...)
			return cmd.Run()
		},
		ide:       uiIDE,
		timing:    uiwindow.DefaultTiming,
		stateWait: 5 * time.Second,
		stopWait:  10 * time.Second,
		ports:     uiwindow.Ports,
		notify: func(title, body string) {
			notify.NewDesktop(notify.System()).Send(notify.Message{Title: title, Body: body})
		},
	}, nil
}

func newUICmd() *cobra.Command {
	// Help texts in the user's language; the global locale is put back.
	prev := i18n.Locale()
	setCLILocale()
	defer i18n.SetLocale(prev)

	var f uiFlags
	cmd := &cobra.Command{
		Use:   "ui",
		Short: i18n.T("ui.cmd.short"),
		Long:  i18n.T("ui.cmd.long"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			i18n.Setup(cfg.Locale)
			deps, err := realUIDeps()
			if err != nil {
				return err
			}
			return runUI(cmd.Context(), cmd.OutOrStdout(), cfg, f, deps)
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.cwd, "cwd", "", i18n.T("ui.flag.cwd"))
	fl.IntVar(&f.port, "port", 0, i18n.T("ui.flag.port"))
	fl.BoolVar(&f.window, "window", false, i18n.T("ui.flag.window"))
	fl.BoolVar(&f.browser, "browser", false, i18n.T("ui.flag.browser"))
	fl.BoolVar(&f.noOpen, "no-open", false, i18n.T("ui.flag.noOpen"))
	fl.BoolVar(&f.last, "last", false, i18n.T("ui.flag.last"))
	fl.BoolVar(&f.code, "code", false, i18n.T("ui.flag.code"))
	fl.BoolVar(&f.installShortcut, "install-shortcut", false, i18n.T("ui.flag.installShortcut"))
	fl.BoolVar(&f.removeShortcut, "remove-shortcut", false, i18n.T("ui.flag.removeShortcut"))
	fl.BoolVar(&f.stop, "stop", false, i18n.T("ui.flag.stop"))
	fl.BoolVar(&f.all, "all", false, i18n.T("ui.flag.all"))
	return cmd
}

// uiShow — how the page is shown.
type uiShow int

const (
	showAuto   uiShow = iota // a window; a tab without a suitable browser
	showWindow               // a window, or an error
	showTab                  // a tab of the default browser
	showNone                 // nothing is opened
)

// show reads the flags; flags that contradict each other are an error.
func (f uiFlags) show() (uiShow, error) {
	if count(f.window, f.browser, f.noOpen) > 1 {
		return 0, errors.New(i18n.T("ui.flags.exclusive"))
	}
	if count(f.code, f.installShortcut, f.removeShortcut, f.stop) > 1 {
		return 0, errors.New(i18n.T("ui.flags.oneAction"))
	}
	if f.all && !f.stop {
		return 0, errors.New(i18n.T("ui.flags.allNeedsStop"))
	}
	switch {
	case f.window:
		return showWindow, nil
	case f.browser:
		return showTab, nil
	case f.noOpen:
		return showNone, nil
	}
	return showAuto, nil
}

func count(flags ...bool) (n int) {
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n
}

// runUI — execai ui after the flags and the config are read.
func runUI(ctx context.Context, out io.Writer, cfg *config.Config, f uiFlags, d uiDeps) error {
	show, err := f.show()
	if err != nil {
		return err
	}
	if f.installShortcut || f.removeShortcut {
		return uiShortcut(out, f, d)
	}
	if f.stop && f.all {
		return uiStopAll(out, d)
	}
	cwd := f.cwd
	if cwd == "" && f.last {
		// A launcher shortcut starts in no folder of its own. The folder of
		// the last run is the project; when there has been none, there is
		// no project — and the home folder is not one: everything the agent
		// may touch "inside the project" would be the whole of it. The
		// person is told how to name a folder, in the terminal and, since a
		// shortcut has none, by a notification.
		if cwd = uiwindow.LastDir(d.configDir); cwd == "" {
			if d.notify != nil {
				d.notify(i18n.T("ui.last.noneTitle"), i18n.T("ui.last.noneNotice"))
			}
			return errors.New(i18n.T("ui.last.none"))
		}
	}
	dir, err := uiDir(cwd)
	if err != nil {
		return err
	}
	inst, held, err := uiwindow.Acquire(d.configDir, dir)
	if err != nil {
		return err
	}
	defer inst.Release()
	if f.stop {
		return uiStop(out, inst, held, dir, d)
	}
	// Linux without a graphical session — a server reached over SSH, the
	// very place where other people have accounts. Nothing can be shown
	// there, so nothing is started: no browser, and not the system's opener
	// either, which would only put the address with the key among its
	// arguments for every program of the computer to read. The link is
	// printed; the person opens it where they sit.
	noScreen := d.goos == "linux" && d.getenv("DISPLAY") == "" && d.getenv("WAYLAND_DISPLAY") == ""
	st, running := uiHolder(inst, d)
	if f.code {
		return uiCode(out, st, running, d)
	}
	if running {
		if noScreen {
			show = showNone
		}
		return uiAlready(out, inst, st, show, d)
	}
	if !inst.Retry() {
		// Somebody holds the folder and says nothing about itself.
		fmt.Fprintln(out, i18n.T("ui.already.starting"))
		return nil
	}
	_ = uiwindow.SaveLastDir(d.configDir, dir)
	// What panels left for folders that are long gone, out of the way. It
	// runs beside the start and is waited for on the way out (briefly: a
	// command that ends at once — an error, no screen — must not leave it
	// working in the config folder behind its back).
	swept := make(chan struct{})
	go func() {
		defer close(swept)
		uiwindow.Sweep(d.configDir, d.home)
	}()
	defer func() {
		select {
		case <-swept:
		case <-time.After(2 * time.Second):
		}
	}()

	said := false
	if noScreen {
		switch show {
		case showWindow:
			return errors.New(i18n.T("ui.window.noScreen"))
		case showAuto, showTab:
			show, said = showNone, true
		}
	}
	// The browser is looked for before anything starts: --window without
	// one fails at once.
	var browser *uiwindow.Browser
	if show == showAuto || show == showWindow {
		b, err := uiBrowser(out, cfg, d)
		switch {
		case err == nil:
			browser = &b
		case show == showWindow:
			return errors.New(i18n.Tf("ui.window.none", uiwindow.Supported(d.goos)))
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	notices := ide.NewSyncNotices(0)
	defer startIDESync(ctx, cfg, uiSyncHooks(out, notices))()

	p := &uiPanel{out: out, d: d, inst: inst, cancel: cancel, show: show, noScreen: said,
		state: uiwindow.State{Cwd: dir, PID: os.Getpid(), Started: time.Now().UTC().Format(time.RFC3339), Mode: uiwindow.ModeNone}}
	opts := uiserve.Options{Port: f.port, TryPorts: d.ports(inst.Key()), Cwd: dir,
		Version: agentver.Get(), Locale: cfg.Locale, OnStop: p.askedToStop,
		OnSignIn: p.signedIn, OnIntruder: p.intruder}
	if browser != nil {
		// In the window links leave for the person's own browser.
		opts.OpenExternal = d.openTab
	}
	err = uiserve.Run(ctx, opts, d.ide(cfg, dir, notices), func(s *uiserve.Server) {
		p.started(s, show, browser)
	})
	p.stop()
	fmt.Fprintln(out, i18n.T("ui.stopped"))
	if failed := p.failure(); failed != nil {
		return failed
	}
	return err
}

// uiBrowser — the browser for the window: the one the person named
// (EXECAI_UI_BROWSER, then ui_browser of config.json), else the one found.
// A named browser that is not on this computer (removed since, a mistyped
// path) is said and does not stop the search.
func uiBrowser(out io.Writer, cfg *config.Config, d uiDeps) (uiwindow.Browser, error) {
	named := strings.TrimSpace(d.getenv(uiBrowserEnv))
	if named == "" {
		named = strings.TrimSpace(cfg.UIBrowser)
	}
	if named != "" {
		b, err := d.find(named)
		if err == nil {
			return b, nil
		}
		fmt.Fprintln(out, i18n.Tf("ui.window.notHere", named))
	}
	return d.find("")
}

// uiHolder tells who has the folder. running — another execai ui holds it
// and has said where it listens (st). Otherwise the folder is free, or will
// be in a moment: a lock that is taken with no panel behind it is a look by
// `execai ui --code`, `--stop --all` or the sweep, which take it for an
// instant — so the lock is tried again while the state is waited for, and a
// run that started in that instant becomes the holder all the same.
func uiHolder(inst *uiwindow.Instance, d uiDeps) (st uiwindow.State, running bool) {
	deadline := time.Now().Add(d.stateWait)
	for {
		if inst.Retry() {
			return uiwindow.State{}, false
		}
		if st, err := inst.Read(); err == nil && st.Port != 0 && st.PID != 0 {
			return st, true
		}
		if !time.Now().Before(deadline) {
			return uiwindow.State{}, false
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func uiStable(port int) string { return "http://127.0.0.1:" + strconv.Itoa(port) + "/" }

// uiAlready — the folder has its panel: show it instead of starting another.
// The window is asked for the way it was opened — the same address in the
// same profile, which the running browser of that profile takes over; the
// link's key is not needed there, the profile holds the session.
//
// The browser is not touched until the first run has marked that its window
// is up: two starts of one profile at the same moment race for it, and the
// loser's window lands in a browser nobody watches.
func uiAlready(out io.Writer, inst *uiwindow.Instance, st uiwindow.State, show uiShow, d uiDeps) error {
	url := uiStable(st.Port)
	if show != showNone && show != showTab && st.Mode == uiwindow.ModeWindow {
		for deadline := time.Now().Add(d.stateWait); !st.WindowUp && time.Now().Before(deadline); {
			time.Sleep(25 * time.Millisecond)
			if next, err := inst.Read(); err == nil && next.PID == st.PID {
				st = next
			}
		}
		if !st.WindowUp || st.Mode != uiwindow.ModeWindow || st.Browser == nil || st.Profile == "" {
			fmt.Fprintln(out, i18n.T("ui.already.starting"))
			return nil
		}
		if proc, err := uiwindow.Open(d.start, *st.Browser, st.Profile, url, d.goos); err == nil {
			// The started program passes the request on and ends; a moment
			// for that, and it is left alone either way.
			ended := make(chan struct{})
			go func() { _ = proc.Wait(); close(ended) }()
			select {
			case <-ended:
			case <-time.After(2 * time.Second):
			}
			fmt.Fprintln(out, i18n.Tf("ui.already.window", st.Browser.Name))
			return nil
		}
	}
	fmt.Fprintln(out, i18n.Tf("ui.already.tab", url))
	if show == showNone || st.Mode == uiwindow.ModeNone {
		return nil
	}
	if err := d.openTab(url); err != nil {
		fmt.Fprintln(out, i18n.Tf("ui.openFailed", err))
	}
	return nil
}

// uiCode — execai ui --code: asks the panel running for this folder for a
// one-time code and prints it with the address of the page it is for. The
// code is made on this request and lives a few minutes: there is none to
// guess, or to coax out of the person, at any other time.
func uiCode(out io.Writer, st uiwindow.State, running bool, d uiDeps) error {
	if !running || st.Control == "" {
		return errors.New(i18n.T("ui.code.notRunning"))
	}
	status, body, err := uiControl(st.Port, st.Control, "code")
	var answer struct {
		Code string `json:"code"`
		TTL  int    `json:"ttl"`
	}
	if err != nil || status != http.StatusOK || json.Unmarshal(body, &answer) != nil || answer.Code == "" {
		return errors.New(i18n.T("ui.code.notRunning"))
	}
	fmt.Fprintln(out, i18n.Tf("ui.code.line", uiStable(st.Port), answer.Code, (answer.TTL+59)/60))
	return nil
}

// uiStop — execai ui --stop: end the panel that runs for this folder, also
// one started from a shortcut that has no terminal to press Ctrl+C in.
func uiStop(out io.Writer, inst *uiwindow.Instance, held bool, dir string, d uiDeps) error {
	stopped, _, err := uiStopPanel(inst, held, dir, d)
	switch {
	case err != nil:
		return err
	case stopped:
		fmt.Fprintln(out, i18n.Tf("ui.stop.done", dir))
	default:
		fmt.Fprintln(out, i18n.T("ui.stop.notRunning"))
	}
	return nil
}

// uiStopAll — execai ui --stop --all: the panels of all folders.
func uiStopAll(out io.Writer, d uiDeps) error {
	n := 0
	var failed []string
	for _, key := range uiwindow.Keys(d.configDir) {
		inst, held, err := uiwindow.AcquireKey(d.configDir, key)
		if err != nil {
			continue
		}
		stopped, dir, err := uiStopPanel(inst, held, "", d)
		inst.Release()
		switch {
		case err != nil:
			failed = append(failed, err.Error())
		case stopped:
			n++
			fmt.Fprintln(out, i18n.Tf("ui.stop.done", dir))
		}
	}
	if len(failed) > 0 {
		// Said once, by whoever prints the command's error.
		return errors.New(strings.Join(failed, "\n"))
	}
	if n == 0 {
		fmt.Fprintln(out, i18n.T("ui.stop.noneRunning"))
	}
	return nil
}

// uiStopPanel asks the panel that holds a folder to end and waits until it
// has. stopped=false without an error: no panel runs there. dir — the
// folder, when the caller knows it; the state tells it otherwise.
//
// No signal is sent to any process: the pid in a state file may be of a
// panel long gone and since given to another program. The panel is asked at
// its own address, with the control secret of its run (POST /stop), which
// only the process that wrote the state under the folder's lock can match;
// and it counts as stopped only when that lock is free — the kernel drops
// it when the process ends, on Linux, macOS and Windows alike. A panel that
// holds its folder and does not answer is left alone and reported.
func uiStopPanel(inst *uiwindow.Instance, held bool, dir string, d uiDeps) (stopped bool, folder string, err error) {
	if held {
		return false, dir, nil // the lock was free: nothing runs, whatever the state file says
	}
	deadline := time.Now().Add(d.stopWait)
	asked, nextAsk := false, time.Time{}
	for {
		if !asked && !time.Now().Before(nextAsk) {
			// The state is read anew each time: a panel that is still
			// starting writes it a moment after it takes the lock.
			if st, rerr := inst.Read(); rerr == nil {
				if dir == "" {
					dir = st.Cwd
				}
				if st.Port != 0 && st.Control != "" {
					status, _, aerr := uiControl(st.Port, st.Control, "stop")
					asked = aerr == nil && status == http.StatusNoContent
				}
			}
			nextAsk = time.Now().Add(250 * time.Millisecond)
		}
		if inst.Retry() {
			return true, dir, nil
		}
		if !time.Now().Before(deadline) {
			return false, dir, errors.New(i18n.Tf("ui.stop.failed", dir))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// uiControl — POST /<what> to the panel on this computer's port, with the
// control secret of its run.
func uiControl(port int, control, what string) (status int, body []byte, err error) {
	req, err := http.NewRequest(http.MethodPost, uiStable(port)+what, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("X-Execai-Control", control)
	client := &http.Client{
		Timeout: 2 * time.Second,
		// Straight to 127.0.0.1: no proxy, and no following a redirect with
		// the secret in hand.
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, body, nil
}

// uiShortcut — --install-shortcut and --remove-shortcut: the general
// shortcut, or with --cwd the one of that folder.
func uiShortcut(out io.Writer, f uiFlags, d uiDeps) error {
	exe, err := d.exe()
	if err != nil {
		return err
	}
	spec := uiwindow.ShortcutSpec{GOOS: d.goos, Home: d.home, ConfigDir: d.configDir, Getenv: d.getenv,
		Exe: exe, Comment: i18n.T("ui.shortcut.comment")}
	if f.cwd != "" {
		if spec.Cwd, err = uiDir(f.cwd); err != nil {
			return err
		}
	}
	s, err := uiwindow.PlanShortcut(spec)
	if err != nil {
		return uiShortcutErr(err)
	}
	if f.removeShortcut {
		removed, err := s.Uninstall()
		if err != nil {
			return uiShortcutErr(err)
		}
		if removed {
			fmt.Fprintln(out, i18n.Tf("ui.shortcut.removed", s.Path))
		} else {
			fmt.Fprintln(out, i18n.Tf("ui.shortcut.absent", s.Path))
		}
		return nil
	}
	path, fallback, err := s.Install(d.runScript)
	if err != nil {
		return uiShortcutErr(err)
	}
	if fallback {
		fmt.Fprintln(out, i18n.Tf("ui.shortcut.fallback", path))
	} else {
		fmt.Fprintln(out, i18n.Tf("ui.shortcut.installed", path))
	}
	if spec.Cwd == "" && uiwindow.LastDir(d.configDir) == "" {
		fmt.Fprintln(out, i18n.T("ui.shortcut.noLast"))
	}
	if _, err := d.find(""); err != nil {
		// A shortcut has no terminal to stop the panel from.
		fmt.Fprintln(out, i18n.Tf("ui.shortcut.tabOnly", uiwindow.Supported(d.goos)))
	}
	return nil
}

// uiShortcutErr — what the shortcut code refuses with, in the person's
// language.
func uiShortcutErr(err error) error {
	var notOurs *uiwindow.NotOursError
	switch {
	case errors.As(err, &notOurs):
		return errors.New(i18n.Tf("ui.shortcut.notOurs", notOurs.Path))
	case errors.Is(err, uiwindow.ErrNoHome):
		return errors.New(i18n.T("ui.shortcut.noHome"))
	case errors.Is(err, uiwindow.ErrNoShortcut):
		return errors.New(i18n.T("ui.shortcut.unsupported"))
	}
	return err
}

// uiSyncHooks — what sync says in execai ui goes two ways: to the terminal
// that started the page, as before, and to the page itself as notice events
// (ide.SyncNotices). The page has no slash commands for sync, so the
// messages name the shell command.
func uiSyncHooks(out io.Writer, notices *ide.SyncNotices) agentsync.Hooks {
	return agentsync.Hooks{
		Notify: func(s string) {
			fmt.Fprintln(out, s)
			notices.Notify(s)
		},
		Cmd: "execai sync",
	}
}

// uiIDE — the agent behind the page: the editor protocol over the given
// streams, with the sync lines for the panel.
func uiIDE(cfg *config.Config, dir string, notices *ide.SyncNotices) uiserve.IDE {
	return func(ctx context.Context, in io.Reader, w io.Writer) error {
		// The page may be opened later than the agent starts, or closed for
		// a while: the lines wait for a page that listens.
		if l, ok := w.(interface{ Listening() bool }); ok {
			notices.WhileListening(l.Listening)
		}
		return ide.Run(ctx, cfg, ide.Options{Cwd: dir, In: in, Out: w, SyncNotices: notices})
	}
}

// uiDir — the project directory as an absolute path: the page shows it and
// uploads are checked against it.
func uiDir(cwd string) (string, error) {
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		cwd = wd
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return "", errors.New(i18n.Tf("ui.badCwd", abs))
	}
	return abs, nil
}
