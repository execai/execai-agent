// /start, /machine and /phone in the chat — the getting-started steps, the
// "My machine" card and the link that calls this machine from the phone.
// The facts come from internal/machine, the same snapshot the editor panel
// gets as the machine_state event; the chat only draws them. Everything that
// may touch the network (the folder bindings, a check of the sources) runs
// off the UI loop.
package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	"github.com/velesbsdllc/agent-vbai/internal/machine"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// Marks of the tutorial steps and of the model sources on the card.
const (
	markDone  = "✓"
	markTodo  = "○"
	markAlive = "●"
	markDown  = "○"
)

// styleQR paints the code white on black, so it scans on light terminal
// themes too.
var styleQR = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#000000"))

// machineMsg — what /start, /machine or /phone has to show: hint segments
// in order, and a QR code between the first and the rest.
type machineMsg struct {
	before string
	qr     string
	after  string
}

// isMachineCommand — /start, /machine [check], /phone.
func isMachineCommand(cmd string) bool {
	switch cmd {
	case "/start", "/machine", "/machine check", "/phone":
		return true
	}
	return false
}

// handleMachineCommand runs one of the three commands. The snapshot is taken
// from what the chat holds now; the network part runs in the returned Cmd.
func (m *tuiModel) handleMachineCommand(cmd string) tea.Cmd {
	in := m.machineInputs()
	token, self := m.credsToken(), m.selfAgentID()
	switch cmd {
	case "/start":
		return func() tea.Msg {
			in.Bindings = machine.CountBindings(context.Background(), in.APIBase, token, self)
			return machineMsg{before: m.tutorialText(machine.Build(in))}
		}
	case "/machine":
		checker := m.plan
		return func() tea.Msg {
			in.Plan = ensurePlan(checker, token, in.Plan)
			in.Bindings = machine.CountBindings(context.Background(), in.APIBase, token, self)
			return machineMsg{before: machineCardText(machine.Build(in))}
		}
	case "/machine check":
		m.statusMessage = i18n.T("machine.checking")
		checker := m.plan
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			in.Checks = machine.CheckSources(ctx, in.Subs)
			if checker != nil && token != "" {
				_ = checker.Refresh(ctx)
				in.Plan = checker.Current()
			}
			in.Bindings = machine.CountBindings(ctx, in.APIBase, token, self)
			return machineMsg{before: machineCardText(machine.Build(in))}
		}
	case "/phone":
		checker := m.plan
		return func() tea.Msg {
			in.Plan = ensurePlan(checker, token, in.Plan)
			return phoneMsg(machine.Build(in))
		}
	}
	return nil
}

// ensurePlan — the plan answer for a card the person asked for. Billing is
// asked first when there is no answer for this sign-in (right after
// /login) or it is older than a day — the rule of the sync gate; otherwise
// the saved answer is used. known — what the chat held already.
func ensurePlan(checker *plan.Checker, token string, known *plan.Plan) *plan.Plan {
	if checker == nil || token == "" {
		return known
	}
	ctx, cancel := context.WithTimeout(context.Background(), plan.FetchTimeout)
	defer cancel()
	return checker.EnsureDaily(ctx)
}

// updateMachine shows the result of a machine command; ok=false for any
// other message.
func (m *tuiModel) updateMachine(msg tea.Msg) (tea.Cmd, bool) {
	mm, ok := msg.(machineMsg)
	if !ok {
		return nil, false
	}
	m.statusMessage = ""
	if mm.before != "" {
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: mm.before})
	}
	if mm.qr != "" {
		m.uiSegments = append(m.uiSegments, segment{kind: "qr", body: mm.qr})
	}
	if mm.after != "" {
		m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: mm.after})
	}
	m.refreshViewport()
	return nil, true
}

// machineInputs — the local facts, read on the UI loop: the chat's own
// credentials, a copy of the sources (the check runs on it off the loop),
// the latest plan answer, this machine's files.
func (m *tuiModel) machineInputs() machine.Inputs {
	base := config.Defaults().APIBase
	if m.cfg != nil && m.cfg.APIBase != "" {
		base = m.cfg.APIBase
	}
	var p *plan.Plan
	if m.plan != nil {
		p = m.plan.Current()
	}
	return machine.Local(base, m.creds, copySubs(m.subs), p)
}

