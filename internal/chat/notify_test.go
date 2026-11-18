package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/notify"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// alertModel — a chat model whose notifications land in *sent and whose
// clock is *clock; the working folder is <tmp>/secret-parent/my-proj.
func alertModel(t *testing.T, mode string) (*tuiModel, *[]notify.Message, *time.Time) {
	t.Helper()
	i18n.SetLocale("en")
	dir := filepath.Join(t.TempDir(), "secret-parent", "my-proj")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	m := newBurstTestModel()
	m.cfg.Notifications = mode
	clock := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	var sent []notify.Message
	a := m.alert()
	a.now = func() time.Time { return clock }
	a.send = func(msg notify.Message) { sent = append(sent, msg) }
	return m, &sent, &clock
}

// turn runs one turn the person started: started, then done after d.
func turn(m *tuiModel, clock *time.Time, d time.Duration, end tea.Msg) {
	bgTurn(m, turnTyped, clock, d, end)
}

// bgTurn runs one turn of the given kind.
func bgTurn(m *tuiModel, kind turnKind, clock *time.Time, d time.Duration, end tea.Msg) {
	m.noteTurnStart(kind)
	m.streaming = true
	*clock = clock.Add(d)
	m.Update(end)
}

func bodies(sent []notify.Message) []string {
	out := make([]string, 0, len(sent))
	for _, s := range sent {
		out = append(out, s.Title+"|"+s.Body)
	}
	return out
}

// The window is away: a finished turn is announced with the folder name
// only — nothing of the prompt, the answer or the path above the folder.
func TestNotify_TurnDoneWhileAway(t *testing.T) {
	m, sent, clock := alertModel(t, "")
	m.Update(tea.BlurMsg{})
	m.uiSegments = append(m.uiSegments, segment{kind: "user", body: "deploy with key SECRET-PROMPT-42"})
	hist := []llm.AIMessage{{Role: "user", Content: "deploy with key SECRET-PROMPT-42"}, {Role: "assistant", Content: "done: SECRET-ANSWER"}}
	turn(m, clock, 2*time.Second, agentDoneMsg{history: hist})
	if got := bodies(*sent); len(got) != 1 || got[0] != "ExecAI|ExecAI: turn finished — my-proj" {
		t.Fatalf("sent %q", got)
	}
	for _, s := range *sent {
		for _, bad := range []string{"SECRET", "secret-parent", string(filepath.Separator) + "my-proj"} {
			if strings.Contains(s.Body, bad) || strings.Contains(s.Title, bad) {
				t.Fatalf("notification carries %q: %+v", bad, s)
			}
		}
	}
}

// Focus reports change nothing on screen: a redraw would drop the person's
// text selection.
func TestNotify_FocusDoesNotRedraw(t *testing.T) {
	// Colours on, or the cursor looks the same either way.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	m, _, _ := alertModel(t, "")
	m.textarea.SetValue("draft")
	before := m.View()
	m.Update(tea.BlurMsg{})
	if m.View() != before {
		t.Fatal("blur changed the screen")
	}
	m.Update(tea.FocusMsg{})
	if m.View() != before {
		t.Fatal("focus changed the screen")
	}
}

// In focus: nothing, however long the turn.
func TestNotify_TurnDoneInFocus(t *testing.T) {
	m, sent, clock := alertModel(t, "system")
	m.Update(tea.FocusMsg{})
	turn(m, clock, 5*time.Minute, agentDoneMsg{})
	if len(*sent) != 0 {
		t.Fatalf("sent %q", bodies(*sent))
	}
}

// Focus never reported: only a turn of 30 s or more since the last key.
func TestNotify_TurnDoneFocusUnknown(t *testing.T) {
	m, sent, clock := alertModel(t, "bell")
	turn(m, clock, 10*time.Second, agentDoneMsg{})
	if len(*sent) != 0 {
		t.Fatalf("a short turn: %q", bodies(*sent))
	}
	turn(m, clock, 31*time.Second, agentDoneMsg{})
	if len(*sent) != 1 {
		t.Fatalf("a long turn: %q", bodies(*sent))
	}
}

// A turn the person stopped is not announced; an error ending it is.
func TestNotify_TurnStoppedOrFailed(t *testing.T) {
	m, sent, clock := alertModel(t, "")
	m.Update(tea.BlurMsg{})
	turn(m, clock, time.Minute, agentErrMsg{err: context.Canceled})
	if len(*sent) != 0 {
		t.Fatalf("a stopped turn: %q", bodies(*sent))
	}
	turn(m, clock, time.Minute, agentErrMsg{err: fmt.Errorf("upstream 502")})
	if len(*sent) != 1 || strings.Contains((*sent)[0].Body, "502") {
		t.Fatalf("a failed turn: %q", bodies(*sent))
	}
}

