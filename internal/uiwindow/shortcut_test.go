package uiwindow

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/brand"
)

func fileOf(t *testing.T, s Shortcut, path string) File {
	t.Helper()
	for _, f := range s.Files {
		if f.Path == path {
			return f
		}
	}
	var have []string
	for _, f := range s.Files {
		have = append(have, f.Path)
	}
	t.Fatalf("no %s among %q", path, have)
	return File{}
}

func TestShortcut_Linux(t *testing.T) {
	spec := ShortcutSpec{GOOS: "linux", Home: "/home/u", ConfigDir: "/home/u/.config/execai",
		Getenv: func(string) string { return "" }, Exe: "/home/u/.local/bin/execai", Comment: "The agent panel"}
	s, err := PlanShortcut(spec)
	if err != nil {
		t.Fatal(err)
	}
	const desktop = "/home/u/.local/share/applications/execai-ui.desktop"
	const icon = "/home/u/.local/share/icons/hicolor/256x256/apps/execai-ui.png"
	if s.Path != desktop {
		t.Fatalf("path %s", s.Path)
	}
	want := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Version=1.0\n" +
		"Name=ExecAI\n" +
		"Comment=The agent panel\n" +
		"Exec=\"/home/u/.local/bin/execai\" ui --last\n" +
		"Icon=" + icon + "\n" +
		"Terminal=false\n" +
		"Categories=Development;\n" +
		"StartupWMClass=execai-ui\n" +
		"X-ExecAI-Shortcut=execai-ui-shortcut\n"
	if got := string(fileOf(t, s, desktop).Data); got != want {
		t.Errorf("desktop entry:\n%s\nwant:\n%s", got, want)
	}
	if !bytes.Equal(fileOf(t, s, icon).Data, brand.IconPNG) {
		t.Error("the icon is not the product icon")
	}
	if !slices.Contains(s.Remove, desktop) || !slices.Contains(s.Remove, icon) {
		t.Errorf("remove %q", s.Remove)
	}

	// XDG_DATA_HOME moves both files.
	spec.Getenv = func(k string) string {
		if k == "XDG_DATA_HOME" {
			return "/data"
		}
		return ""
	}
	if s, _ := PlanShortcut(spec); s.Path != "/data/applications/execai-ui.desktop" {
		t.Errorf("XDG_DATA_HOME: %s", s.Path)
	}
}

// A shortcut for one project: its own file and name, the folder quoted by
// the rules of the Exec key.
func TestShortcut_LinuxProject(t *testing.T) {
	const cwd = `/home/u/work/my $proj 100%`
	spec := ShortcutSpec{GOOS: "linux", Home: "/home/u", ConfigDir: "/home/u/.config/execai",
		Getenv: func(string) string { return "" }, Exe: "/opt/exec ai/execai", Cwd: cwd, Comment: "c"}
	s, err := PlanShortcut(spec)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := "/home/u/.local/share/applications/execai-ui-" + Key(cwd)[:8] + ".desktop"
	if s.Path != wantPath {
		t.Fatalf("path %s, want %s", s.Path, wantPath)
	}
	got := string(fileOf(t, s, wantPath).Data)
	if !strings.Contains(got, "Name=ExecAI — my $proj 100%\n") {
		t.Errorf("name:\n%s", got)
	}
	if !strings.Contains(got, `Exec="/opt/exec ai/execai" ui --cwd "/home/u/work/my \\$proj 100%%"`+"\n") {
		t.Errorf("exec:\n%s", got)
	}
	// The icon is shared with the general shortcut and stays when a project's
	// shortcut is removed.
	if len(s.Remove) != 1 || s.Remove[0] != wantPath {
		t.Errorf("remove %q", s.Remove)
	}
}

