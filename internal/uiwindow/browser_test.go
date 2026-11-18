package uiwindow

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// fakeSys — a computer made of maps: what is on PATH, which files exist,
// what links resolve to, which browser is the default one.
type fakeSys struct {
	goos  string
	home  string
	env   map[string]string
	path  map[string]string // command name → where PATH finds it
	files []string          // executables that exist
	links map[string]string // path → what it resolves to
	def   string
	reg   map[string]string // Windows App Paths: exe name → path
}

func (f fakeSys) system() System {
	exists := map[string]bool{}
	for _, p := range f.files {
		exists[p] = true
	}
	for _, p := range f.path {
		exists[p] = true
	}
	return System{
		GOOS:   f.goos,
		Home:   f.home,
		Getenv: func(k string) string { return f.env[k] },
		LookPath: func(name string) (string, error) {
			if p, ok := f.path[name]; ok {
				return p, nil
			}
			return "", os.ErrNotExist
		},
		Exists: func(p string) bool { return exists[p] },
		Resolve: func(p string) string {
			if to, ok := f.links[p]; ok {
				return to
			}
			return p
		},
		DefaultBrowser: func() string { return f.def },
		AppPath:        func(exe string) string { return f.reg[exe] },
	}
}

var winEnv = map[string]string{
	"ProgramFiles":      `C:\Program Files`,
	"ProgramFiles(x86)": `C:\Program Files (x86)`,
	"LocalAppData":      `C:\Users\u\AppData\Local`,
}