// localSnapshot — the snapshot without the network (the first launch).
func (m *tuiModel) localSnapshot() machine.Snapshot {
	return machine.Build(m.machineInputs())
}

func copySubs(s *subscriptions.Store) *subscriptions.Store {
	out := &subscriptions.Store{Subscriptions: map[string]subscriptions.Subscription{}}
	if s == nil {
		return out
	}
	out.Active = s.Active
	for _, sub := range s.List() {
		out.Subscriptions[sub.Provider] = sub
	}
	return out
}

// tutorialText — /start: the five steps with their marks, one line on what
// each is for and the command to type.
func (m *tuiModel) tutorialText(s machine.Snapshot) string {
	var b strings.Builder
	t := s.Tutorial
	b.WriteString(i18n.T("start.title") + " · " + i18n.Tf("start.progress", t.Done(), len(t.Steps)) + "\n")
	if t.Done() == len(t.Steps) {
		b.WriteString(i18n.T("start.allDone") + "\n")
	}
	const indent = "     "
	for i, st := range t.Steps {
		mark := markTodo
		if st.Done {
			mark = markDone
		}
		key := "start." + st.ID
		fmt.Fprintf(&b, "\n%s %d. %s\n", mark, i+1, i18n.T(key+".title"))
		b.WriteString(indent + i18n.T(key+".why") + "\n")
		if st.ID == machine.StepSync {
			for _, k := range []string{"what", "notWhat", "password"} {
				b.WriteString(indent + i18n.T(key+"."+k) + "\n")
			}
		}
		b.WriteString(indent + i18n.T(key+".cmd") + "\n")
	}
	b.WriteString("\n" + i18n.T("start.footer"))
	return b.String()
}

// machineCardText — /machine: who, what is connected, what syncs, where
// the machine can be called from.
func machineCardText(s machine.Snapshot) string {
	st := s.Machine
	type row struct {
		label string
		lines []string
	}
	rows := []row{
		{i18n.T("machine.label.machine"), []string{i18n.Tf("machine.identity", st.Name, osName(st.OS, st.Arch), st.Version)}},
		{i18n.T("machine.label.account"), []string{accountLine(st)}},
		{i18n.T("machine.label.models"), sourceLines(st)},
		{i18n.T("machine.label.sync"), syncLines(st.Sync)},
		{i18n.T("machine.label.presence"), presenceLines(st.Presence, st.PlanPending)},
	}
	width := 0
	for _, r := range rows {
		if w := lipgloss.Width(r.label); w > width {
			width = w
		}
	}
	var b strings.Builder
	b.WriteString(i18n.T("machine.title") + "\n")
	for _, r := range rows {
		for i, line := range r.lines {
			label := ""
			if i == 0 {
				label = r.label
			}
			pad := strings.Repeat(" ", width-lipgloss.Width(label))
			b.WriteString("  " + label + pad + "  " + line + "\n")
		}
	}
	b.WriteString("\n" + i18n.T("machine.footer"))
	return b.String()
}

func accountLine(st machine.State) string {
	switch {
	case !st.LoggedIn:
		return i18n.T("machine.account.out")
	case st.PlanPending:
		return i18n.Tf("machine.account.planPending", st.EmailMasked)
	case st.Plan == "":
		return i18n.Tf("machine.account.noPlan", st.EmailMasked)
	}
	return i18n.Tf("machine.account.in", st.EmailMasked, st.PlanTitle)
}

func sourceLines(st machine.State) []string {
	var out []string
	for _, src := range st.Sources {
		mark := markDown
		var state string
		switch {
		case src.Alive:
			mark = markAlive
			state = i18n.Tf("machine.source.alive", stampTime(src.Checked))
		case src.ID == machine.ExecAIID && !st.LoggedIn:
			state = i18n.T("machine.source.needLogin")
		case src.ID == machine.ExecAIID && !src.Checked.IsZero():
			state = i18n.T("machine.source.noPlan")
		case !src.Checked.IsZero():
			state = i18n.Tf("machine.source.down", stampTime(src.Checked))
		default:
			state = i18n.T("machine.source.unchecked")
		}
		title := src.Title
		if src.Active {
			title += " (" + i18n.T("machine.source.active") + ")"
		}
		out = append(out, mark+" "+title+" — "+state)
	}
	return out
}

