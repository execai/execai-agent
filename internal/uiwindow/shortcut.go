package uiwindow

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/brand"
)

// A launcher shortcut starts `execai ui` from the desktop's own menu:
//
//	Linux    ~/.local/share/applications/execai-ui.desktop, the icon beside
//	         the other application icons of the person
//	macOS    ~/Applications/ExecAI.app — a folder with a launcher script,
//	         Info.plist and the icon
//	Windows  "ExecAI" in the Start menu: a .lnk written through the shell's
//	         own COM object by PowerShell (no file format of ours to keep
//	         right); where that is not allowed, a .cmd file instead
//
// Without a project the shortcut runs `execai ui --last` (the folder of the
// last run, or the home folder); with one, `execai ui --cwd <folder>` under
// a name and a file of its own.

// ShortcutSpec — what the shortcut is made from.
type ShortcutSpec struct {
	GOOS      string
	Home      string
	ConfigDir string
	Getenv    func(string) string
	// Exe — the absolute path of execai.
	Exe string
	// Cwd — the project folder; empty for the folder of the last run.
	Cwd string
	// Comment — one line about the program, in the person's language.
	Comment string
}

// File — one file of a shortcut.
type File struct {
	Path string
	Data []byte
	Mode os.FileMode
}

// Shortcut — what Install writes and Uninstall takes away.
type Shortcut struct {
	// Path — the shortcut itself, to show to the person.
	Path  string
	Files []File
	// Script — Windows: the command that writes the .lnk, run after Files
	// with Env added to the environment.
	Script []string
	Env    []string
	// Fallback — Windows: written in place of the .lnk when Script fails.
	Fallback *File
	// Remove — what Uninstall deletes: files, and the .app folder on macOS.
	Remove []string
}

const shortcutName = "ExecAI"

// shortcutMark — what a shortcut of ours carries inside: a line of the
// desktop entry and of the .cmd file, the name of a file in the bundle.
// Install and Uninstall touch only what has it (ours_): "ExecAI.app" in
// ~/Applications may be somebody else's program.
const shortcutMark = "execai-ui-shortcut"

// What Install, Uninstall and PlanShortcut refuse with. The texts are for
// logs; the command says them in the person's language.
var (
	// ErrNotOurs — what is at the shortcut's place was not made by Install
	// (a *NotOursError names the place).
	ErrNotOurs = errors.New("not a shortcut made by execai ui --install-shortcut")
	// ErrNoHome — there is no home folder to keep a shortcut in.
	ErrNoHome = errors.New("no home folder: nowhere to keep a shortcut")
	// ErrNoShortcut — this system has no launcher shortcut of ours.
	ErrNoShortcut = errors.New("no launcher shortcut on this system")
)

// NotOursError — Path holds something Install did not make.
type NotOursError struct{ Path string }

func (e *NotOursError) Error() string { return e.Path + ": " + ErrNotOurs.Error() }
func (e *NotOursError) Unwrap() error { return ErrNotOurs }

// PlanShortcut — the shortcut for spec, nothing written yet.
func PlanShortcut(spec ShortcutSpec) (Shortcut, error) {
	if spec.Getenv == nil {
		spec.Getenv = func(string) string { return "" }
	}
	if strings.TrimSpace(spec.Home) == "" {
		// "" + "/Applications/ExecAI.app" is the computer's folder, not the
		// person's.
		return Shortcut{}, ErrNoHome
	}
	title, suffix := shortcutName, ""
	if spec.Cwd != "" {
		suffix = Key(spec.Cwd)[:8]
	}
	switch spec.GOOS {
	case "linux":
		if spec.Cwd != "" {
			title += " — " + baseName(spec.Cwd)
		}
		return linuxShortcut(spec, title, suffix), nil
	case "darwin":
		if spec.Cwd != "" {
			title += " (" + fileSafe(baseName(spec.Cwd)) + ")"
		}
		return macShortcut(spec, title, suffix), nil
	case "windows":
		if spec.Cwd != "" {
			title += " (" + fileSafe(baseName(spec.Cwd)) + ")"
		}
		return windowsShortcut(spec, title), nil
	}
	return Shortcut{}, fmt.Errorf("%w (%s)", ErrNoShortcut, spec.GOOS)
}

