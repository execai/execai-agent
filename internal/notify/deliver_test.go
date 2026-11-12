package notify

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// fakeTool puts an executable called name into dir. It records its
// arguments (one per line) and the two notification variables into
// <dir>/<name>.log, and runs extra after that (a sleep, a failure).
func fakeTool(t *testing.T, dir, name, extra string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	log := filepath.Join(dir, name+".log")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\"; done > '" + log + "'\n" +
		"printf 'TITLE=%s\\nTEXT=%s\\n' \"$EXECAI_NOTIFY_TITLE\" \"$EXECAI_NOTIFY_TEXT\" >> '" + log + "'\n" +
		extra + "\n"
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return log
}

// envOf — an environment with the given variables and the real PATH lookup
// over dir only.
func envOf(goos, dir string, vars map[string]string) Env {
	return Env{
		GOOS:   goos,
		Getenv: func(k string) string { return vars[k] },
		LookPath: func(name string) (string, error) {
			p := filepath.Join(dir, name)
			st, err := os.Stat(p)
			if err != nil || st.Mode()&0o111 == 0 {
				return "", os.ErrNotExist
			}
			return p, nil
		},
	}
}

func readLog(t *testing.T, p string) []string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("the tool did not run: %v", err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

var msg = Message{Title: "ExecAI", Body: "ExecAI: turn finished — proj"}

// Linux with a desktop session: notify-send with the text as an argument
// after "--", plus the bell in the terminal; no terminal sequence on top of
// the desktop notification (it would be a second popup).
func TestSend_LinuxDesktop(t *testing.T) {
	dir := t.TempDir()
	log := fakeTool(t, dir, "notify-send", "")
	var term bytes.Buffer
	n := New(ModeSystem, envOf("linux", dir, map[string]string{"DISPLAY": ":0", "TERM": "xterm-256color"}), &term)
	n.Send(msg)
	got := readLog(t, log)
	want := []string{"--app-name=ExecAI", "--", msg.Body}
	if strings.Join(got[:3], "|") != strings.Join(want, "|") {
		t.Fatalf("notify-send args %q, want %q", got[:3], want)
	}
	if term.String() != "\a" {
		t.Fatalf("terminal got %q, want only the bell", term.String())
	}
}

// Wayland alone is a desktop session too.
func TestChoose_Wayland(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "notify-send", "")
	p := Choose(ModeSystem, envOf("linux", dir, map[string]string{"WAYLAND_DISPLAY": "wayland-0"}))
	if p.kind != kindNotifySend {
		t.Fatalf("plan %+v", p)
	}
}

// No desktop session, or no notify-send: the terminal sequence and the bell.
func TestChoose_LinuxNoDesktop(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "notify-send", "")
	for name, vars := range map[string]map[string]string{
		"no display": {"TERM": "xterm-256color"},
		"over ssh":   {"DISPLAY": "localhost:10.0", "SSH_CONNECTION": "10.0.0.1 5000 10.0.0.2 22"},
		"ssh tty":    {"DISPLAY": ":0", "SSH_TTY": "/dev/pts/3"},
	} {
		p := Choose(ModeSystem, envOf("linux", dir, vars))
		if p.Tool != "" || p.Seq != seqOSC9 || !p.Bell {
			t.Errorf("%s: plan %+v", name, p)
		}
	}
	empty := t.TempDir()
	p := Choose(ModeSystem, envOf("linux", empty, map[string]string{"DISPLAY": ":0"}))
	if p.Tool != "" || p.Seq != seqOSC9 || !p.Bell {
		t.Errorf("no notify-send: plan %+v", p)
	}
}

// macOS: osascript gets a fixed script and the texts as arguments — no text
// is ever pasted into AppleScript source.
func TestSend_MacOS(t *testing.T) {
	dir := t.TempDir()
	log := fakeTool(t, dir, "osascript", "")
	var term bytes.Buffer
	evil := Message{Title: "ExecAI", Body: `ExecAI: turn finished — a" & (do shell script "id") & "`}
	New(ModeSystem, envOf("darwin", dir, map[string]string{"TERM_PROGRAM": "Apple_Terminal"}), &term).Send(evil)
	got := readLog(t, log)
	want := []string{"-e", "on run argv", "-e", "display notification (item 1 of argv) with title (item 2 of argv)", "-e", "end run", evil.Body, "ExecAI"}
	if strings.Join(got[:len(want)], "|") != strings.Join(want, "|") {
		t.Fatalf("osascript args %q", got)
	}
	for _, a := range got[:6] {
		if strings.Contains(a, "do shell script") {
			t.Fatal("text leaked into the script")
		}
	}
	if term.String() != "\a" {
		t.Fatalf("terminal got %q", term.String())
	}
}