func TestShortcut_Mac(t *testing.T) {
	spec := ShortcutSpec{GOOS: "darwin", Home: "/Users/u", ConfigDir: "/Users/u/Library/Application Support/execai",
		Getenv: func(string) string { return "" }, Exe: "/usr/local/bin/execai", Comment: "The agent panel"}
	s, err := PlanShortcut(spec)
	if err != nil {
		t.Fatal(err)
	}
	const app = "/Users/u/Applications/ExecAI.app"
	if s.Path != app {
		t.Fatalf("path %s", s.Path)
	}
	plist := string(fileOf(t, s, app+"/Contents/Info.plist").Data)
	for _, want := range []string{
		"<key>CFBundleExecutable</key>\n\t<string>execai-ui</string>",
		"<key>CFBundleIdentifier</key>\n\t<string>ru.execai.ui</string>",
		"<key>CFBundleName</key>\n\t<string>ExecAI</string>",
		"<key>CFBundleIconFile</key>\n\t<string>icon</string>",
		"<key>CFBundlePackageType</key>\n\t<string>APPL</string>",
		"<key>LSUIElement</key>\n\t<true/>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("Info.plist lacks %q:\n%s", want, plist)
		}
	}
	run := fileOf(t, s, app+"/Contents/MacOS/execai-ui")
	if run.Mode != 0o755 {
		t.Errorf("launcher mode %v", run.Mode)
	}
	script := string(run.Data)
	if !strings.HasPrefix(script, "#!/bin/sh\n") || !strings.Contains(script, `'/usr/local/bin/execai' 'ui' '--last'`) ||
		!strings.Contains(script, " -l -c ") {
		t.Errorf("launcher:\n%s", script)
	}
	if !bytes.Equal(fileOf(t, s, app+"/Contents/Resources/icon.icns").Data, brand.IconICNS()) {
		t.Error("the icon is not the product icon")
	}
	if len(s.Remove) != 1 || s.Remove[0] != app {
		t.Errorf("remove %q", s.Remove)
	}

	// A project: its own app, the folder safe inside single quotes.
	spec.Cwd = "/Users/u/work/it's mine"
	s, err = PlanShortcut(spec)
	if err != nil {
		t.Fatal(err)
	}
	const papp = "/Users/u/Applications/ExecAI (it's mine).app"
	if s.Path != papp {
		t.Fatalf("path %s", s.Path)
	}
	script = string(fileOf(t, s, papp+"/Contents/MacOS/execai-ui").Data)
	if !strings.Contains(script, `'ui' '--cwd' '/Users/u/work/it'\''s mine'`) {
		t.Errorf("launcher:\n%s", script)
	}
	plist = string(fileOf(t, s, papp+"/Contents/Info.plist").Data)
	if !strings.Contains(plist, "<string>ru.execai.ui."+Key(spec.Cwd)[:8]+"</string>") ||
		!strings.Contains(plist, "<string>ExecAI (it&apos;s mine)</string>") && !strings.Contains(plist, "<string>ExecAI (it&#39;s mine)</string>") {
		t.Errorf("Info.plist:\n%s", plist)
	}
}