// A permission request while away: at once, with the tool name and not its
// arguments; the timer that follows adds nothing.
func TestNotify_ApproveWhileAway(t *testing.T) {
	m, sent, _ := alertModel(t, "")
	m.Update(tea.BlurMsg{})
	m.noteTurnStart(turnTyped)
	_, cmd := m.Update(agentApproveAskMsg{name: "Bash", summary: "rm -rf /srv/SECRET-DIR",
		args: json.RawMessage(`{"command":"rm -rf /srv/SECRET-DIR"}`), reply: make(chan agent.ApproveDecision, 1)})
	if got := bodies(*sent); len(got) != 1 || got[0] != "ExecAI|ExecAI is waiting for approval: Bash" {
		t.Fatalf("sent %q", got)
	}
	_ = cmd
	m.Update(notifyCheckMsg{id: m.alert().lastWait})
	if len(*sent) != 1 {
		t.Fatalf("told twice: %q", bodies(*sent))
	}
}

// Focus unknown: a question is announced when the timer fires, once; an
// answered one is not.
func TestNotify_AskFocusUnknown(t *testing.T) {
	m, sent, _ := alertModel(t, "")
	m.noteTurnStart(turnTyped)
	_, cmd := m.Update(agentAskChoiceMsg{question: "Drop the SECRET-TABLE?", options: []tools.AskOption{{Label: "yes"}, {Label: "no"}}, reply: make(chan string, 1)})
	if len(*sent) != 0 {
		t.Fatalf("told at once: %q", bodies(*sent))
	}
	if cmd == nil {
		t.Fatal("no timer for the question")
	}
	id := m.alert().lastWait
	m.Update(notifyCheckMsg{id: id})
	m.Update(notifyCheckMsg{id: id})
	if got := bodies(*sent); len(got) != 1 || got[0] != "ExecAI|ExecAI has a question" {
		t.Fatalf("sent %q", got)
	}

	// Answered before the timer: nothing.
	m.Update(agentApproveAskMsg{name: "Write", reply: make(chan agent.ApproveDecision, 1)})
	id = m.alert().lastWait
	m.replyApprove(agent.ApproveOnce)
	m.Update(notifyCheckMsg{id: id})
	if len(*sent) != 1 {
		t.Fatalf("told about an answered question: %q", bodies(*sent))
	}
}

// off: nothing is ever sent, no timer is set.
func TestNotify_Off(t *testing.T) {
	m, sent, clock := alertModel(t, "off")
	m.Update(tea.BlurMsg{})
	turn(m, clock, time.Minute, agentDoneMsg{})
	_, cmd := m.Update(agentApproveAskMsg{name: "Bash", reply: make(chan agent.ApproveDecision, 1)})
	if len(*sent) != 0 || cmd != nil {
		t.Fatalf("off sent %q (timer %v)", bodies(*sent), cmd != nil)
	}
}

// /notifications: shows the modes, switches and saves, refuses an unknown
// word, sends a test one even in focus.
func TestNotificationsCommand(t *testing.T) {
	m, sent, _ := alertModel(t, "")
	m.Update(tea.FocusMsg{})

	m.handleCommand("/notifications")
	out := lastHint(m)
	for _, want := range []string{"system", "bell", "off", "/notifications test"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status lacks %q: %s", want, out)
		}
	}

	_, cmd := m.handleCommand("/notifications off")
	if m.cfg.Notifications != "off" || m.alert().mode != notify.ModeOff {
		t.Fatalf("not switched: %q %q", m.cfg.Notifications, m.alert().mode)
	}
	if cmd == nil || fmt.Sprintf("%T", cmd()) != "tea.disableReportFocusMsg" {
		t.Fatal("focus reports not switched off")
	}
	saved, _ := config.Load()
	if saved.Notifications != "off" {
		t.Fatalf("not saved: %+v", saved)
	}
	m.handleCommand("/notifications test")
	if len(*sent) != 0 || !strings.Contains(lastHint(m), "off") {
		t.Fatalf("test while off: %q / %s", bodies(*sent), lastHint(m))
	}

	_, cmd = m.handleCommand("/notifications bell")
	if m.alert().mode != notify.ModeBell || cmd == nil || fmt.Sprintf("%T", cmd()) != "tea.enableReportFocusMsg" {
		t.Fatal("not switched back on")
	}
	m.handleCommand("/notifications test")
	if got := bodies(*sent); len(got) != 1 || got[0] != "ExecAI|ExecAI: test notification" {
		t.Fatalf("test sent %q", got)
	}

	m.handleCommand("/notifications loud")
	if m.cfg.Notifications != "bell" || !strings.Contains(m.err+lastHint(m), "loud") {
		t.Fatalf("unknown mode: %q %q %q", m.cfg.Notifications, m.err, lastHint(m))
	}

	m.handleCommand("/config")
	if !strings.Contains(lastHint(m), "notifications    : bell") {
		t.Fatalf("/config: %s", lastHint(m))
	}
}

