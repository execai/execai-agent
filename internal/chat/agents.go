// Subagents in the terminal: the per-turn runner, the "Agents" block above
// the input, the status-line counter, /agents, and the automatic message
// when background subagents finish.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/fleet"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llmpick"
	"github.com/velesbsdllc/agent-vbai/internal/sessions"
	"github.com/velesbsdllc/agent-vbai/internal/subagent"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// fleetMsg — a new snapshot of the registry (at most two a second).
type fleetMsg struct{ recs []fleet.Record }

// subagentDoneMsg — background subagents of parent finished and nobody
// waited for them: hand their results to the agent when it is free.
type subagentDoneMsg struct{ parent string }

// agentApproveCancelMsg — the subagent whose question is on screen is gone
// (stopped, timed out): take the dialog down.
type agentApproveCancelMsg struct{ reply chan agent.ApproveDecision }

// tuiAskSem lets one permission question at a time onto the screen: the
// main agent and write subagents may ask at once, and the dialog has room
// for one.
var tuiAskSem = make(chan struct{}, 1)

// startFleet wires the registry to the screen and the server. The returned
// func stops subagents (they end "cancelled") and sends the last report.
func (m *tuiModel) startFleet(cwd string) func() {
	m.fleet = fleet.New()
	tools.SetSubagentDefault(m.cfg.SubagentIterations)
	m.agents = subagent.NewManager(m.fleet)
	unsub := m.fleet.Subscribe(func(recs []fleet.Record) {
		if p := teaProgramRef; p != nil {
			p.Send(fleetMsg{recs: recs})
		}
	})
	apiBase := ""
	if m.cfg != nil {
		apiBase = m.cfg.APIBase
	}
	rep := fleet.StartReporter(m.fleet, fleet.HTTPSender(apiBase, fleet.SavedToken), cwd, 0)
	return func() {
		unsub()
		m.agents.Shutdown(2 * time.Second)
		rep.Close()
		m.fleet.Close()
	}
}

// subagentDeps is the snapshot of this turn that its subagents inherit.
func (m *tuiModel) subagentDeps(ap agent.Approver) subagent.Deps {
	token := ""
	if m.creds != nil {
		token = m.creds.Token
	}
	source := "ExecAI"
	if m.subs != nil {
		source = m.subs.SourceLabel()
	}
	parent := ""
	if m.session != nil {
		parent = m.session.ID
	}
	prog := teaProgramRef
	return subagent.Deps{
		Client: m.cli, Source: source, Model: m.current.ID,
		Picker:   llmpick.NewPicker(m.cfg, m.subs, m.execAIModels, token),
		Cwd:      getCWDForBoot(),
		Approver: ap, ParentChat: parent,
		Notify: func(p string) {
			if prog != nil {
				prog.Send(subagentDoneMsg{parent: p})
			}
		},
	}
}

// agentsBusy — the agent cannot take an automatic message right now.
func (m *tuiModel) agentsBusy() bool {
	return m.streaming || m.approving || m.asking || m.loginMode || m.compacting
}

// onSubagentDone hands finished background results to the agent as a new
// turn, or retries shortly while the agent is busy.
func (m *tuiModel) onSubagentDone(msg subagentDoneMsg) (tea.Model, tea.Cmd) {
	if m.agents == nil || m.session == nil || msg.parent != m.session.ID {
		// Another chat's subagents: their results stay in /agents.
		return m, nil
	}
	if m.agentsBusy() {
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return msg })
	}
	text := m.agents.TakeFinished(msg.parent)
	if text == "" {
		return m, nil
	}
	m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: i18n.T("fleet.tui.autoReply")})
	m.uiSegments = append(m.uiSegments, segment{kind: "user", body: "🤖 " + firstLines(text, 6)})
	m.refreshViewport()
	return m, m.startAgent(text)
}

// relevant — the records the block and the counter show: this chat's
// subagents, and anything still working.
func (m *tuiModel) relevantAgents() []fleet.Record {
	var out []fleet.Record
	cur := ""
	if m.session != nil {
		cur = m.session.ID
	}
	for _, r := range m.fleetRecs {
		if r.ParentChat == cur || r.State.Active() {
			out = append(out, r)
		}
	}
	return out
}

// stateIcon — one glyph per state.
func stateIcon(s fleet.State) string {
	switch s {
	case fleet.Queued:
		return "…"
	case fleet.Running:
		return "▶"
	case fleet.Waiting:
		return "⏸"
	case fleet.Done:
		return "✓"
	default:
		return "✗"
	}
}

// agentsStatus — the status-line item: "2▶ 1✓ 1✗"; "" when there is nothing.
func (m *tuiModel) agentsStatus() string {
	recs := m.relevantAgents()
	if len(recs) == 0 {
		return ""
	}
	active, done, failed := fleet.Counts(recs)
	var parts []string
	if active > 0 {
		parts = append(parts, fmt.Sprintf("%d▶", active))
	}
	if done > 0 {
		parts = append(parts, fmt.Sprintf("%d✓", done))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d✗", failed))
	}
	return strings.Join(parts, " ")
}

