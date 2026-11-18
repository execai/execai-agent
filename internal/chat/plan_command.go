// Subscription gate in the TUI. Only the ExecAI models need a plan. While
// they are the active source the plan is re-asked in the background at
// launch and every 24 h; with an own source billing is not asked at all
// (my_plan may renew, i.e. charge, an expired period) until a switch to the
// ExecAI models, a turn there, or /plan. Every turn asks the gate in its own
// goroutine before the model is called. The logic lives in
// internal/plan — serve and the editor panel use the same rules.
package chat

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
)

// planRefreshedMsg — a background refresh finished. tick: schedule the next
// daily one (only the launch chain does, so ticks never multiply).
type planRefreshedMsg struct{ tick bool }

// planTickMsg — time for the daily refresh.
type planTickMsg struct{}

// planGateMsg — the gate refused a turn; text replaces the model's answer.
type planGateMsg struct{ text string }

// planShowMsg — the /plan report.
type planShowMsg struct{ text string }

// planRefreshCmd re-asks billing off the UI loop — only while the active
// source goes to the ExecAI models; otherwise it just keeps the daily tick.
func (m *tuiModel) planRefreshCmd(tick bool) tea.Cmd {
	c := m.plan
	if c == nil {
		return nil
	}
	own := plan.RouteOf(m.subs).Own
	return func() tea.Msg {
		if !own {
			ctx, cancel := context.WithTimeout(context.Background(), plan.FetchTimeout)
			defer cancel()
			_ = c.Refresh(ctx)
		}
		return planRefreshedMsg{tick: tick}
	}
}

// planEnsureCmd — the switch to the ExecAI models: ask billing unless a
// fresh answer is saved, then announce a refusal right away.
func (m *tuiModel) planEnsureCmd() tea.Cmd {
	c := m.plan
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), plan.FetchTimeout)
		defer cancel()
		c.Ensure(ctx)
		return planRefreshedMsg{}
	}
}

// planAfterSwitch — called after a /source-like command: a move from an own
// source to the ExecAI models is when the plan is first needed.
func (m *tuiModel) planAfterSwitch(wasOwn bool) tea.Cmd {
	if m.plan == nil || !wasOwn || plan.RouteOf(m.subs).Own {
		return nil
	}
	return m.planEnsureCmd()
}

// planShowCmd renders /plan after a fresh look at billing.
func (m *tuiModel) planShowCmd() tea.Cmd {
	c := m.plan
	if c == nil {
		return nil
	}
	return func() tea.Msg { return planShowMsg{text: c.Report(context.Background())} }
}

// planGate snapshots what the gate needs on the UI loop (the source may
// change while a turn runs) and returns the check for the turn's goroutine.
// nil — no gate (tests that build a bare model).
func (m *tuiModel) planGate() func(context.Context) plan.Verdict {
	c := m.plan
	if c == nil {
		return nil
	}
	route := plan.RouteOf(m.subs)
	return func(ctx context.Context) plan.Verdict { return c.Check(ctx, route) }
}

// planAfterError — a 402 from the ExecAI backend most likely means the plan
// has just ended: re-ask billing, and announcePlan says so if it has.
func (m *tuiModel) planAfterError(err error) tea.Cmd {
	if errors.Is(err, llm.ErrPaymentRequired) {
		return m.planRefreshCmd(false)
	}
	return nil
}

// updatePlan handles the gate's messages; ok=false for any other message.
func (m *tuiModel) updatePlan(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case planRefreshedMsg:
		var next tea.Cmd
		if msg.tick {
			next = tea.Tick(plan.RefreshEvery, func(time.Time) tea.Msg { return planTickMsg{} })
		}
		m.announcePlan()
		return next, true
	case planTickMsg:
		return m.planRefreshCmd(true), true
	case planGateMsg:
		m.streaming = false
		m.loopActive = false // a /loop would only repeat the refusal
		m.planWarned = true
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: msg.text})
		m.refreshViewport()
		return nil, true
	case planShowMsg:
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: msg.text})
		m.refreshViewport()
		return nil, true
	}
	return nil, false
}

// announcePlan says once, right away, that turns will be refused — rather
// than after the person has typed a long task. Silent on the login screen
// and while the gate lets the current source through.
func (m *tuiModel) announcePlan() {
	if m.plan == nil || m.loginMode {
		return
	}
	route := plan.RouteOf(m.subs)
	v := plan.Decide(m.plan.Current(), m.plan.SignedIn(), route.Own, time.Now())
	v.Fallback = route.Fallback
	if v.Allowed {
		m.planWarned = false
		return
	}
	if m.planWarned {
		return
	}
	m.planWarned = true
	m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: v.Message()})
	m.refreshViewport()
}
