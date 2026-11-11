package notify

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
)

// Message is one notification. Body is one line that names a folder or a
// tool at most — never a prompt, an answer, a path or a key.
type Message struct {
	Title string
	Body  string
}

// Env is what delivery looks at; System() is the real one, tests pass their
// own.
type Env struct {
	GOOS     string
	Getenv   func(string) string
	LookPath func(string) (string, error)
}

// System — this process: its OS, its environment, its PATH.
func System() Env {
	return Env{GOOS: runtime.GOOS, Getenv: os.Getenv, LookPath: exec.LookPath}
}

func (e Env) get(k string) string {
	if e.Getenv == nil {
		return ""
	}
	return e.Getenv(k)
}

// The desktop notification programs.
const (
	kindNotifySend = "notify-send"
	kindOSAScript  = "osascript"
	kindPowerShell = "PowerShell"
)

// seq is a terminal notification sequence.
type seq int

const (
	seqNone seq = iota
	// seqOSC9 — ESC ] 9 ; text BEL: iTerm2, WezTerm, Ghostty, kitty.
	seqOSC9
	// seqOSC777 — ESC ] 777 ; notify ; title ; body BEL: foot (WezTerm and
	// Ghostty read it too).
	seqOSC777
)

// Plan is how a notification is delivered on this machine. It is chosen
// once: the OS and the terminal do not change while the agent runs.
type Plan struct {
	// Tool — the desktop notification program (full path), "" — none.
	Tool string
	kind string
	// Seq — the terminal's own notification, sent with every notification.
	Seq seq
	// Fallback — the sequence sent when the desktop program fails.
	Fallback seq
	// Bell — ring the terminal bell (BEL): most terminals mark the tab or
	// the taskbar entry, and tmux and screen pass it on.
	Bell bool
}

// Desktop names the desktop notification program, "" — none.
func (p Plan) Desktop() string { return p.kind }

// Sequence names the terminal sequence, "" — none.
func (p Plan) Sequence() string {
	switch p.Seq {
	case seqOSC9:
		return "OSC 9"
	case seqOSC777:
		return "OSC 777"
	}
	return ""
}

// Describe — the plan in one technical line ("notify-send + bell").
func (p Plan) Describe() string {
	var parts []string
	if d := p.Desktop(); d != "" {
		parts = append(parts, d)
	}
	if s := p.Sequence(); s != "" {
		parts = append(parts, s)
	}
	if p.Bell {
		parts = append(parts, "bell")
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " + ")
}

// Choose picks the delivery for a mode in an environment.
//
// One popup per event: a terminal known to show notifications itself gets
// its sequence and no desktop program — the two would duplicate each other,
// and the terminal's own notification brings the right window forward. Other
// terminals get the desktop program (mode system, a local desktop session),
// and the sequence only if the program fails. Inside tmux or screen a
// sequence goes nowhere, so there only the bell is left to them.
func Choose(mode Mode, env Env) Plan {
	if mode == ModeOff {
		return Plan{}
	}
	p := Plan{Bell: true}
	s, known := terminalSeq(env)
	if known && s != seqNone {
		p.Seq = s
		return p
	}
	if mode == ModeSystem {
		if tool, kind := desktopTool(env); tool != "" {
			p.Tool, p.kind = tool, kind
			p.Fallback = s
			return p
		}
	}
	p.Seq = s
	return p
}

// Desktop — only the desktop program, nothing through a terminal: for
// execai ide, whose stdout is the editor's protocol. Tool is empty when this
// machine has no desktop session to show it in (over SSH, no display).
func Desktop(env Env) Plan {
	tool, kind := desktopTool(env)
	return Plan{Tool: tool, kind: kind}
}