// Windows: a fixed PowerShell script, base64-encoded; the texts come in
// through environment variables and become XML text nodes there.
func TestSend_Windows(t *testing.T) {
	dir := t.TempDir()
	log := fakeTool(t, dir, "powershell.exe", "")
	var term bytes.Buffer
	evil := Message{Title: "ExecAI", Body: "ExecAI: turn finished — <x>&'$(calc)'"}
	New(ModeSystem, envOf("windows", dir, map[string]string{}), &term).Send(evil)
	got := readLog(t, log)
	if len(got) < 6 || got[0] != "-NoProfile" || got[1] != "-NonInteractive" || got[2] != "-EncodedCommand" {
		t.Fatalf("powershell args %q", got)
	}
	raw, err := base64.StdEncoding.DecodeString(got[3])
	if err != nil || len(raw)%2 != 0 {
		t.Fatalf("not base64 UTF-16: %v", err)
	}
	u := make([]uint16, len(raw)/2)
	for i := range u {
		u[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	script := string(utf16.Decode(u))
	if !strings.Contains(script, "$env:EXECAI_NOTIFY_TEXT") || !strings.Contains(script, "CreateTextNode") ||
		strings.Contains(script, "calc") {
		t.Fatalf("script %q", script)
	}
	if got[len(got)-2] != "TITLE=ExecAI" || got[len(got)-1] != "TEXT="+evil.Body {
		t.Fatalf("variables %q", got[len(got)-2:])
	}
	if term.String() != "\a" {
		t.Fatalf("terminal got %q", term.String())
	}
}

// A terminal known to show notifications itself gets its sequence and no
// desktop popup — the two would duplicate each other.
func TestChoose_KnownTerminals(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "notify-send", "")
	fakeTool(t, dir, "osascript", "")
	cases := []struct {
		goos string
		vars map[string]string
		seq  seq
	}{
		{"darwin", map[string]string{"TERM_PROGRAM": "iTerm.app"}, seqOSC9},
		{"linux", map[string]string{"LC_TERMINAL": "iTerm2", "SSH_TTY": "/dev/pts/1"}, seqOSC9},
		{"linux", map[string]string{"DISPLAY": ":0", "TERM_PROGRAM": "WezTerm"}, seqOSC9},
		{"linux", map[string]string{"DISPLAY": ":0", "TERM": "xterm-ghostty"}, seqOSC9},
		{"linux", map[string]string{"DISPLAY": ":0", "TERM": "xterm-kitty"}, seqOSC9},
		{"linux", map[string]string{"DISPLAY": ":0", "TERM": "foot"}, seqOSC777},
	}
	for _, c := range cases {
		p := Choose(ModeSystem, envOf(c.goos, dir, c.vars))
		if p.Tool != "" || p.Seq != c.seq || !p.Bell {
			t.Errorf("%s %v: plan %+v", c.goos, c.vars, p)
		}
	}
}

// Inside tmux or screen a notification sequence goes nowhere: only the
// bell, which the multiplexer passes on; the desktop tool where there is one.
func TestChoose_Multiplexer(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "notify-send", "")
	p := Choose(ModeBell, envOf("linux", dir, map[string]string{"TMUX": "/tmp/tmux-1/default,1,0", "KITTY_WINDOW_ID": "1"}))
	if p.Tool != "" || p.Seq != seqNone || !p.Bell {
		t.Errorf("tmux bell: %+v", p)
	}
	p = Choose(ModeSystem, envOf("linux", dir, map[string]string{"TERM": "screen-256color", "DISPLAY": ":0"}))
	if p.kind != kindNotifySend || p.Seq != seqNone || p.Fallback != seqNone {
		t.Errorf("screen system: %+v", p)
	}
}

// bell: never starts a program; off: nothing at all.
func TestChoose_Modes(t *testing.T) {
	dir := t.TempDir()
	log := fakeTool(t, dir, "notify-send", "")
	env := envOf("linux", dir, map[string]string{"DISPLAY": ":0", "TERM": "xterm-256color"})

	var term bytes.Buffer
	New(ModeBell, env, &term).Send(msg)
	if _, err := os.Stat(log); err == nil {
		t.Fatal("bell started notify-send")
	}
	if term.String() != "\x1b]9;ExecAI: turn finished — proj\a\a" {
		t.Fatalf("bell wrote %q", term.String())
	}

	term.Reset()
	New(ModeOff, env, &term).Send(msg)
	if term.Len() != 0 {
		t.Fatalf("off wrote %q", term.String())
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("off started notify-send")
	}
}

// The desktop tool may hang (a stuck notification daemon): it is cut off at
// the timeout, and the terminal sequence goes out instead.
func TestSend_TimeoutFallsBack(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "notify-send", "sleep 5")
	var term bytes.Buffer
	n := New(ModeSystem, envOf("linux", dir, map[string]string{"DISPLAY": ":0"}), &term)
	n.Timeout = 200 * time.Millisecond
	start := time.Now()
	n.Send(msg)
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("Send blocked for %v", d)
	}
	if term.String() != "\a\x1b]9;ExecAI: turn finished — proj\a" {
		t.Fatalf("terminal got %q", term.String())
	}
}