func syncLines(sy machine.Sync) []string {
	if !sy.Enabled {
		return []string{i18n.T("machine.sync.off")}
	}
	var head string
	switch {
	case sy.Paused != "":
		head = i18n.T("machine.sync.paused")
	case sy.LastPush.IsZero():
		head = i18n.T("machine.sync.never")
	default:
		head = i18n.Tf("machine.sync.on", stampTime(sy.LastPush))
	}
	var parts []string
	if sy.Config {
		parts = append(parts, i18n.T("machine.sync.config"))
	}
	if sy.PersonalMemoryFiles > 0 {
		parts = append(parts, i18n.Tf("machine.sync.memory", sy.PersonalMemoryFiles))
	}
	if sy.Projects > 0 {
		parts = append(parts, i18n.Tf("machine.sync.projects", sy.Projects))
	}
	inside := i18n.T("machine.sync.empty")
	if len(parts) > 0 {
		inside = strings.Join(parts, " · ")
	}
	return []string{head, i18n.Tf("machine.sync.inside", inside),
		i18n.Tf("machine.sync.mode", i18n.T("machine.mode."+sy.Mode))}
}

func presenceLines(p machine.Presence, planPending bool) []string {
	var out []string
	switch {
	case !p.ListenerRunning:
		out = append(out, i18n.T("machine.presence.noListener"))
	case p.ListenerKind == inbox.KindIDE:
		out = append(out, i18n.Tf("machine.presence.ide", p.ListenerPID))
	default:
		out = append(out, i18n.Tf("machine.presence.serve", p.ListenerPID))
	}
	if p.ReachableFromWeb {
		return append(out, i18n.T("machine.presence.reachable"))
	}
	reasons := make([]string, 0, len(p.Blockers))
	for _, bl := range p.Blockers {
		if bl == machine.BlockPlan && planPending {
			reasons = append(reasons, i18n.T("machine.block.planPending"))
			continue
		}
		reasons = append(reasons, i18n.T("machine.block."+bl))
	}
	return append(out, i18n.Tf("machine.presence.blocked", strings.Join(reasons, ", ")))
}

// phoneMsg — /phone: the QR code and the same link as text. The first time
// a link is shown, step 5 of the tutorial counts as done.
func phoneMsg(s machine.Snapshot) machineMsg {
	if s.PhoneURL == "" {
		return machineMsg{before: i18n.T("phone.title") + "\n" + i18n.T("phone.needLogin")}
	}
	out := machineMsg{before: i18n.T("phone.title") + "\n" + i18n.T("phone.scan")}
	lines, err := machine.QRLines(s.PhoneURL)
	if err == nil {
		out.qr = strings.Join(lines, "\n")
	}
	after := []string{s.PhoneURL, i18n.T("phone.what")}
	if err != nil {
		after = append(after, i18n.Tf("phone.qrFailed", err))
	}
	for _, bl := range s.Machine.Presence.Blockers {
		switch bl {
		case machine.BlockPlan:
			after = append(after, i18n.Tf("phone.plan", s.PayURL))
		case machine.BlockListener:
			after = append(after, i18n.T("phone.listener"))
		}
	}
	out.after = strings.Join(after, "\n")
	machine.MarkPhoneShown()
	return out
}

// renderQR paints the code; every line has the same width.
func renderQR(body string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = styleQR.Render(l)
	}
	return strings.Join(lines, "\n")
}

// osName — "Linux amd64", "macOS arm64", "Windows amd64".
func osName(goos, arch string) string {
	name := goos
	switch goos {
	case "linux":
		name = "Linux"
	case "darwin":
		name = "macOS"
	case "windows":
		name = "Windows"
	}
	if arch != "" {
		name += " " + arch
	}
	return name
}

func stampTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}