// terminalSeq — the notification sequence of the terminal; known=false when
// the terminal is not recognized (OSC 9 is then the best guess: unknown OSC
// sequences are swallowed by terminals that do not support them).
func terminalSeq(env Env) (s seq, known bool) {
	term, prog := env.get("TERM"), env.get("TERM_PROGRAM")
	if env.get("TMUX") != "" || env.get("STY") != "" || prog == "tmux" ||
		strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux") {
		return seqNone, true
	}
	switch {
	case prog == "iTerm.app" || env.get("LC_TERMINAL") == "iTerm2",
		prog == "WezTerm",
		prog == "ghostty" || term == "xterm-ghostty",
		term == "xterm-kitty" || env.get("KITTY_WINDOW_ID") != "":
		return seqOSC9, true
	case term == "foot" || strings.HasPrefix(term, "foot-"):
		return seqOSC777, true
	}
	return seqOSC9, false
}

// remote — the agent runs over SSH: a desktop program would show the
// notification on the remote machine's screen, if anywhere.
func remote(env Env) bool {
	return env.get("SSH_CONNECTION") != "" || env.get("SSH_CLIENT") != "" || env.get("SSH_TTY") != ""
}

// desktopTool finds the desktop notification program of a local desktop
// session; "" when there is none.
func desktopTool(env Env) (path, kind string) {
	if remote(env) || env.LookPath == nil {
		return "", ""
	}
	name := ""
	switch env.GOOS {
	case "darwin":
		name, kind = "osascript", kindOSAScript
	case "windows":
		name, kind = "powershell.exe", kindPowerShell
	case "android", "ios", "plan9", "js", "wasip1":
		return "", ""
	default: // Linux and the BSDs: a desktop session is an X or a Wayland display
		if env.get("DISPLAY") == "" && env.get("WAYLAND_DISPLAY") == "" {
			return "", ""
		}
		name, kind = "notify-send", kindNotifySend
	}
	p, err := env.LookPath(name)
	if err != nil || p == "" {
		return "", ""
	}
	return p, kind
}

// Notifier delivers notifications by its Plan.
type Notifier struct {
	Plan Plan
	// Out — the terminal (the TUI's stdout).
	Out io.Writer
	// Timeout — how long the desktop program may run; 0 — the default
	// (2 s, 5 s for PowerShell, which takes long to start cold).
	Timeout time.Duration
	// Log — the debug log (EXECAI_NOTIFY_LOG=<file>); nil — none. It never
	// gets the text of a notification.
	Log func(string)
}

// New — a notifier for a mode in an environment.
func New(mode Mode, env Env, out io.Writer) *Notifier {
	n := &Notifier{Plan: Choose(mode, env), Out: out}
	if p := env.get("EXECAI_NOTIFY_LOG"); p != "" {
		n.Log = fileLog(p)
	}
	return n
}

// NewDesktop — a notifier that only starts the desktop program (Desktop).
func NewDesktop(env Env) *Notifier {
	n := &Notifier{Plan: Desktop(env)}
	if p := env.get("EXECAI_NOTIFY_LOG"); p != "" {
		n.Log = fileLog(p)
	}
	return n
}

// FolderName — a working folder as a notification names it: its last
// element only, "~" for the home folder — never the path.
func FolderName(dir string) string {
	dir = filepath.Clean(dir)
	if home, err := os.UserHomeDir(); err == nil && dir == filepath.Clean(home) {
		return "~"
	}
	return filepath.Base(dir)
}

// Send delivers one notification. It blocks for the desktop program at most
// Timeout, so the caller runs it off the UI loop; every failure is
// swallowed (and written to the debug log).
func (n *Notifier) Send(m Message) {
	if n == nil {
		return
	}
	title, body := clean(m.Title, 60), clean(m.Body, 200)
	if body == "" {
		return
	}
	term := sequence(n.Plan.Seq, title, body)
	if n.Plan.Bell {
		term += "\a"
	}
	n.write(term)
	if n.Plan.Tool == "" {
		n.logf("sent: %s", n.Plan.Describe())
		return
	}
	if err := n.desktop(title, body); err != nil {
		n.logf("%s failed: %v", n.Plan.kind, err)
		n.write(sequence(n.Plan.Fallback, title, body))
		return
	}
	n.logf("sent: %s", n.Plan.Describe())
}

