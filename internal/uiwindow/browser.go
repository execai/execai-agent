// Package uiwindow shows the page of execai ui as a window of its own: it
// finds a Chromium-family browser on this computer, starts it in app mode
// (--app=<address>: no tabs, no address bar) with a profile kept in the
// agent's config directory, and tells when that window is closed. It also
// keeps what one execai ui per project folder needs (a lock and a state
// file) and writes the launcher shortcuts.
//
// The agent stays one static file: no cgo, no system webview — the window is
// a browser the person already has. Without one, execai ui opens a tab.
package uiwindow

import (
	"errors"
	"strings"
)

// Browser — a browser that can show a page as an app window.
type Browser struct {
	// ID — "chrome", "chromium", "edge", "yandex", "brave", "vivaldi"; for a
	// program the person named, its file name.
	ID string `json:"id"`
	// Name — how to call it in a message.
	Name string `json:"name"`
	// Path — the executable to start.
	Path string `json:"path"`
	// Kind — "" for an ordinary program; "snap" or "flatpak" for one that
	// runs in a sandbox and may write only inside its own folders.
	Kind string `json:"kind,omitempty"`
	// Sandbox — the snap name or the Flatpak application id.
	Sandbox string `json:"sandbox,omitempty"`
}

const (
	KindSnap    = "snap"
	KindFlatpak = "flatpak"
)

// System — what the search asks the computer; tests answer from maps.
type System struct {
	GOOS string
	Home string
	// Getenv — the environment (ProgramFiles, LocalAppData on Windows).
	Getenv func(string) string
	// LookPath — a command name on PATH.
	LookPath func(name string) (string, error)
	// Exists — a file that can be started is at this path.
	Exists func(path string) bool
	// Resolve — the path with its links followed.
	Resolve func(path string) string
	// DefaultBrowser — what opens http links: a desktop file name on Linux
	// ("google-chrome.desktop"), a bundle id on macOS ("com.google.chrome"),
	// a ProgId on Windows ("ChromeHTML"). Empty when unknown.
	DefaultBrowser func() string
	// AppPath — Windows: the path the registry's App Paths gives for an
	// executable name.
	AppPath func(exe string) string
}

var (
	// ErrNoBrowser — this computer has no browser with app windows.
	ErrNoBrowser = errors.New("no browser with app windows found")
	// ErrNotHere — the browser the person named is not on this computer.
	ErrNotHere = errors.New("the named browser is not on this computer")
)

// family — one browser of the Chromium family on the three systems.
//
// All of them are builds of Chromium and are started with its --app switch
// (chrome/common/chrome_switches). Of the six, Chromium and Google Chrome
// were run this way while this was written; Edge, Yandex Browser, Brave and
// Vivaldi are taken on the strength of the shared startup code — a browser
// that does not come up gives way to a tab. Arc is left out: nothing was
// found that says it honours the switch. Safari and Firefox have no such
// mode.
type family struct {
	id, name string
	aliases  []string // other names the person may write in ui_browser
	// Linux
	commands []string // on PATH
	flatpak  string   // application id
	snap     string   // snap name
	desktop  []string // beginnings of the desktop file name, lower case
	// macOS
	macApp    string // "Google Chrome" → "Google Chrome.app/Contents/MacOS/Google Chrome"
	macBundle string // lower case
	// Windows
	winExe  string   // App Paths key
	winDirs []string // under Program Files, Program Files (x86) and LocalAppData
	winProg string   // beginning of the ProgId, lower case
}

var families = map[string]family{
	"chrome": {id: "chrome", name: "Google Chrome", aliases: []string{"google-chrome", "google chrome"},
		commands: []string{"google-chrome", "google-chrome-stable"}, flatpak: "com.google.Chrome",
		desktop: []string{"google-chrome", "com.google.chrome"},
		macApp:  "Google Chrome", macBundle: "com.google.chrome",
		winExe: "chrome.exe", winDirs: []string{`Google\Chrome\Application\chrome.exe`}, winProg: "chromehtml"},
	"chromium": {id: "chromium", name: "Chromium", aliases: []string{"chromium-browser"},
		commands: []string{"chromium", "chromium-browser"}, flatpak: "org.chromium.Chromium", snap: "chromium",
		desktop: []string{"chromium", "org.chromium.chromium"},
		macApp:  "Chromium", macBundle: "org.chromium.chromium",
		winDirs: []string{`Chromium\Application\chrome.exe`}, winProg: "chromiumhtm"},
	"edge": {id: "edge", name: "Microsoft Edge", aliases: []string{"msedge", "microsoft-edge", "microsoft edge"},
		commands: []string{"microsoft-edge", "microsoft-edge-stable"}, flatpak: "com.microsoft.Edge",
		desktop: []string{"microsoft-edge", "com.microsoft.edge"},
		macApp:  "Microsoft Edge", macBundle: "com.microsoft.edgemac",
		winExe: "msedge.exe", winDirs: []string{`Microsoft\Edge\Application\msedge.exe`}, winProg: "msedgehtm"},
	"yandex": {id: "yandex", name: "Yandex Browser", aliases: []string{"yandex-browser", "yandex browser"},
		commands: []string{"yandex-browser", "yandex-browser-stable"}, flatpak: "ru.yandex.Browser",
		desktop: []string{"yandex-browser", "ru.yandex.browser"},
		macApp:  "Yandex", macBundle: "ru.yandex.desktop.yandex-browser",
		winExe: "browser.exe", winDirs: []string{`Yandex\YandexBrowser\Application\browser.exe`}, winProg: "yandexhtml"},
	"brave": {id: "brave", name: "Brave", aliases: []string{"brave-browser", "brave browser"},
		commands: []string{"brave-browser", "brave-browser-stable", "brave"}, flatpak: "com.brave.Browser", snap: "brave",
		desktop: []string{"brave", "com.brave.browser"},
		macApp:  "Brave Browser", macBundle: "com.brave.browser",
		winExe: "brave.exe", winDirs: []string{`BraveSoftware\Brave-Browser\Application\brave.exe`}, winProg: "bravehtml"},
	"vivaldi": {id: "vivaldi", name: "Vivaldi", aliases: []string{"vivaldi-stable"},
		commands: []string{"vivaldi", "vivaldi-stable"}, flatpak: "com.vivaldi.Vivaldi",
		desktop: []string{"vivaldi", "com.vivaldi.vivaldi"},
		macApp:  "Vivaldi", macBundle: "com.vivaldi.vivaldi",
		winExe: "vivaldi.exe", winDirs: []string{`Vivaldi\Application\vivaldi.exe`}, winProg: "vivaldihtm"},
}

