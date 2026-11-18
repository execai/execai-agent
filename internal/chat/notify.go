// Notifications of the terminal chat (E105): a turn finished, or the agent
// waits for a permission or an answer, while the person is in another
// window. internal/notify decides when and delivers; this file feeds it the
// chat's events and holds the /notifications command.
//
// The texts name the working folder or the tool at most: never the prompt,
// the answer, the arguments of a call or a path.
package chat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/notify"
)

// notifyCheckMsg — the timer of an open question fired (notify.WaitAfter).
type notifyCheckMsg struct{ id int }

// turnKind — who started a turn.
type turnKind int

const (
	// turnTyped — the person (a typed message, the results of their
	// subagents): its end is announced when they are away.
	turnTyped turnKind = iota
	// turnLoop — a /loop iteration: work the person put in the background.
	turnLoop
	// turnWake — a wake-up the agent scheduled itself (schedule_wakeup).
	turnWake
)

// alerts — the chat's notification state.
type alerts struct {
	watch notify.Watcher
	mode  notify.Mode
	out   *notify.Notifier
	// lastWait — the id of the last question handed to the watcher.
	lastWait int
	// turnKind — who started the running (or the last) turn.
	turnKind turnKind
	// bgTold — an error or the step limit of background work was announced;
	// the next ones of the same streak stay quiet until a clean turn.
	bgTold bool
	// send replaces delivery in tests; nil — out.Send off the UI loop.
	send func(notify.Message)
	now  func() time.Time
}

// alert returns the chat's notification state, made on first use.
func (m *tuiModel) alert() *alerts {
	if m.alerts == nil {
		m.alerts = &alerts{now: time.Now}
		m.alerts.setMode(m.cfg.GetNotifications())
	}
	return m.alerts
}

func (a *alerts) setMode(mode notify.Mode) {
	a.mode = mode
	a.out = notify.New(mode, notify.System(), os.Stdout)
}

// deliver — a command that sends one notification off the UI loop (the
// desktop program may take up to its timeout); nil when off.
func (a *alerts) deliver(body string) tea.Cmd {
	if a.mode == notify.ModeOff || body == "" {
		return nil
	}
	msg := notify.Message{Title: "ExecAI", Body: body}
	if a.send != nil {
		a.send(msg)
		return nil
	}
	out := a.out
	return func() tea.Msg {
		out.Send(msg)
		return nil
	}
}

// waitStarted hands a question to the watcher: a notification now, or the
// timer that checks on it later.
func (a *alerts) waitStarted(body string) tea.Cmd {
	id, now := a.watch.WaitStarted(a.now(), body)
	a.lastWait = id
	if a.mode == notify.ModeOff {
		return nil
	}
	if now {
		return a.deliver(body)
	}
	return tea.Tick(notify.WaitAfter, func(time.Time) tea.Msg { return notifyCheckMsg{id: id} })
}

// observeNotify looks at every message before the chat handles it and
// returns what has to be delivered.
func (m *tuiModel) observeNotify(msg tea.Msg) tea.Cmd {
	a := m.alert()
	switch msg := msg.(type) {
	case tea.FocusMsg:
		a.watch.SetFocus(true, a.now())
	case tea.BlurMsg:
		a.watch.SetFocus(false, a.now())
	case tea.KeyMsg:
		a.watch.Touch(a.now())
	case agentApproveAskMsg:
		return a.waitStarted(i18n.Tf("notify.approve", msg.name))
	case agentAskChoiceMsg:
		return a.waitStarted(i18n.T("notify.ask"))
	case notifyCheckMsg:
		if !m.approving && !m.asking {
			return nil
		}
		if body, ok := a.watch.WaitDue(msg.id); ok {
			return a.deliver(body)
		}
	}
	return nil
}

// turnEndNotify — after the chat handled the end of a turn (it knows by
// then whether a wake-up is scheduled and whether /loop still runs): what of
// it is news for the person.
//
// A turn the person started: "turn finished" when they are away. Background
// work (/loop, wake-ups) is not announced turn by turn — the person put it
// there; only its end (a wake-up that schedules no next one, a loop stopped
// by the plan gate), an error and the step limit are, once per streak.
// Questions inside it are told like any other (observeNotify).
func (m *tuiModel) turnEndNotify(msg tea.Msg) tea.Cmd {
	a := m.alert()
	now := a.now()
	var stopped, failed, refused, exhausted bool
	switch msg := msg.(type) {
	case agentDoneMsg:
		exhausted = msg.exhausted
	case planGateMsg:
		refused = true
	case agentErrMsg:
		stopped = isCancel(msg.err)
		failed = !stopped
	default:
		return nil
	}
	if !a.watch.InTurn() {
		return nil
	}
	folder := projectFolder()
	if a.turnKind == turnTyped {
		if a.watch.TurnEnded(now, stopped) {
			return a.deliver(i18n.Tf("notify.turnDone", folder))
		}
		return nil
	}
	a.watch.TurnEnded(now, stopped)
	var body string
	switch {
	case stopped:
		return nil
	case refused:
		body = i18n.Tf("notify.bgStopped", folder)
	case failed || exhausted:
		if a.bgTold {
			return nil
		}
		body = i18n.Tf("notify.bgError", folder)
		if exhausted && !failed {
			body = i18n.Tf("notify.bgLimit", folder)
		}
	default:
		a.bgTold = false
		if a.turnKind != turnWake || m.wakeScheduled {
			return nil
		}
		body = i18n.Tf("notify.bgDone", folder)
	}
	if !a.watch.Away(now) {
		return nil
	}
	if failed || exhausted {
		a.bgTold = true
	}
	return a.deliver(body)
}