// write puts bytes on the terminal in one call: a notification sequence
// does not move the cursor, and one write is not split by the renderer's.
func (n *Notifier) write(s string) {
	if s == "" || n.Out == nil {
		return
	}
	if _, err := io.WriteString(n.Out, s); err != nil {
		n.logf("terminal write failed: %v", err)
	}
}

func (n *Notifier) timeout() time.Duration {
	if n.Timeout > 0 {
		return n.Timeout
	}
	if n.Plan.kind == kindPowerShell {
		return 5 * time.Second
	}
	return 2 * time.Second
}

// desktop runs the desktop program. The text is never part of a command
// line a shell or an interpreter parses: it is an argument (notify-send,
// osascript's argv) or an environment variable (PowerShell).
func (n *Notifier) desktop(title, body string) error {
	args, extra := desktopArgs(n.Plan.kind, title, body)
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, n.Plan.Tool, args...)
	cmd.Env = append(os.Environ(), extra...)
	cmd.WaitDelay = 500 * time.Millisecond
	hideWindow(cmd)
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// desktopArgs — the arguments and the extra environment of a program.
func desktopArgs(kind, title, body string) (args, env []string) {
	switch kind {
	case kindNotifySend:
		// The summary is plain text (the body would be read as markup);
		// "--" ends the options, whatever the text starts with.
		return []string{"--app-name=" + title, "--", body}, nil
	case kindOSAScript:
		// A fixed script; the texts arrive in argv.
		return []string{
			"-e", "on run argv",
			"-e", "display notification (item 1 of argv) with title (item 2 of argv)",
			"-e", "end run",
			body, title,
		}, nil
	case kindPowerShell:
		return []string{"-NoProfile", "-NonInteractive", "-EncodedCommand", psEncoded},
			[]string{"EXECAI_NOTIFY_TITLE=" + title, "EXECAI_NOTIFY_TEXT=" + body}
	}
	return nil, nil
}

// psScript shows a toast with the stock WinRT API of Windows PowerShell 5.1
// (no modules). The texts come from the environment and become XML text
// nodes, so nothing in them is parsed. The app id is PowerShell's own,
// which Windows 10 and 11 accept without registering anything.
const psScript = `$ErrorActionPreference = 'Stop'
$null = [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]
$null = [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime]
$xml = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$t = $xml.GetElementsByTagName('text')
$null = $t.Item(0).AppendChild($xml.CreateTextNode($env:EXECAI_NOTIFY_TITLE))
$null = $t.Item(1).AppendChild($xml.CreateTextNode($env:EXECAI_NOTIFY_TEXT))
$app = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($app).Show([Windows.UI.Notifications.ToastNotification]::new($xml))
`

// psEncoded — psScript for -EncodedCommand: base64 of UTF-16LE.
var psEncoded = func() string {
	u := utf16.Encode([]rune(psScript))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}()

// sequence builds a terminal notification. It ends with BEL, not ST: the
// Linux console and older parsers end an OSC string only on BEL.
func sequence(s seq, title, body string) string {
	field := func(v string) string { return strings.ReplaceAll(v, ";", ",") }
	switch s {
	case seqOSC9:
		return "\x1b]9;" + field(body) + "\a"
	case seqOSC777:
		return "\x1b]777;notify;" + field(title) + ";" + field(body) + "\a"
	}
	return ""
}

// clean makes text safe for a notification: one line, no control or
// formatting characters (an escape sequence or a direction override cannot
// ride in on a folder name), at most max characters.
func clean(s string, max int) string {
	s = strings.ToValidUTF8(s, "")
	var b strings.Builder
	n := 0
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			r = ' '
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || unicode.Is(unicode.Cf, r):
			continue
		}
		if n == max {
			b.WriteRune('…')
			break
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

func (n *Notifier) logf(format string, args ...any) {
	if n.Log != nil {
		n.Log(fmt.Sprintf(format, args...))
	}
}

var logMu sync.Mutex

// fileLog appends lines to a file; failures are ignored.
func fileLog(path string) func(string) {
	return func(line string) {
		logMu.Lock()
		defer logMu.Unlock()
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintf(f, "%s notify: %s\n", time.Now().UTC().Format(time.RFC3339), line)
	}
}