// order — which browser is taken first when the default one is not of the
// family.
func order(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"chrome", "edge", "yandex", "chromium", "brave", "vivaldi"}
	case "windows":
		return []string{"edge", "chrome", "yandex", "brave", "vivaldi", "chromium"}
	}
	return []string{"chrome", "chromium", "edge", "yandex", "brave", "vivaldi"}
}

// Supported — the browsers execai ui can open a window with on this system,
// for a message.
func Supported(goos string) string {
	var names []string
	for _, id := range order(goos) {
		names = append(names, families[id].name)
	}
	return strings.Join(names, ", ")
}

// Find picks the browser for the window. explicit — what the person named
// (EXECAI_UI_BROWSER, ui_browser in config.json): a name of the list, a
// command or a path; ErrNotHere when this computer has no such thing. With
// nothing named: the default browser when it is of the family, otherwise
// the first one found in the order of the system; ErrNoBrowser when none.
func Find(sys System, explicit string) (Browser, error) {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return findExplicit(sys, explicit)
	}
	if def := strings.ToLower(strings.TrimSpace(sys.DefaultBrowser())); def != "" {
		for _, id := range order(sys.GOOS) {
			if f := families[id]; f.isDefault(sys.GOOS, def) {
				if b, ok := f.find(sys); ok {
					return b, nil
				}
				break
			}
		}
	}
	// Ordinary programs first: a browser in a sandbox keeps its profile in
	// another place and is taken only when there is nothing else.
	var wrapped *Browser
	for _, id := range order(sys.GOOS) {
		b, ok := families[id].find(sys)
		switch {
		case !ok:
		case b.Kind == "":
			return b, nil
		case wrapped == nil:
			wrapped = &b
		}
	}
	if wrapped != nil {
		return *wrapped, nil
	}
	return Browser{}, ErrNoBrowser
}

func findExplicit(sys System, explicit string) (Browser, error) {
	if strings.ContainsAny(explicit, `/\`) {
		// A path, and only a whole one: "bin/chrome" would be whatever lies
		// in the folder execai ui is started in — a file of the project.
		if !isAbs(explicit, sys.GOOS) || !sys.Exists(explicit) {
			return Browser{}, ErrNotHere
		}
		id := programName(explicit)
		return Browser{ID: id, Name: id, Path: explicit}, nil
	}
	want := strings.ToLower(explicit)
	for _, id := range order(sys.GOOS) {
		f := families[id]
		if want != f.id && !contains(f.aliases, want) && !contains(f.commands, want) {
			continue
		}
		if b, ok := f.find(sys); ok {
			return b, nil
		}
	}
	if p, err := sys.LookPath(explicit); err == nil && p != "" {
		return Browser{ID: programName(explicit), Name: explicit, Path: p}, nil
	}
	return Browser{}, ErrNotHere
}

// isAbs — path names a place by itself on the given system: /… on Unix; a
// drive with its root (C:\…) or a network share (\\server\…) on Windows.
func isAbs(path, goos string) bool {
	if goos != "windows" {
		return strings.HasPrefix(path, "/")
	}
	p := strings.ReplaceAll(path, "/", `\`)
	if strings.HasPrefix(p, `\\`) {
		return len(p) > 2
	}
	return len(p) >= 3 && p[1] == ':' && p[2] == '\\' &&
		(p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z')
}

// programName — the file name of a path of either system, without ".exe"
// and ".app".
func programName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		path = path[i+1:]
	}
	low := strings.ToLower(path)
	for _, ext := range []string{".exe", ".app"} {
		if strings.HasSuffix(low, ext) {
			path = path[:len(path)-len(ext)]
		}
	}
	return path
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// isDefault — def (lower case) names this browser as the system's default.
func (f family) isDefault(goos, def string) bool {
	switch goos {
	case "darwin":
		return f.macBundle != "" && def == f.macBundle
	case "windows":
		return f.winProg != "" && strings.HasPrefix(def, f.winProg)
	}
	for _, d := range f.desktop {
		if strings.HasPrefix(def, d) {
			return true
		}
	}
	return false
}

// find — where this browser is on the computer.
func (f family) find(sys System) (Browser, bool) {
	b := Browser{ID: f.id, Name: f.name}
	switch sys.GOOS {
	case "darwin":
		if f.macApp == "" {
			return b, false
		}
		// The program inside the bundle, not `open`: the window must be a
		// process of ours to wait for.
		rel := "/Applications/" + f.macApp + ".app/Contents/MacOS/" + f.macApp
		for _, p := range []string{rel, sys.Home + rel} {
			if sys.Exists(p) {
				b.Path = p
				return b, true
			}
		}
		return b, false
	case "windows":
		var candidates []string
		for _, root := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
			if dir := sys.Getenv(root); dir != "" {
				for _, rel := range f.winDirs {
					candidates = append(candidates, strings.TrimRight(dir, `\`)+`\`+rel)
				}
			}
		}
		if f.winExe != "" && sys.AppPath != nil {
			if p := strings.Trim(strings.TrimSpace(sys.AppPath(f.winExe)), `"`); p != "" {
				candidates = append(candidates, p)
			}
		}
		for _, p := range candidates {
			if sys.Exists(p) {
				b.Path = p
				return b, true
			}
		}
		return b, false
	}
	return f.findLinux(sys, b)
}