// noteTurnStart — a turn begins; kind says who started it.
func (m *tuiModel) noteTurnStart(kind turnKind) {
	a := m.alert()
	a.turnKind = kind
	a.watch.TurnStarted(a.now(), kind != turnTyped)
}

// noteWaitEnded — the open question was answered or dismissed.
func (m *tuiModel) noteWaitEnded() { m.alert().watch.WaitEnded() }

// isCancel — the person stopped the turn (Ctrl+C).
func isCancel(err error) bool { return errors.Is(err, context.Canceled) }

// projectFolder — the working folder as a notification names it: its last
// element only, "~" for the home folder.
func projectFolder() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "execai"
	}
	return notify.FolderName(cwd)
}

// notifyPlanText — how notifications are delivered here, for the person.
func (a *alerts) planText() string {
	if a.out == nil || a.mode == notify.ModeOff {
		return "—"
	}
	p := a.out.Plan
	var parts []string
	if d := p.Desktop(); d != "" {
		parts = append(parts, d)
	}
	if s := p.Sequence(); s != "" {
		parts = append(parts, s)
	}
	if p.Bell {
		parts = append(parts, i18n.T("notify.bell"))
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " + ")
}

// notificationsCommand — /notifications [system|bell|off|test]. The
// returned command switches the terminal's focus reports with the mode.
func (m *tuiModel) notificationsCommand(arg string) (string, tea.Cmd) {
	a := m.alert()
	word := strings.ToLower(strings.TrimSpace(arg))
	switch word {
	case "":
		return m.notificationsStatus(), nil
	case "test":
		if a.mode == notify.ModeOff {
			return i18n.T("notify.offNow"), nil
		}
		return i18n.Tf("notify.testSent", a.planText()), a.deliver(i18n.T("notify.test"))
	}
	mode, ok := notify.ParseMode(word)
	if !ok {
		return i18n.Tf("notify.unknown", strings.TrimSpace(arg)), nil
	}
	prev := a.mode
	m.cfg.Notifications = string(mode)
	if err := config.Save(m.cfg); err != nil {
		return err.Error(), nil
	}
	a.setMode(mode)
	var cmd tea.Cmd
	switch {
	case prev == notify.ModeOff && mode != notify.ModeOff:
		// Reports start again; whatever was known before is stale.
		a.watch.ResetFocus()
		cmd = tea.EnableReportFocus
	case prev != notify.ModeOff && mode == notify.ModeOff:
		cmd = tea.DisableReportFocus
	}
	return i18n.Tf("notify.set", mode, i18n.T("notify.desc."+string(mode))), cmd
}

// notificationsStatus — the modes, the current one marked, what this
// computer does and whether the terminal reports focus.
func (m *tuiModel) notificationsStatus() string {
	a := m.alert()
	var b strings.Builder
	b.WriteString("\n" + i18n.T("notify.title") + "\n")
	for _, md := range notify.Modes() {
		mark := "  "
		if md == a.mode {
			mark = "▸ "
		}
		fmt.Fprintf(&b, "%s%-7s — %s\n", mark, md, i18n.T("notify.desc."+string(md)))
	}
	if a.mode != notify.ModeOff {
		b.WriteString("\n" + i18n.Tf("notify.here", a.planText()) + "\n")
		if a.watch.Focus() == notify.FocusUnknown {
			b.WriteString(i18n.T("notify.focus.unknown") + "\n")
		} else {
			b.WriteString(i18n.T("notify.focus.known") + "\n")
		}
	}
	b.WriteString("\n" + i18n.T("notify.howto") + "\n")
	return b.String()
}

// filterNotifyOptions — the menu for "/notifications <arg>".
func filterNotifyOptions(prefix string) []suggestItem {
	low := strings.ToLower(prefix)
	var out []suggestItem
	for _, md := range notify.Modes() {
		if strings.HasPrefix(string(md), low) {
			out = append(out, suggestItem{insert: "/notifications " + string(md), label: string(md), hint: "notify.desc." + string(md)})
		}
	}
	if strings.HasPrefix("test", low) {
		out = append(out, suggestItem{insert: "/notifications test", label: "test", hint: "hint.notificationsTest"})
	}
	return out
}