// The menu offers the modes.
func TestNotificationsSuggest(t *testing.T) {
	items := filterNotifyOptions("b")
	if len(items) != 1 || items[0].insert != "/notifications bell" {
		t.Fatalf("items %+v", items)
	}
	if len(filterNotifyOptions("")) != 4 {
		t.Fatal("all four")
	}
}

// /loop iterations are work the person put in the background: finishing one
// is not news; a question inside one, an error, the step limit and a
// stopped loop are.
func TestNotify_LoopIterations(t *testing.T) {
	m, sent, clock := alertModel(t, "")
	m.Update(tea.BlurMsg{})
	m.loopActive = true
	bgTurn(m, turnLoop, clock, time.Minute, agentDoneMsg{})
	bgTurn(m, turnLoop, clock, time.Minute, agentDoneMsg{})
	if len(*sent) != 0 {
		t.Fatalf("iterations announced: %q", bodies(*sent))
	}

	m.noteTurnStart(turnLoop)
	m.Update(agentApproveAskMsg{name: "Bash", reply: make(chan agent.ApproveDecision, 1)})
	m.replyApprove(agent.ApproveOnce)
	m.Update(agentDoneMsg{})
	if got := bodies(*sent); len(got) != 1 || got[0] != "ExecAI|ExecAI is waiting for approval: Bash" {
		t.Fatalf("a question in an iteration: %q", got)
	}

	// Errors: once per streak; a clean iteration starts a new streak.
	bgTurn(m, turnLoop, clock, time.Minute, agentErrMsg{err: fmt.Errorf("upstream 502")})
	bgTurn(m, turnLoop, clock, time.Minute, agentErrMsg{err: fmt.Errorf("upstream 502")})
	bgTurn(m, turnLoop, clock, time.Minute, agentDoneMsg{exhausted: true})
	if got := bodies(*sent); len(got) != 2 || got[1] != "ExecAI|ExecAI: background turn ended with an error — my-proj" {
		t.Fatalf("errors: %q", got)
	}
	bgTurn(m, turnLoop, clock, time.Minute, agentDoneMsg{})
	bgTurn(m, turnLoop, clock, time.Minute, agentDoneMsg{exhausted: true})
	if got := bodies(*sent); len(got) != 3 || got[2] != "ExecAI|ExecAI: background turn hit the step limit — my-proj" {
		t.Fatalf("limit: %q", got)
	}

	// The plan gate stops the loop.
	bgTurn(m, turnLoop, clock, time.Minute, planGateMsg{text: "need a plan"})
	if got := bodies(*sent); len(got) != 4 || got[3] != "ExecAI|ExecAI: background work stopped — my-proj" || m.loopActive {
		t.Fatalf("stopped: %q (loop %v)", got, m.loopActive)
	}

	// The person stopped it: nothing.
	bgTurn(m, turnLoop, clock, time.Minute, agentErrMsg{err: context.Canceled})
	if len(*sent) != 4 {
		t.Fatalf("a stopped iteration: %q", bodies(*sent))
	}
}

// A wake-up that schedules the next one is a step of the chain; the one
// that does not is its end.
func TestNotify_WakeChain(t *testing.T) {
	m, sent, clock := alertModel(t, "")
	m.Update(tea.BlurMsg{})
	if _, err := (&tools.ScheduleWakeupTool{}).Execute(context.Background(),
		json.RawMessage(`{"delay_seconds":60,"reason":"CI"}`)); err != nil {
		t.Fatal(err)
	}
	bgTurn(m, turnWake, clock, time.Minute, agentDoneMsg{})
	if len(*sent) != 0 {
		t.Fatalf("a step of the chain: %q", bodies(*sent))
	}
	bgTurn(m, turnWake, clock, time.Minute, agentDoneMsg{})
	if got := bodies(*sent); len(got) != 1 || got[0] != "ExecAI|ExecAI: background work finished — my-proj" {
		t.Fatalf("end of the chain: %q", got)
	}

	// Looking at the window: the end of the chain is visible, no popup.
	m.Update(tea.FocusMsg{})
	bgTurn(m, turnWake, clock, time.Minute, agentDoneMsg{})
	if len(*sent) != 1 {
		t.Fatalf("in focus: %q", bodies(*sent))
	}
}