func (f family) findLinux(sys System, b Browser) (Browser, bool) {
	snapBin := ""
	if f.snap != "" && sys.Exists("/snap/bin/"+f.snap) {
		snapBin = "/snap/bin/" + f.snap
	}
	for _, name := range f.commands {
		p, err := sys.LookPath(name)
		if err != nil || p == "" {
			continue
		}
		switch {
		case isSnapPath(p) || isSnapPath(sys.Resolve(p)):
			b.Kind, b.Sandbox = KindSnap, f.snap
		case name == "chromium-browser" && snapBin != "":
			// The package of this name in some distributions is a script
			// that starts the snap.
			p = snapBin
			b.Kind, b.Sandbox = KindSnap, f.snap
		case strings.Contains(p, "/flatpak/exports/bin/"):
			b.Kind, b.Sandbox = KindFlatpak, f.flatpak
		}
		if b.Kind == KindSnap && f.snap == "" {
			continue // a snap we do not know where to keep the profile for
		}
		b.Path = p
		return b, true
	}
	if f.flatpak != "" {
		// The exported launcher is a script that passes its arguments on to
		// `flatpak run`.
		for _, dir := range []string{"/var/lib/flatpak/exports/bin/", sys.Home + "/.local/share/flatpak/exports/bin/"} {
			if sys.Exists(dir + f.flatpak) {
				b.Path, b.Kind, b.Sandbox = dir+f.flatpak, KindFlatpak, f.flatpak
				return b, true
			}
		}
	}
	if snapBin != "" {
		b.Path, b.Kind, b.Sandbox = snapBin, KindSnap, f.snap
		return b, true
	}
	return b, false
}

func isSnapPath(p string) bool {
	return strings.HasPrefix(p, "/snap/") || p == "/usr/bin/snap" || strings.HasPrefix(p, "/var/lib/snapd/")
}

// parseLSHandlers — the bundle id that handles http (or, when http has no
// handler of its own, https) in the output of
// `defaults read com.apple.LaunchServices/com.apple.launchservices.secure LSHandlers`.
// Empty when the person never chose a browser: the default is Safari.
func parseLSHandlers(out string) string {
	byScheme := map[string]string{}
	depth := 0
	var role, scheme string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		opens := strings.HasSuffix(line, "{")
		closes := strings.HasPrefix(line, "}")
		switch {
		case opens:
			depth++
			if depth == 1 {
				role, scheme = "", ""
			}
		case closes:
			if depth == 1 && scheme != "" && role != "" {
				byScheme[scheme] = role
			}
			if depth > 0 {
				depth--
			}
		case depth == 1:
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			v = strings.Trim(strings.TrimSuffix(strings.TrimSpace(v), ";"), `" `)
			switch strings.TrimSpace(k) {
			case "LSHandlerRoleAll":
				role = strings.ToLower(v)
			case "LSHandlerURLScheme":
				scheme = strings.ToLower(v)
			}
		}
	}
	if id := byScheme["http"]; id != "" {
		return id
	}
	return byScheme["https"]
}

// parseMimeapps — the desktop file that handles http in a mimeapps.list
// ([Default Applications]).
func parseMimeapps(list string) string {
	section := ""
	for _, line := range strings.Split(list, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		if section != "[Default Applications]" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != "x-scheme-handler/http" {
			continue
		}
		first, _, _ := strings.Cut(v, ";")
		return strings.TrimSpace(first)
	}
	return ""
}