// A failing tool falls back too, and the reason lands in the debug log only.
func TestSend_FailureLogged(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "notify-send", "exit 3")
	var term bytes.Buffer
	var logged []string
	n := New(ModeSystem, envOf("linux", dir, map[string]string{"DISPLAY": ":0"}), &term)
	n.Log = func(s string) { logged = append(logged, s) }
	n.Send(msg)
	if !strings.HasSuffix(term.String(), "\x1b]9;ExecAI: turn finished — proj\a") {
		t.Fatalf("no fallback: %q", term.String())
	}
	if len(logged) == 0 || !strings.Contains(strings.Join(logged, "\n"), "notify-send") {
		t.Fatalf("log %q", logged)
	}
	if strings.Contains(strings.Join(logged, "\n"), "proj") {
		t.Fatal("the log carries the notification text")
	}
}

// Text from outside (a folder name) cannot break out of the sequence: no
// control characters, no field separators, bounded length.
func TestSequence_Sanitized(t *testing.T) {
	dir := t.TempDir()
	var term bytes.Buffer
	evil := Message{Title: "Exec;AI", Body: "ExecAI: turn finished — a\x1b]0;pwned\a;b\x9c\nc" + strings.Repeat("x", 500)}
	New(ModeBell, envOf("linux", dir, map[string]string{"TERM": "foot"}), &term).Send(evil)
	out := term.String()
	if !strings.HasPrefix(out, "\x1b]777;notify;Exec,AI;ExecAI: turn finished — a]0,pwned,b c") {
		t.Fatalf("sequence %q", out)
	}
	if strings.Count(out, "\x1b") != 1 || strings.Count(out, "\a") != 2 || strings.Contains(out, "\x9c") {
		t.Fatalf("control characters leaked: %q", out)
	}
	if len([]rune(out)) > 260 {
		t.Fatalf("not bounded: %d", len([]rune(out)))
	}
}

// An empty text is not sent at all.
func TestSend_Empty(t *testing.T) {
	var term bytes.Buffer
	New(ModeBell, envOf("linux", t.TempDir(), nil), &term).Send(Message{Title: "ExecAI", Body: " \n"})
	if term.Len() != 0 {
		t.Fatalf("wrote %q", term.String())
	}
}

// Describe names what this machine does — for /notifications.
func TestPlan_Describe(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "notify-send", "")
	for _, c := range []struct {
		mode Mode
		vars map[string]string
		want string
	}{
		{ModeSystem, map[string]string{"DISPLAY": ":0"}, "notify-send + bell"},
		{ModeBell, map[string]string{"DISPLAY": ":0"}, "OSC 9 + bell"},
		{ModeSystem, map[string]string{"TERM": "foot", "DISPLAY": ":0"}, "OSC 777 + bell"},
		{ModeBell, map[string]string{"TMUX": "x"}, "bell"},
		{ModeOff, nil, "—"},
	} {
		if got := Choose(c.mode, envOf("linux", dir, c.vars)).Describe(); got != c.want {
			t.Errorf("%s %v: %q, want %q", c.mode, c.vars, got, c.want)
		}
	}
}

// Desktop: only the desktop program, nothing written anywhere else — for
// execai ide, whose stdout is the editor's protocol.
func TestDesktop(t *testing.T) {
	dir := t.TempDir()
	log := fakeTool(t, dir, "notify-send", "")
	env := envOf("linux", dir, map[string]string{"DISPLAY": ":0", "TERM": "xterm-kitty"})
	p := Desktop(env)
	if p.Tool == "" || p.Seq != seqNone || p.Fallback != seqNone || p.Bell {
		t.Fatalf("plan %+v", p)
	}
	n := NewDesktop(env)
	if n.Out != nil {
		t.Fatal("a terminal writer")
	}
	n.Send(msg)
	if got := readLog(t, log); got[2] != msg.Body {
		t.Fatalf("args %q", got)
	}
	// Over SSH or without a desktop: nothing to use.
	if Desktop(envOf("linux", dir, map[string]string{"DISPLAY": ":0", "SSH_TTY": "/dev/pts/1"})).Tool != "" ||
		Desktop(envOf("linux", dir, nil)).Tool != "" {
		t.Fatal("a desktop program over SSH or without a display")
	}
}

func TestFolderName(t *testing.T) {
	home, _ := os.UserHomeDir()
	for in, want := range map[string]string{"/srv/secret/my-proj": "my-proj", "/srv/x/": "x", home: "~", "": "~"} {
		if in == "" {
			continue
		}
		if got := FolderName(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
