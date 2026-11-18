// /sync in the chat — memory and settings the same on all of the person's
// computers (internal/sync). The short conversations of the command (the
// setup, a password change, the forget word, a key import) take the chat
// input; every step that goes to the network or hashes a password runs off
// the UI loop, and a password is neither shown while typed, nor echoed, nor
// kept in the input history.
package chat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

// syncStepMsg — a step of a /sync conversation, or the answer of a one-shot
// /sync command (flow == nil), is ready.
type syncStepMsg struct {
	flow agentsync.Flow
	step agentsync.Step
}

// syncNoticeMsg — the background sync has something to tell.
type syncNoticeMsg struct{ text string }

// syncPulledMsg — files here were changed by another computer.
type syncPulledMsg struct{ kinds []string }

// syncExchangedMsg — a full background exchange went through.
type syncExchangedMsg struct{}

// ensureMemoryIndexAtBoot writes the memory index template at the start of
// the chat — unless sync is on here: then a template written before the
// first exchange would stand for the index in a folder whose index was
// deleted (or not taken from the server yet), and an index without entries
// looks like every entry removed. The template is written after the first
// exchange instead (memoryIndexAfterSync).
func ensureMemoryIndexAtBoot() {
	if agentsync.EnabledHere() {
		return
	}
	_, _ = agent.EnsureUserMemoryIndex()
}

// memoryIndexAfterSync — the first full exchange of this chat went
// through: the memory is what the server has; a missing index gets the
// template now and the next turn sees the memory as it came.
func (m *tuiModel) memoryIndexAfterSync() {
	if m.memoryIndexChecked {
		return
	}
	m.memoryIndexChecked = true
	_, _ = agent.EnsureUserMemoryIndex()
	m.reloadMemory()
}

// syncService — the sync of this process, made on first use. It shares the
// chat's plan checker, so the chat and sync never ask billing twice.
func (m *tuiModel) syncService() *agentsync.Service {
	if m.syncSvc != nil {
		return m.syncSvc
	}
	base := config.Defaults().APIBase
	if m.cfg != nil && m.cfg.APIBase != "" {
		base = m.cfg.APIBase
	}
	svc, err := agentsync.New(base)
	if err != nil {
		return nil
	}
	if m.plan != nil {
		svc.Gate.Checker = m.plan
	}
	m.syncSvc = svc
	return svc
}

// startBackgroundSync runs the background exchange for the life of the
// chat; the returned function sends what is left on exit. Nothing happens
// while sync is off here.
func (m *tuiModel) startBackgroundSync() func() {
	svc := m.syncService()
	if svc == nil {
		return func() {}
	}
	send := func(msg tea.Msg) {
		if p := teaProgramRef; p != nil {
			p.Send(msg)
		}
	}
	return svc.Start(context.Background(), agentsync.Hooks{
		Notify:    func(text string) { send(syncNoticeMsg{text: text}) },
		Pulled:    func(kinds []string) { send(syncPulledMsg{kinds: kinds}) },
		Exchanged: func() { send(syncExchangedMsg{}) },
	})
}

// handleSyncCommand — /sync [on|off|now|password|forget|keys export|keys import [code]].
func (m *tuiModel) handleSyncCommand(cmd string) tea.Cmd {
	svc := m.syncService()
	if svc == nil {
		m.syncHint(i18n.T("sync.usage"))
		return nil
	}
	args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(cmd, "/sync")))
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch {
	case sub == "" || sub == "status":
		m.syncHint(svc.Status())
		return nil
	case sub == "on":
		return m.startSyncFlow(svc.OnFlow())
	case sub == "password":
		return m.startSyncFlow(svc.PasswordFlow())
	case sub == "forget":
		return m.startSyncFlow(svc.ForgetFlow())
	case sub == "off":
		return m.syncOnce(svc.Off)
	case sub == "now":
		return m.syncOnce(svc.SyncNow)
	case sub == "keys" && len(args) > 1 && args[1] == "export":
		// Hashing the phrase takes a moment: off the UI loop too.
		return m.syncOnce(func(context.Context) string { return svc.KeysExport() })
	case sub == "keys" && len(args) > 1 && args[1] == "import":
		cmd := m.startSyncFlow(svc.KeysImportFlow(strings.Join(args[2:], "")))
		m.syncReloadSubs = true
		return cmd
	}
	m.syncHint(i18n.T("sync.usage"))
	return nil
}

func (m *tuiModel) syncHint(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: text})
	m.refreshViewport()
}

// syncOnce runs a one-shot command off the UI loop.
func (m *tuiModel) syncOnce(run func(context.Context) string) tea.Cmd {
	m.statusMessage = i18n.T("sync.working")
	return func() tea.Msg {
		return syncStepMsg{step: agentsync.Step{Text: run(context.Background())}}
	}
}

// startSyncFlow opens a conversation; its first step runs off the UI loop.
func (m *tuiModel) startSyncFlow(f agentsync.Flow) tea.Cmd {
	m.syncFlow = f
	m.syncStep = agentsync.Step{}
	m.syncBusy = true
	m.syncReloadSubs = false
	m.statusMessage = i18n.T("sync.working")
	return func() tea.Msg { return syncStepMsg{flow: f, step: f.Start(context.Background())} }
}