func TestFind(t *testing.T) {
	const (
		macChrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		macEdge   = "/Users/u/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"
		macYandex = "/Applications/Yandex.app/Contents/MacOS/Yandex"
		macBrave  = "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
		macViv    = "/Applications/Vivaldi.app/Contents/MacOS/Vivaldi"
		macArc    = "/Applications/Arc.app/Contents/MacOS/Arc"
		winEdge   = `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`
		winChrome = `C:\Users\u\AppData\Local\Google\Chrome\Application\chrome.exe`
		winYandex = `C:\Users\u\AppData\Local\Yandex\YandexBrowser\Application\browser.exe`
	)
	cases := []struct {
		name     string
		sys      fakeSys
		explicit string
		wantID   string // "" — no browser
		wantPath string
		wantKind string
	}{
		// Linux
		{name: "linux: only Firefox", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"firefox": "/usr/bin/firefox"}, def: "firefox.desktop"}},
		{name: "linux: Chrome, Firefox is the default", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"google-chrome": "/usr/bin/google-chrome", "firefox": "/usr/bin/firefox"}, def: "firefox.desktop"},
			wantID: "chrome", wantPath: "/usr/bin/google-chrome"},
		{name: "linux: google-chrome-stable only", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"google-chrome-stable": "/usr/bin/google-chrome-stable"}},
			wantID: "chrome", wantPath: "/usr/bin/google-chrome-stable"},
		{name: "linux: the default browser wins over the order of the list", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"google-chrome": "/usr/bin/google-chrome", "chromium": "/usr/bin/chromium"}, def: "chromium.desktop"},
			wantID: "chromium", wantPath: "/usr/bin/chromium"},
		{name: "linux: the default is named but not installed", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"microsoft-edge": "/usr/bin/microsoft-edge"}, def: "google-chrome.desktop"},
			wantID: "edge", wantPath: "/usr/bin/microsoft-edge"},
		{name: "linux: Yandex Browser", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"yandex-browser-stable": "/usr/bin/yandex-browser-stable"}, def: "yandex-browser.desktop"},
			wantID: "yandex", wantPath: "/usr/bin/yandex-browser-stable"},
		{name: "linux: the order — Chrome, Chromium, Edge, Yandex, Brave, Vivaldi", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"vivaldi": "/usr/bin/vivaldi", "brave-browser": "/usr/bin/brave-browser", "yandex-browser": "/usr/bin/yandex-browser"}},
			wantID: "yandex", wantPath: "/usr/bin/yandex-browser"},
		{name: "linux: Brave before Vivaldi", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"vivaldi-stable": "/usr/bin/vivaldi-stable", "brave-browser": "/usr/bin/brave-browser"}},
			wantID: "brave", wantPath: "/usr/bin/brave-browser"},
		{name: "linux: a Flatpak of the system", sys: fakeSys{goos: "linux", home: "/home/u",
			files: []string{"/var/lib/flatpak/exports/bin/com.google.Chrome"}, def: "com.google.Chrome.desktop"},
			wantID: "chrome", wantPath: "/var/lib/flatpak/exports/bin/com.google.Chrome", wantKind: "flatpak"},
		{name: "linux: a Flatpak of the user", sys: fakeSys{goos: "linux", home: "/home/u",
			files: []string{"/home/u/.local/share/flatpak/exports/bin/com.brave.Browser"}},
			wantID: "brave", wantPath: "/home/u/.local/share/flatpak/exports/bin/com.brave.Browser", wantKind: "flatpak"},
		{name: "linux: a snap on PATH", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"chromium": "/snap/bin/chromium"}, links: map[string]string{"/snap/bin/chromium": "/usr/bin/snap"},
			def: "chromium_chromium.desktop"},
			wantID: "chromium", wantPath: "/snap/bin/chromium", wantKind: "snap"},
		{name: "linux: a snap not on PATH", sys: fakeSys{goos: "linux", home: "/home/u",
			files: []string{"/snap/bin/chromium"}},
			wantID: "chromium", wantPath: "/snap/bin/chromium", wantKind: "snap"},
		{name: "linux: the chromium-browser script of a distribution that starts the snap", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"chromium-browser": "/usr/bin/chromium-browser"}, files: []string{"/snap/bin/chromium"}},
			wantID: "chromium", wantPath: "/snap/bin/chromium", wantKind: "snap"},
		{name: "linux: a native browser before a wrapped one", sys: fakeSys{goos: "linux", home: "/home/u",
			path:  map[string]string{"microsoft-edge": "/usr/bin/microsoft-edge"},
			files: []string{"/var/lib/flatpak/exports/bin/com.google.Chrome"}},
			wantID: "edge", wantPath: "/usr/bin/microsoft-edge"},

		// macOS
		{name: "mac: only Safari", sys: fakeSys{goos: "darwin", home: "/Users/u", def: "com.apple.safari"}},
		{name: "mac: Arc alone is not enough", sys: fakeSys{goos: "darwin", home: "/Users/u",
			files: []string{macArc}, def: "company.thebrowser.browser"}},
		{name: "mac: Chrome in /Applications", sys: fakeSys{goos: "darwin", home: "/Users/u",
			files: []string{macChrome}, def: "com.apple.safari"},
			wantID: "chrome", wantPath: macChrome},
		{name: "mac: Edge in ~/Applications", sys: fakeSys{goos: "darwin", home: "/Users/u",
			files: []string{macEdge}},
			wantID: "edge", wantPath: macEdge},
		{name: "mac: the default browser wins", sys: fakeSys{goos: "darwin", home: "/Users/u",
			files: []string{macChrome, macYandex}, def: "ru.yandex.desktop.yandex-browser"},
			wantID: "yandex", wantPath: macYandex},
		{name: "mac: the order with Safari as the default", sys: fakeSys{goos: "darwin", home: "/Users/u",
			files: []string{macViv, macBrave}, def: "com.apple.safari"},
			wantID: "brave", wantPath: macBrave},

		// Windows
		{name: "windows: nothing", sys: fakeSys{goos: "windows", home: `C:\Users\u`, env: winEnv, def: "FirefoxURL-308046B0AF4A39CB"}},
		{name: "windows: Edge, Firefox is the default", sys: fakeSys{goos: "windows", home: `C:\Users\u`, env: winEnv,
			files: []string{winEdge}, def: "FirefoxURL-308046B0AF4A39CB"},
			wantID: "edge", wantPath: winEdge},
		{name: "windows: Edge before Chrome by the list", sys: fakeSys{goos: "windows", home: `C:\Users\u`, env: winEnv,
			files: []string{winEdge, winChrome}},
			wantID: "edge", wantPath: winEdge},
		{name: "windows: Chrome is the default", sys: fakeSys{goos: "windows", home: `C:\Users\u`, env: winEnv,
			files: []string{winEdge, winChrome}, def: "ChromeHTML"},
			wantID: "chrome", wantPath: winChrome},
		{name: "windows: Chrome only in the registry", sys: fakeSys{goos: "windows", home: `C:\Users\u`, env: winEnv,
			files: []string{`D:\Apps\Chrome\chrome.exe`}, reg: map[string]string{"chrome.exe": `D:\Apps\Chrome\chrome.exe`}},
			wantID: "chrome", wantPath: `D:\Apps\Chrome\chrome.exe`},
		{name: "windows: a registry entry of a removed browser", sys: fakeSys{goos: "windows", home: `C:\Users\u`, env: winEnv,
			files: []string{winEdge}, reg: map[string]string{"chrome.exe": `D:\Gone\chrome.exe`}, def: "ChromeHTML"},
			wantID: "edge", wantPath: winEdge},
		{name: "windows: Yandex Browser is the default", sys: fakeSys{goos: "windows", home: `C:\Users\u`, env: winEnv,
			files: []string{winEdge, winYandex}, def: "YandexHTML.ABCDEF"},
			wantID: "yandex", wantPath: winYandex},

		// Named by the person.
		{name: "explicit: a name", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"google-chrome": "/usr/bin/google-chrome", "microsoft-edge": "/usr/bin/microsoft-edge"}, def: "google-chrome.desktop"},
			explicit: "edge", wantID: "edge", wantPath: "/usr/bin/microsoft-edge"},
		{name: "explicit: a path", sys: fakeSys{goos: "linux", home: "/home/u",
			path:  map[string]string{"google-chrome": "/usr/bin/google-chrome"},
			files: []string{"/opt/pw/chrome-linux64/chrome"}},
			explicit: "/opt/pw/chrome-linux64/chrome", wantID: "chrome", wantPath: "/opt/pw/chrome-linux64/chrome"},
		{name: "explicit: a command on PATH", sys: fakeSys{goos: "linux", home: "/home/u",
			path: map[string]string{"my-browser": "/usr/local/bin/my-browser"}},
			explicit: "my-browser", wantID: "my-browser", wantPath: "/usr/local/bin/my-browser"},
		{name: "explicit: a Windows path", sys: fakeSys{goos: "windows", home: `C:\Users\u`, env: winEnv,
			files: []string{`D:\Portable\msedge.exe`}},
			explicit: `D:\Portable\msedge.exe`, wantID: "msedge", wantPath: `D:\Portable\msedge.exe`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := Find(c.sys.system(), c.explicit)
			if c.wantID == "" {
				if !errors.Is(err, ErrNoBrowser) {
					t.Fatalf("got %+v, %v; want ErrNoBrowser", b, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if b.ID != c.wantID || b.Path != c.wantPath || b.Kind != c.wantKind {
				t.Fatalf("got %s %q kind %q, want %s %q kind %q", b.ID, b.Path, b.Kind, c.wantID, c.wantPath, c.wantKind)
			}
			if b.Name == "" {
				t.Error("no name to show")
			}
		})
	}
}