// renderAgentsBlock — the live block above the input while subagents work:
// state, title, step, last tool, time. Finished ones of the same batch stay
// in it until the last one ends, so the person sees the whole picture.
func (m *tuiModel) renderAgentsBlock() string {
	recs := m.relevantAgents()
	active, _, _ := fleet.Counts(recs)
	if active == 0 {
		return ""
	}
	var shown []fleet.Record
	for _, r := range recs {
		if r.State.Active() || (r.FinishedAt != nil && time.Since(*r.FinishedAt) < 10*time.Minute) {
			shown = append(shown, r)
		}
	}
	const maxRows = 6
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5FAFFF"))
	dim := styleHelpDim
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAF00"))
	var b strings.Builder
	b.WriteString(head.Render(i18n.T("fleet.tui.header")))
	b.WriteString("\n")
	start := 0
	if len(shown) > maxRows {
		start = len(shown) - maxRows
	}
	now := time.Now()
	for _, r := range shown[start:] {
		line := fmt.Sprintf(" %s %s", stateIcon(r.State), truncate(r.Title, 40))
		meta := []string{subagent.StateLabel(r.State)}
		steps, low := stepsLabel(r)
		if steps != "" && !low {
			meta = append(meta, steps)
		}
		if r.LastTool != "" && r.State.Active() {
			meta = append(meta, r.LastTool)
		}
		meta = append(meta, shortDuration(r.Elapsed(now)))
		b.WriteString(line + "  " + dim.Render(strings.Join(meta, " · ")))
		if low {
			b.WriteString(dim.Render(" · ") + warn.Render(steps+" — "+i18n.Tf("fleet.tui.moreHint", r.ID)))
		}
		b.WriteString("\n")
	}
	if start > 0 {
		b.WriteString(dim.Render(i18n.Tf("fleet.tui.more", start)))
		b.WriteString("\n")
	}
	return lipgloss.NewStyle().MaxWidth(max(m.width, 20)).Render(strings.TrimRight(b.String(), "\n")) + "\n"
}

// lowSteps — at this many steps left a working subagent is flagged, so the
// person can give it more before it stops.
const lowSteps = 5

// stepsLabel — "step 12/50 · 38 left" for a working subagent, "12/50" for a
// finished one, plus a note when it hit its limit. low: few steps left.
func stepsLabel(r fleet.Record) (label string, low bool) {
	if r.MaxIter <= 0 {
		return "", false
	}
	if r.State.Active() {
		left := max(r.MaxIter-r.Iter, 0)
		return i18n.Tf("fleet.tui.steps", r.Iter, r.MaxIter, left), left <= lowSteps
	}
	label = fmt.Sprintf("%d/%d", r.Iter, r.MaxIter)
	if r.Exhausted {
		label += " · " + i18n.T("fleet.tui.atLimit")
	}
	return label, false
}

func shortDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// firstLines keeps the first n lines of s.
func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		return strings.Join(lines[:n], "\n") + "\n…"
	}
	return s
}

// agentsCommand handles /agents, /agents open <n>, /agents stop <n>,
// /agents more <n> [steps], /agents limit [N].
func (m *tuiModel) agentsCommand(arg string) string {
	if m.fleet == nil {
		return i18n.T("fleet.tui.listEmpty")
	}
	recs := m.fleet.Snapshot()
	fields := strings.Fields(arg)
	pick := func(ref string) (fleet.Record, bool) {
		if n, err := strconv.Atoi(ref); err == nil && n >= 1 && n <= len(recs) {
			return recs[n-1], true
		}
		for _, r := range recs {
			if r.ID == ref {
				return r, true
			}
		}
		return fleet.Record{}, false
	}
	switch {
	case len(fields) == 0 || fields[0] == "list":
		return formatAgentsList(recs, time.Now())
	case len(fields) == 2 && fields[0] == "open":
		rec, ok := pick(fields[1])
		if !ok {
			return i18n.Tf("fleet.tui.notFound", fields[1])
		}
		_, sess, err := m.agents.Transcript(rec.ID)
		if err != nil {
			return err.Error()
		}
		return formatTranscript(rec, sess)
	case len(fields) == 2 && fields[0] == "stop":
		rec, ok := pick(fields[1])
		if !ok {
			return i18n.Tf("fleet.tui.notFound", fields[1])
		}
		if !m.agents.Stop(rec.ID) {
			return i18n.Tf("fleet.tui.notRunning", rec.Title)
		}
		return i18n.Tf("fleet.tui.stopping", rec.Title)
	case (len(fields) == 2 || len(fields) == 3) && fields[0] == "more":
		rec, ok := pick(fields[1])
		if !ok {
			return i18n.Tf("fleet.tui.notFound", fields[1])
		}
		steps := 0
		if len(fields) == 3 {
			n, err := strconv.Atoi(fields[2])
			if err != nil || n <= 0 {
				return i18n.T("fleet.tui.usage")
			}
			steps = n
		}
		msg, err := m.agents.Extend(rec.ID, steps)
		if err != nil {
			return err.Error()
		}
		return msg
	case len(fields) == 1 && fields[0] == "limit":
		return i18n.Tf("fleet.tui.limitShow", tools.SubagentDefault(), tools.SubagentDefaultIterations,
			tools.SubagentMaxIterations)
	case len(fields) == 2 && fields[0] == "limit":
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 1 || n > tools.SubagentMaxIterations {
			return i18n.Tf("fleet.tui.limitBad", tools.SubagentMaxIterations)
		}
		m.cfg.SubagentIterations = n
		if err := config.Save(m.cfg); err != nil {
			return err.Error()
		}
		tools.SetSubagentDefault(n)
		return i18n.Tf("fleet.tui.limitSet", n)
	}
	return i18n.T("fleet.tui.usage")
}