// syncAnswer takes the line typed for the current question.
func (m *tuiModel) syncAnswer(raw string) (tea.Model, tea.Cmd) {
	f, step := m.syncFlow, m.syncStep
	line := strings.TrimSpace(m.expandPasteMarkers(raw))
	echo := line
	if step.Secret || step.Quiet {
		// The answer itself never reaches the transcript, and a pasted
		// secret does not stay behind in the paste store either.
		m.dropPastes(raw)
		echo = "••••••••"
	}
	m.uiSegments = append(m.uiSegments, segment{kind: "user", body: echo})
	m.syncBusy = true
	m.statusMessage = i18n.T("sync.working")
	m.refreshViewport()
	return m, func() tea.Msg { return syncStepMsg{flow: f, step: f.Answer(context.Background(), line)} }
}

// dropPastes forgets the pastes whose markers are in raw.
func (m *tuiModel) dropPastes(raw string) {
	for id, p := range m.pasteStore {
		if strings.Contains(raw, p.marker) {
			delete(m.pasteStore, id)
		}
	}
}

// cancelSyncFlow — Esc during a /sync conversation.
func (m *tuiModel) cancelSyncFlow() {
	m.endSyncFlow()
	m.syncHint(i18n.T("sync.cancelled"))
}

func (m *tuiModel) endSyncFlow() {
	m.syncFlow = nil
	m.syncStep = agentsync.Step{}
	m.syncBusy = false
	m.syncReloadSubs = false
	m.textarea.Placeholder = i18n.T("placeholder.chat")
}

// updateSync handles the sync messages; ok=false for any other message.
func (m *tuiModel) updateSync(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case syncStepMsg:
		m.applySyncStep(msg)
		return nil, true
	case syncNoticeMsg:
		m.syncHint(msg.text)
		return nil, true
	case syncPulledMsg:
		m.applyPulled(msg.kinds)
		return nil, true
	case syncExchangedMsg:
		m.memoryIndexAfterSync()
		return nil, true
	}
	return nil, false
}

func (m *tuiModel) applySyncStep(msg syncStepMsg) {
	m.statusMessage = ""
	if msg.flow == nil { // a one-shot command
		m.syncHint(msg.step.Text)
		return
	}
	if msg.flow != m.syncFlow {
		// Cancelled while the step ran: say what it did, ask nothing.
		m.syncHint(msg.step.Text)
		return
	}
	m.syncBusy = false
	body := strings.TrimSpace(msg.step.Text)
	if !msg.step.Done() {
		if body != "" {
			body += "\n\n"
		}
		body += msg.step.Prompt
	}
	m.syncHint(body)
	if msg.step.Done() {
		reload := m.syncReloadSubs
		m.endSyncFlow()
		// A key import may have added sources: the chat must not save its
		// stale copy of subscriptions.json over them.
		if reload {
			if fresh, err := subscriptions.Load(); err == nil && fresh != nil {
				m.subs = fresh
			}
		}
		return
	}
	m.syncStep = msg.step
	m.textarea.Placeholder = i18n.T("sync.placeholder")
}

// applyPulled — another computer changed files here. Memory is seen from
// the next turn. The config is global (settings, sources with their keys,
// permissions): the chat takes it into its own copies, so a later save here
// does not put the old values back, and follows a change of the active
// source.
func (m *tuiModel) applyPulled(kinds []string) {
	for _, k := range kinds {
		switch k {
		case agentsync.KindMemory, agentsync.KindProjectMemory:
			m.reloadMemory()
		case agentsync.KindConfig:
			m.reloadSyncedConfig()
			m.reloadSyncedSources()
		}
	}
}

// reloadSyncedConfig re-reads config.json into m.cfg; this machine's
// api_base stays. Settings apply from their next use; nothing is switched
// mid-turn.
func (m *tuiModel) reloadSyncedConfig() {
	if m.cfg == nil {
		return
	}
	dir, err := config.Dir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return
	}
	var c config.Config
	if json.Unmarshal(data, &c) != nil {
		return
	}
	c.APIBase = m.cfg.APIBase
	*m.cfg = c
}

// reloadSyncedSources re-reads subscriptions.json; when the active source
// changed on the other computer, the chat switches to it between turns and
// says so.
func (m *tuiModel) reloadSyncedSources() {
	fresh, err := subscriptions.Load()
	if err != nil || fresh == nil {
		return
	}
	norm := func(s string) string {
		if s == "" {
			return subscriptions.SourceExecAI
		}
		return s
	}
	before := ""
	if m.subs != nil {
		before = m.subs.Active
	}
	m.subs = fresh
	if norm(fresh.Active) == norm(before) || m.streaming {
		return
	}
	m.applySubscriptionSource()
	m.syncHint(i18n.Tf("sync.pulledSource", fresh.SourceLabel()))
}

// inputView — the input line; a password is shown as dots.
func (m *tuiModel) inputView() string {
	if m.syncFlow != nil && m.syncStep.Secret {
		return m.textarea.Prompt + strings.Repeat("•", utf8.RuneCountInString(m.textarea.Value()))
	}
	return m.textarea.View()
}