// A browser the person named that is not on this computer is its own error:
// the caller says so and looks for another one.
func TestFind_ExplicitMissing(t *testing.T) {
	sys := fakeSys{goos: "linux", home: "/home/u", path: map[string]string{"google-chrome": "/usr/bin/google-chrome"}}
	for _, explicit := range []string{"edge", "/opt/none/chrome", "no-such-command", `C:\Program Files\Google\Chrome\Application\chrome.exe`} {
		_, err := Find(sys.system(), explicit)
		if !errors.Is(err, ErrNotHere) {
			t.Errorf("%s: %v, want ErrNotHere", explicit, err)
		}
	}
}

// A named browser with a slash in it is a path, and only a whole one: a
// path that depends on the folder execai ui is started in would run
// whatever lies there — a file of the project.
func TestFind_ExplicitPathMustBeAbsolute(t *testing.T) {
	linux := fakeSys{goos: "linux", home: "/home/u",
		files: []string{"./chrome", "bin/chrome", "../chrome", "tools/chrome/chrome", "~/chrome", "/opt/chrome/chrome"}}
	for _, rel := range []string{"./chrome", "bin/chrome", "../chrome", "tools/chrome/chrome", "~/chrome"} {
		if b, err := Find(linux.system(), rel); !errors.Is(err, ErrNotHere) {
			t.Errorf("linux %s: %+v %v", rel, b, err)
		}
	}
	if b, err := Find(linux.system(), "/opt/chrome/chrome"); err != nil || b.Path != "/opt/chrome/chrome" {
		t.Errorf("linux, a whole path: %+v %v", b, err)
	}
	win := fakeSys{goos: "windows", home: `C:\Users\u`, env: winEnv,
		files: []string{`tools\chrome.exe`, `.\chrome.exe`, `\chrome.exe`, `C:chrome.exe`, `/chrome.exe`, `C:\Tools\chrome.exe`, `\\server\share\chrome.exe`, `c:/Tools/chrome.exe`}}
	for _, rel := range []string{`tools\chrome.exe`, `.\chrome.exe`, `\chrome.exe`, `C:chrome.exe`, `/chrome.exe`} {
		if b, err := Find(win.system(), rel); !errors.Is(err, ErrNotHere) {
			t.Errorf("windows %s: %+v %v", rel, b, err)
		}
	}
	for _, abs := range []string{`C:\Tools\chrome.exe`, `\\server\share\chrome.exe`, `c:/Tools/chrome.exe`} {
		if b, err := Find(win.system(), abs); err != nil || b.Path != abs {
			t.Errorf("windows, a whole path %s: %+v %v", abs, b, err)
		}
	}
}