// formatAgentsList — /agents: every subagent of this process, numbered,
// with its state, model, steps, tokens and the start of its result.
func formatAgentsList(recs []fleet.Record, now time.Time) string {
	if len(recs) == 0 {
		return i18n.T("fleet.tui.listEmpty")
	}
	var b strings.Builder
	b.WriteString(i18n.T("fleet.tui.listHeader"))
	b.WriteString("\n")
	for i, r := range recs {
		steps, _ := stepsLabel(r)
		fmt.Fprintf(&b, "%2d. %s %s  [%s · %s · %s · %s", i+1, stateIcon(r.State), r.Title,
			subagent.StateLabel(r.State), r.Mode, steps, shortDuration(r.Elapsed(now)))
		if r.Model != "" {
			b.WriteString(" · " + r.Model)
		}
		if r.PromptTokens > 0 {
			fmt.Fprintf(&b, " · %d tok", r.PromptTokens)
		}
		b.WriteString("]  " + r.ID + "\n")
		body := r.Result
		if r.Error != "" {
			body = r.Error
		} else if body == "" {
			body = r.Preview
		}
		if body = strings.TrimSpace(body); body != "" {
			b.WriteString("    " + strings.ReplaceAll(truncate(body, 300), "\n", "\n    ") + "\n")
		}
	}
	b.WriteString(i18n.T("fleet.tui.listHint"))
	return b.String()
}

// formatTranscript — /agents open: what the subagent was asked and did.
func formatTranscript(rec fleet.Record, sess *sessions.Session) string {
	var b strings.Builder
	b.WriteString(i18n.Tf("fleet.tui.openHeader", rec.Title, subagent.StateLabel(rec.State), rec.ID))
	b.WriteString("\n")
	lines := subagent.TranscriptOf(sess, 400)
	if len(lines) == 0 {
		b.WriteString(i18n.T("fleet.tui.openEmpty"))
		return b.String()
	}
	for _, l := range lines {
		switch l.Role {
		case "user":
			b.WriteString("\n› " + l.Text + "\n")
		case "assistant":
			b.WriteString("\n● " + l.Text + "\n")
		case "tool":
			b.WriteString("▶ " + truncate(l.Text, 200) + "\n")
		case "tool_result":
			b.WriteString("  " + strings.ReplaceAll(l.Text, "\n", "\n  ") + "\n")
		}
	}
	if rec.Error != "" {
		b.WriteString("\n✗ " + rec.Error + "\n")
	}
	return b.String()
}

// taskCardResult — the line under a Task card when the subagent is done.
func taskCardResult(title, result string, err error) string {
	icon, state := "✓", subagent.StateLabel(fleet.Done)
	body := result
	if err != nil {
		icon, state = "✗", subagent.StateLabel(fleet.Failed)
		body = err.Error()
		if result != "" {
			body += "\n" + result
		}
	}
	head := i18n.Tf("fleet.tui.cardDone", icon, title, state)
	if body = strings.TrimSpace(body); body == "" {
		return head
	}
	return head + "\n" + truncate(body, 600)
}

// taskTitle — the title of a Task call from its arguments.
func taskTitle(args json.RawMessage) string {
	var p struct {
		Description string `json:"description"`
	}
	_ = json.Unmarshal(args, &p)
	if t := strings.TrimSpace(p.Description); t != "" {
		return t
	}
	return "Task"
}

// AskApproveAs shows a write subagent's question in the same dialog, marked
// with the subagent's title. If the subagent is gone before the person
// answers, the dialog is taken down and the answer is "no".
func (a *tuiApprover) AskApproveAs(ctx context.Context, who agent.Asker, name string, args json.RawMessage, summary string) agent.ApproveDecision {
	if a.prog == nil {
		return agent.ApproveDeny
	}
	select {
	case tuiAskSem <- struct{}{}:
	case <-ctx.Done():
		return agent.ApproveDeny
	}
	defer func() { <-tuiAskSem }()
	reply := make(chan agent.ApproveDecision, 1)
	a.prog.Send(agentApproveAskMsg{name: name, args: args, summary: summary, reply: reply, agent: who.Title})
	select {
	case d := <-reply:
		return d
	case <-ctx.Done():
		a.prog.Send(agentApproveCancelMsg{reply: reply})
		return agent.ApproveDeny
	}
}