func TestShortcut_Windows(t *testing.T) {
	env := map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`}
	spec := ShortcutSpec{GOOS: "windows", Home: `C:\Users\u`, ConfigDir: `C:\Users\u\AppData\Roaming\execai`,
		Getenv: func(k string) string { return env[k] }, Exe: `C:\Users\u\AppData\Local\Programs\execai\execai.exe`, Comment: "The agent panel"}
	s, err := PlanShortcut(spec)
	if err != nil {
		t.Fatal(err)
	}
	const programs = `C:\Users\u\AppData\Roaming\Microsoft\Windows\Start Menu\Programs`
	const ico = `C:\Users\u\AppData\Roaming\execai\ui\execai.ico`
	if s.Path != programs+`\ExecAI.lnk` {
		t.Fatalf("path %s", s.Path)
	}
	if !bytes.Equal(fileOf(t, s, ico).Data, brand.IconICO()) {
		t.Error("the icon is not the product icon")
	}
	if len(s.Script) < 3 || s.Script[0] != "powershell.exe" || !slices.Contains(s.Script, "-NoProfile") {
		t.Fatalf("script %q", s.Script)
	}
	ps := s.Script[len(s.Script)-1]
	for _, want := range []string{"WScript.Shell", "CreateShortcut($env:EXECAI_LNK)", "$env:EXECAI_TARGET", "$env:EXECAI_ARGS", "$env:EXECAI_ICON", ".Save()"} {
		if !strings.Contains(ps, want) {
			t.Errorf("the script lacks %q:\n%s", want, ps)
		}
	}
	// The paths go through the environment, never into the script text.
	if strings.Contains(ps, `C:\`) {
		t.Errorf("a path inside the script:\n%s", ps)
	}
	for _, want := range []string{
		`EXECAI_LNK=` + programs + `\ExecAI.lnk`,
		`EXECAI_TARGET=C:\Users\u\AppData\Local\Programs\execai\execai.exe`,
		`EXECAI_ARGS=ui --last`,
		`EXECAI_ICON=` + ico,
		`EXECAI_DIR=C:\Users\u`,
		`EXECAI_DESC=The agent panel (execai-ui-shortcut)`,
	} {
		if !slices.Contains(s.Env, want) {
			t.Errorf("no %s in %q", want, s.Env)
		}
	}
	if s.Fallback == nil || s.Fallback.Path != programs+`\ExecAI.cmd` {
		t.Fatalf("fallback %+v", s.Fallback)
	}
	// chcp 65001: the file is UTF-8, and a folder with letters outside the
	// console's code page must reach execai as it is.
	if got, want := string(s.Fallback.Data), "@echo off\r\nrem execai-ui-shortcut\r\nchcp 65001 >nul\r\nstart \"ExecAI\" /min \"C:\\Users\\u\\AppData\\Local\\Programs\\execai\\execai.exe\" ui --last\r\n"; got != want {
		t.Errorf("fallback %q, want %q", got, want)
	}
	for _, want := range []string{programs + `\ExecAI.lnk`, programs + `\ExecAI.cmd`, ico} {
		if !slices.Contains(s.Remove, want) {
			t.Errorf("remove %q lacks %s", s.Remove, want)
		}
	}

	// A project folder with a space and a percent sign.
	spec.Cwd = `D:\work\my proj 100%`
	s, err = PlanShortcut(spec)
	if err != nil {
		t.Fatal(err)
	}
	if s.Path != programs+`\ExecAI (my proj 100%).lnk` {
		t.Fatalf("path %s", s.Path)
	}
	if !slices.Contains(s.Env, `EXECAI_ARGS=ui --cwd "D:\work\my proj 100%"`) || !slices.Contains(s.Env, `EXECAI_DIR=D:\work\my proj 100%`) {
		t.Errorf("env %q", s.Env)
	}
	if !strings.Contains(string(s.Fallback.Data), `ui --cwd "D:\work\my proj 100%%"`) {
		t.Errorf("fallback %q", s.Fallback.Data)
	}
	if slices.Contains(s.Remove, ico) {
		t.Error("removing a project's shortcut takes the shared icon")
	}
}

func TestShortcut_UnsupportedOS(t *testing.T) {
	if _, err := PlanShortcut(ShortcutSpec{GOOS: "freebsd", Home: "/home/u", Exe: "/x"}); !errors.Is(err, ErrNoShortcut) {
		t.Fatalf("a shortcut on an OS without one: %v", err)
	}
}

// Install writes the files; Uninstall takes away what Install wrote and
// nothing else.
func TestShortcut_InstallUninstall(t *testing.T) {
	home := t.TempDir()
	for _, goos := range []string{"linux", "darwin"} {
		spec := ShortcutSpec{GOOS: goos, Home: home, ConfigDir: filepath.Join(home, "cfg"),
			Getenv: func(string) string { return "" }, Exe: "/usr/local/bin/execai", Comment: "c"}
		s, err := PlanShortcut(spec)
		if err != nil {
			t.Fatal(err)
		}
		path, fallback, err := s.Install(nil)
		if err != nil || fallback || path != s.Path {
			t.Fatalf("%s: %q %v %v", goos, path, fallback, err)
		}
		for _, f := range s.Files {
			fi, err := os.Stat(f.Path)
			if err != nil {
				t.Fatalf("%s: %v", goos, err)
			}
			if f.Mode == 0o755 && fi.Mode().Perm()&0o100 == 0 {
				t.Errorf("%s: %s is not executable", goos, f.Path)
			}
		}
		neighbour := filepath.Join(filepath.Dir(s.Path), "other.keep")
		if err := os.WriteFile(neighbour, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		removed, err := s.Uninstall()
		if err != nil || !removed {
			t.Fatalf("%s: removed=%v err=%v", goos, removed, err)
		}
		if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
			t.Errorf("%s: %s is still there", goos, s.Path)
		}
		if _, err := os.Stat(neighbour); err != nil {
			t.Errorf("%s: a neighbour file was removed", goos)
		}
		if removed, err := s.Uninstall(); err != nil || removed {
			t.Errorf("%s: a second removal: removed=%v err=%v", goos, removed, err)
		}
	}
}

// On Windows the .lnk is written by a script; when it cannot run, a .cmd
// file takes its place.
func TestShortcut_ScriptAndFallback(t *testing.T) {
	dir := t.TempDir()
	s := Shortcut{
		Path:     filepath.Join(dir, "ExecAI.lnk"),
		Files:    []File{{Path: filepath.Join(dir, "ui", "execai.ico"), Data: []byte("ico"), Mode: 0o644}},
		Script:   []string{"powershell.exe", "-NoProfile", "-Command", "x"},
		Env:      []string{"EXECAI_LNK=" + filepath.Join(dir, "ExecAI.lnk")},
		Fallback: &File{Path: filepath.Join(dir, "ExecAI.cmd"), Data: []byte("@echo off\r\n"), Mode: 0o644},
	}
	var ran [][]string
	path, fallback, err := s.Install(func(argv, env []string) error {
		ran = append(ran, argv, env)
		return nil
	})
	if err != nil || fallback || path != s.Path || len(ran) != 2 || ran[0][0] != "powershell.exe" || ran[1][0] != s.Env[0] {
		t.Fatalf("%q %v %v %q", path, fallback, err, ran)
	}
	if _, err := os.Stat(s.Fallback.Path); !os.IsNotExist(err) {
		t.Error("the fallback was written although the script ran")
	}
	path, fallback, err = s.Install(func(argv, env []string) error { return errors.New("blocked by policy") })
	if err != nil || !fallback || path != s.Fallback.Path {
		t.Fatalf("%q %v %v", path, fallback, err)
	}
	if b, _ := os.ReadFile(s.Fallback.Path); string(b) != "@echo off\r\n" {
		t.Errorf("fallback file %q", b)
	}
}

// Without a home folder there is nowhere of the person's to put a shortcut
// — and "/Applications/ExecAI.app" is not a path to write to, or to remove.
func TestShortcut_NoHome(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, home := range []string{"", "  "} {
			s, err := PlanShortcut(ShortcutSpec{GOOS: goos, Home: home, ConfigDir: "/cfg", Exe: "/x/execai", Comment: "c"})
			if !errors.Is(err, ErrNoHome) {
				t.Errorf("%s with the home folder %q: a shortcut at %s, %v", goos, home, s.Path, err)
			}
		}
	}
}

// Install and Uninstall touch only what Install made: a shortcut carries a
// mark of ours inside, and something else of the same name — another
// program's bundle in ~/Applications, most of all — is neither overwritten
// nor removed.
func TestShortcut_OnlyItsOwn(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		home := t.TempDir()
		spec := ShortcutSpec{GOOS: goos, Home: home, ConfigDir: filepath.Join(home, "cfg"),
			Getenv: func(string) string { return "" }, Exe: "/usr/local/bin/execai", Comment: "c"}
		s, err := PlanShortcut(spec)
		if err != nil {
			t.Fatal(err)
		}
		// Somebody else's thing is where the shortcut would go.
		foreign := s.Path
		precious := foreign
		if goos == "darwin" {
			precious = filepath.Join(foreign, "Contents", "MacOS", "Another Program")
		}
		if err := os.MkdirAll(filepath.Dir(precious), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(precious, []byte("not ours"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Install(nil)
		var notOurs *NotOursError
		if !errors.Is(err, ErrNotOurs) || !errors.As(err, &notOurs) || notOurs.Path != foreign {
			t.Errorf("%s: Install over something else: %v", goos, err)
		}
		if removed, err := s.Uninstall(); !errors.Is(err, ErrNotOurs) || removed {
			t.Errorf("%s: Uninstall of something else: removed=%v err=%v", goos, removed, err)
		}
		if b, err := os.ReadFile(precious); err != nil || string(b) != "not ours" {
			t.Fatalf("%s: somebody else's file was touched: %q %v", goos, b, err)
		}
		entries, _ := os.ReadDir(filepath.Dir(precious))
		if len(entries) != 1 {
			t.Errorf("%s: files were added beside somebody else's: %v", goos, entries)
		}
		if err := os.RemoveAll(foreign); err != nil {
			t.Fatal(err)
		}

		// Our own: installed, installed again over itself, removed.
		for i := 0; i < 2; i++ {
			if _, _, err := s.Install(nil); err != nil {
				t.Fatalf("%s: install %d: %v", goos, i+1, err)
			}
		}
		if removed, err := s.Uninstall(); err != nil || !removed {
			t.Fatalf("%s: removed=%v err=%v", goos, removed, err)
		}
		if _, err := os.Lstat(s.Path); !os.IsNotExist(err) {
			t.Errorf("%s: the shortcut is still there", goos)
		}
	}
}

// What a .lnk of ours is known by: the mark in its description, as the
// shell writes it (UTF-16) or plain. The name of the program alone proves
// nothing — any shortcut somebody made to execai has it.
func TestShortcut_OursLnk(t *testing.T) {
	dir := t.TempDir()
	utf16 := func(s string) []byte {
		var b []byte
		for _, r := range s {
			b = append(b, byte(r), 0)
		}
		return b
	}
	write := func(name string, body []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, append([]byte("L\x00\x00\x00 header "), body...), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	marked := write("ExecAI.lnk", append(utf16(`The agent panel (execai-ui-shortcut)`), utf16(`C:\Programs\execai\execai.exe`)...))
	plain := write("Plain.lnk", []byte("The agent panel (execai-ui-shortcut) C:\\Programs\\execai\\execai.exe"))
	handmade := write("Mine.lnk", utf16(`C:\Programs\execai\execai.exe ui --cwd D:\work`))
	other := write("Other.lnk", utf16(`C:\Programs\other\other.exe`))
	for p, want := range map[string]bool{marked: true, plain: true, handmade: false, other: false} {
		if got := ours_(p); got != want {
			t.Errorf("%s: ours=%v, want %v", filepath.Base(p), got, want)
		}
	}
	s := Shortcut{Path: handmade, Remove: []string{handmade}}
	if removed, err := s.Uninstall(); !errors.Is(err, ErrNotOurs) || removed {
		t.Errorf("a shortcut to execai somebody made by hand: removed=%v err=%v", removed, err)
	}
}

// Install replaces what is at its files' places and never writes through
// it: a link there — to a file of the person's, put by a restore of their
// settings or by their own hand — is replaced by the file, and what it
// pointed at stays as it was. A link in the place of the shortcut itself is
// not a shortcut of ours at all.
func TestShortcut_NoWritingThroughLinks(t *testing.T) {
	home := t.TempDir()
	spec := ShortcutSpec{GOOS: "linux", Home: home, ConfigDir: filepath.Join(home, "cfg"),
		Getenv: func(string) string { return "" }, Exe: "/usr/local/bin/execai", Comment: "c"}
	s, err := PlanShortcut(spec)
	if err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "precious")
	if err := os.WriteFile(victim, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	var icon string
	for _, f := range s.Files {
		if strings.HasSuffix(f.Path, ".png") {
			icon = f.Path
		}
	}
	if err := os.MkdirAll(filepath.Dir(icon), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, icon); err != nil {
		t.Skip("no links here")
	}
	if _, _, err := s.Install(nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "untouched" {
		t.Fatalf("the icon was written through a link: %q", b)
	}
	if li, err := os.Lstat(icon); err != nil || li.Mode()&os.ModeSymlink != 0 || li.Mode().Perm() != 0o644 {
		t.Fatalf("the icon after Install: %v %v", li, err)
	}
	if _, err := s.Uninstall(); err != nil {
		t.Fatal(err)
	}

	// The shortcut's own place taken by a link.
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	marked := filepath.Join(t.TempDir(), "looks-like-ours.desktop")
	if err := os.WriteFile(marked, []byte("[Desktop Entry]\nX-ExecAI-Shortcut=execai-ui-shortcut\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(marked, s.Path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Install(nil); !errors.Is(err, ErrNotOurs) {
		t.Errorf("Install over a link: %v", err)
	}
	if removed, err := s.Uninstall(); !errors.Is(err, ErrNotOurs) || removed {
		t.Errorf("Uninstall of a link: removed=%v err=%v", removed, err)
	}
	if b, _ := os.ReadFile(marked); !strings.HasPrefix(string(b), "[Desktop Entry]\nX-ExecAI") {
		t.Errorf("the file behind the link was rewritten: %q", b)
	}
}

// desktopExec reads an Exec value the way the Desktop Entry Specification
// says a launcher does: the value is a string (\\ is a backslash), split
// into arguments at spaces outside double quotes; inside quotes a backslash
// escapes the next character; %% is a percent sign.
func desktopExec(t *testing.T, value string) []string {
	t.Helper()
	var str strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			i++
			switch value[i] {
			case 's':
				str.WriteByte(' ')
			case 'n':
				str.WriteByte('\n')
			case 't':
				str.WriteByte('\t')
			case 'r':
				str.WriteByte('\r')
			default:
				str.WriteByte(value[i])
			}
			continue
		}
		str.WriteByte(value[i])
	}
	var args []string
	var cur strings.Builder
	in, quoted := false, false
	line := str.String()
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quoted && c == '\\' && i+1 < len(line):
			i++
			cur.WriteByte(line[i])
		case c == '"':
			quoted, in = !quoted, true
		case c == ' ' && !quoted:
			if in {
				args = append(args, cur.String())
				cur.Reset()
				in = false
			}
		case c == '%' && i+1 < len(line) && line[i+1] == '%':
			i++
			cur.WriteByte('%')
			in = true
		case c == '%':
			t.Fatalf("a field code in %q", value)
		default:
			cur.WriteByte(c)
			in = true
		}
	}
	if quoted {
		t.Fatalf("an open quote in %q", value)
	}
	if in {
		args = append(args, cur.String())
	}
	return args
}

// hostileNames — folder names a shell, a desktop file or a batch file would
// like to take for something else.
var hostileNames = []string{
	`plain`, `with space`, `q"uote`, `it's`, "back`tick`", `$(touch PWNED)`, `semi;touch PWNED2`, `amp&touch PWNED3`,
	`100%f %u %% %c`, `back\slash\`, `папка проекта`, `-dash`, `%EXECAI%!x!^y`, `a'; touch PWNED4; '`, `tab\there`,
}

// The folder reaches execai as it is, whatever its name: read back from the
// desktop entry by the rules of its format.
func TestShortcut_LinuxHostileNames(t *testing.T) {
	const exe = `/opt/bin dir/exe'c$ai`
	for _, name := range hostileNames {
		cwd := "/projects/" + name
		s, err := PlanShortcut(ShortcutSpec{GOOS: "linux", Home: "/home/u", ConfigDir: "/home/u/cfg", Exe: exe, Cwd: cwd, Comment: "c"})
		if err != nil {
			t.Fatal(err)
		}
		var exec string
		for _, line := range strings.Split(string(fileOf(t, s, s.Path).Data), "\n") {
			if v, ok := strings.CutPrefix(line, "Exec="); ok {
				exec = v
			}
		}
		if got, want := desktopExec(t, exec), []string{exe, "ui", "--cwd", cwd}; !slices.Equal(got, want) {
			t.Errorf("%q: the launcher would run %q, want %q\n(Exec=%s)", name, got, want, exec)
		}
		if n := strings.Count(string(fileOf(t, s, s.Path).Data), "\n"); n != 11 {
			t.Errorf("%q: the entry has %d lines: a name broke a line", name, n)
		}
	}
}