func TestSupported(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		got := Supported(goos)
		for _, want := range []string{"Google Chrome", "Microsoft Edge", "Yandex"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: %q lacks %s", goos, got, want)
			}
		}
		if strings.Contains(got, "Arc") || strings.Contains(got, "Safari") || strings.Contains(got, "Firefox") {
			t.Errorf("%s: %q names a browser without app windows", goos, got)
		}
	}
}

// The default browser on macOS: the http handler of LaunchServices, as
// `defaults read` prints it.
func TestParseLSHandlers(t *testing.T) {
	const out = `(
        {
        LSHandlerContentType = "public.html";
        LSHandlerPreferredVersions =         {
            LSHandlerRoleAll = "-";
        };
        LSHandlerRoleAll = "com.apple.safari";
    },
        {
        LSHandlerPreferredVersions =         {
            LSHandlerRoleAll = "-";
        };
        LSHandlerRoleAll = "com.google.chrome";
        LSHandlerURLScheme = https;
    },
        {
        LSHandlerPreferredVersions =         {
            LSHandlerRoleAll = "-";
        };
        LSHandlerRoleAll = "ru.yandex.desktop.yandex-browser";
        LSHandlerURLScheme = http;
    },
        {
        LSHandlerRoleAll = "com.apple.mail";
        LSHandlerURLScheme = mailto;
    }
)`
	if got := parseLSHandlers(out); got != "ru.yandex.desktop.yandex-browser" {
		t.Errorf("http handler %q", got)
	}
	if got := parseLSHandlers("(\n)\n"); got != "" {
		t.Errorf("no handlers: %q", got)
	}
	onlyHTTPS := strings.Replace(out, "LSHandlerURLScheme = http;", "LSHandlerURLScheme = ftp;", 1)
	if got := parseLSHandlers(onlyHTTPS); got != "com.google.chrome" {
		t.Errorf("https handler when http has none: %q", got)
	}
}

// The default browser on Linux without xdg-settings: mimeapps.list.
func TestParseMimeapps(t *testing.T) {
	const list = `[Added Associations]
x-scheme-handler/http=firefox.desktop;

[Default Applications]
text/html=firefox.desktop
x-scheme-handler/http=microsoft-edge.desktop;firefox.desktop;
x-scheme-handler/https=microsoft-edge.desktop
`
	if got := parseMimeapps(list); got != "microsoft-edge.desktop" {
		t.Errorf("default %q", got)
	}
	if got := parseMimeapps("[Default Applications]\ntext/html=firefox.desktop\n"); got != "" {
		t.Errorf("no http handler: %q", got)
	}
}