// baseName — the last part of a path of either system.
func baseName(path string) string {
	path = strings.TrimRight(path, `/\`)
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		path = path[i+1:]
	}
	return path
}

// fileSafe — a name without the characters file systems refuse.
func fileSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 0x20 {
			return '-'
		}
		return r
	}, s)
}

func linuxShortcut(spec ShortcutSpec, title, suffix string) Shortcut {
	data := spec.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = spec.Home + "/.local/share"
	}
	name := "execai-ui"
	if suffix != "" {
		name += "-" + suffix
	}
	desktop := data + "/applications/" + name + ".desktop"
	icon := data + "/icons/hicolor/256x256/apps/execai-ui.png"
	exec := desktopArg(spec.Exe) + " ui --last"
	if spec.Cwd != "" {
		exec = desktopArg(spec.Exe) + " ui --cwd " + desktopArg(spec.Cwd)
	}
	entry := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Version=1.0\n" +
		"Name=" + desktopString(title) + "\n" +
		"Comment=" + desktopString(spec.Comment) + "\n" +
		"Exec=" + exec + "\n" +
		"Icon=" + desktopString(icon) + "\n" +
		"Terminal=false\n" +
		"Categories=Development;\n" +
		"StartupWMClass=" + WindowClass + "\n" +
		"X-ExecAI-Shortcut=" + shortcutMark + "\n"
	s := Shortcut{Path: desktop, Files: []File{
		{Path: icon, Data: brand.IconPNG, Mode: 0o644},
		{Path: desktop, Data: []byte(entry), Mode: 0o644},
	}, Remove: []string{desktop}}
	if suffix == "" {
		// The icon is shared by the shortcuts of single projects; it goes
		// with the general one.
		s.Remove = append(s.Remove, icon)
	}
	return s
}

// desktopString — a value of a desktop entry: one line, a backslash doubled.
func desktopString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.NewReplacer("\n", " ", "\r", " ").Replace(s)
}

// desktopArg — one argument of an Exec key: in double quotes, with the
// characters the quoting rule names escaped, then as a string value (a
// backslash doubled again) with the field code sign doubled.
func desktopArg(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '`', '$', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return strings.ReplaceAll(desktopString(b.String()), "%", "%%")
}

func macShortcut(spec ShortcutSpec, title, suffix string) Shortcut {
	app := spec.Home + "/Applications/" + title + ".app"
	id := "ru.execai.ui"
	if suffix != "" {
		id += "." + suffix
	}
	args := []string{spec.Exe, "ui", "--last"}
	if spec.Cwd != "" {
		args = []string{spec.Exe, "ui", "--cwd", spec.Cwd}
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shQuote(a)
	}
	// Programs started from the Finder get a bare PATH; the agent runs the
	// person's tools, so it starts through their login shell, which sets the
	// PATH they have in a terminal.
	script := "#!/bin/sh\n" +
		"# ExecAI launcher: the agent panel (execai ui), started with the environment of a login shell.\n" +
		"shell=\"${SHELL:-/bin/zsh}\"\n" +
		"case \"$shell\" in\n" +
		"  */zsh|*/bash|*/sh|*/dash|*/ksh) ;;\n" +
		"  *) shell=/bin/zsh ;;\n" +
		"esac\n" +
		"exec \"$shell\" -l -c 'exec \"$0\" \"$@\"' " + strings.Join(quoted, " ") + "\n"
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>` + xmlText(title) + `</string>
	<key>CFBundleDisplayName</key>
	<string>` + xmlText(title) + `</string>
	<key>CFBundleIdentifier</key>
	<string>` + id + `</string>
	<key>CFBundleExecutable</key>
	<string>execai-ui</string>
	<key>CFBundleIconFile</key>
	<string>icon</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>CFBundleShortVersionString</key>
	<string>1.0</string>
	<key>LSUIElement</key>
	<true/>
</dict>
</plist>
`
	return Shortcut{Path: app, Files: []File{
		{Path: app + "/Contents/Info.plist", Data: []byte(plist), Mode: 0o644},
		{Path: app + "/Contents/MacOS/execai-ui", Data: []byte(script), Mode: 0o755},
		{Path: app + "/Contents/Resources/icon.icns", Data: brand.IconICNS(), Mode: 0o644},
		{Path: app + "/Contents/Resources/" + shortcutMark, Data: []byte("made by execai ui --install-shortcut\n"), Mode: 0o644},
	}, Remove: []string{app}}
}

// shQuote — one word for a POSIX shell.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// lnkScript — PowerShell: the shell's COM object writes the .lnk. Every
// value comes from the environment, so no path or text is ever part of the
// script. WindowStyle 7 — the console of execai starts minimized: the window
// people look at is the panel.
const lnkScript = `$ErrorActionPreference = 'Stop'; ` +
	`$s = (New-Object -ComObject WScript.Shell).CreateShortcut($env:EXECAI_LNK); ` +
	`$s.TargetPath = $env:EXECAI_TARGET; ` +
	`$s.Arguments = $env:EXECAI_ARGS; ` +
	`$s.WorkingDirectory = $env:EXECAI_DIR; ` +
	`$s.IconLocation = $env:EXECAI_ICON; ` +
	`$s.Description = $env:EXECAI_DESC; ` +
	`$s.WindowStyle = 7; ` +
	`$s.Save()`

func windowsShortcut(spec ShortcutSpec, title string) Shortcut {
	appdata := spec.Getenv("APPDATA")
	if appdata == "" {
		appdata = strings.TrimRight(spec.Home, `\`) + `\AppData\Roaming`
	}
	programs := strings.TrimRight(appdata, `\`) + `\Microsoft\Windows\Start Menu\Programs`
	lnk := programs + `\` + title + `.lnk`
	cmd := programs + `\` + title + `.cmd`
	ico := strings.TrimRight(spec.ConfigDir, `\`) + `\ui\execai.ico`
	args, dir := "ui --last", spec.Home
	if spec.Cwd != "" {
		args, dir = "ui --cwd "+winArg(spec.Cwd), spec.Cwd
	}
	// UTF-8 for the paths (chcp 65001): a folder with letters outside the
	// console's code page reaches execai as it is.
	batch := "@echo off\r\n" +
		"rem " + shortcutMark + "\r\n" +
		"chcp 65001 >nul\r\n" +
		"start \"" + shortcutName + "\" /min " + strings.ReplaceAll(winArg(spec.Exe)+" "+args, "%", "%%") + "\r\n"
	s := Shortcut{
		Path:   lnk,
		Files:  []File{{Path: ico, Data: brand.IconICO(), Mode: 0o644}},
		Script: []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", lnkScript},
		Env: []string{
			"EXECAI_LNK=" + lnk,
			"EXECAI_TARGET=" + spec.Exe,
			"EXECAI_ARGS=" + args,
			"EXECAI_DIR=" + dir,
			"EXECAI_ICON=" + ico,
			// The mark of our making, where a .lnk can carry one.
			"EXECAI_DESC=" + spec.Comment + " (" + shortcutMark + ")",
		},
		Fallback: &File{Path: cmd, Data: []byte(batch), Mode: 0o644},
		Remove:   []string{lnk, cmd},
	}
	if spec.Cwd == "" {
		s.Remove = append(s.Remove, ico)
	}
	return s
}

// winArg — a path as one argument of a Windows command line. A backslash
// before the closing quote would escape it, so it is doubled.
func winArg(s string) string {
	if strings.HasSuffix(s, `\`) {
		s += `\`
	}
	return `"` + s + `"`
}

// ours_ — what is at path was made by Install: a bundle with the mark
// file, a desktop entry or a .cmd with the mark line, a .lnk with the mark
// in its description. Never a link: Install makes none. Icons are known by
// their place and name.
func ours_(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	low := strings.ToLower(path)
	switch {
	case strings.HasSuffix(low, ".app"):
		mark, err := os.Lstat(filepath.Join(path, "Contents", "Resources", shortcutMark))
		return err == nil && mark.Mode().IsRegular()
	case strings.HasSuffix(low, ".desktop"), strings.HasSuffix(low, ".cmd"):
		data, err := os.ReadFile(path)
		return err == nil && bytes.Contains(data, []byte(shortcutMark))
	case strings.HasSuffix(low, ".lnk"):
		// The shell writes the description in UTF-16; an older one, plain.
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		wide := make([]byte, 0, 2*len(shortcutMark))
		for _, c := range []byte(shortcutMark) {
			wide = append(wide, c, 0)
		}
		return bytes.Contains(data, wide) || bytes.Contains(data, []byte(shortcutMark))
	}
	return true
}

// exists — something is at path (a link too).
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// Install writes the shortcut. run starts Script (nil when there is none).
// fallback=true: the script could not run and the Fallback file was written
// instead; path is then that file. ErrNotOurs: something else has the
// shortcut's place — it is left as it is.
func (s Shortcut) Install(run func(argv, env []string) error) (path string, fallback bool, err error) {
	places := []string{s.Path}
	if s.Fallback != nil {
		places = append(places, s.Fallback.Path)
	}
	for _, p := range places {
		if exists(p) && !ours_(p) {
			return "", false, &NotOursError{Path: p}
		}
	}
	for _, f := range s.Files {
		if err := writeFile(f); err != nil {
			return "", false, err
		}
	}
	if len(s.Script) == 0 {
		return s.Path, false, nil
	}
	var rerr error
	if run == nil {
		rerr = fmt.Errorf("no way to run %s", s.Script[0])
	} else {
		rerr = run(s.Script, s.Env)
	}
	if rerr == nil {
		return s.Path, false, nil
	}
	if s.Fallback == nil {
		return "", false, rerr
	}
	if err := writeFile(*s.Fallback); err != nil {
		return "", false, err
	}
	return s.Fallback.Path, true, nil
}

// writeFile puts the file in its place whole: written beside it and moved
// over. What was there is replaced, never written into — a link at the
// place is replaced by the file, and what it pointed at is not touched.
func writeFile(f File) error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	return writeAtomic(f.Path, f.Data, f.Mode)
}

// Uninstall deletes what Install wrote, and only that: what is at a
// shortcut's place without our mark stays, and is reported (ErrNotOurs).
// removed=false: nothing of ours was there.
func (s Shortcut) Uninstall() (removed bool, err error) {
	for _, p := range s.Remove {
		if !exists(p) {
			continue
		}
		if !ours_(p) {
			err = &NotOursError{Path: p}
			continue
		}
		var rerr error
		if strings.HasSuffix(p, ".app") {
			rerr = os.RemoveAll(p)
		} else {
			rerr = os.Remove(p)
		}
		if rerr != nil {
			return removed, rerr
		}
		removed = true
	}
	return removed, err
}
