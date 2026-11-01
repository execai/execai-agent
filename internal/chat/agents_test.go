package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/fleet"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/sessions"
	"github.com/velesbsdllc/agent-vbai/internal/subagent"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// The block above the input shows working subagents with their step and
// last tool; the status line counts them; nothing shows when none work.
func TestAgentsBlockAndStatus(t *testing.T) {
	i18n.SetLocale("en")
	m := &tuiModel{width: 100, cfg: &config.Config{}, session: &sessions.Session{ID: "chat-1"}}
	if m.renderAgentsBlock() != "" || m.agentsStatus() != "" {
		t.Fatal("block shown without subagents")
	}
	now := time.Now()
	done := now.Add(-time.Second)
	m.fleetRecs = []fleet.Record{
		{ID: "sa-1", ParentChat: "chat-1", Title: "scan handlers", State: fleet.Running, Iter: 3, MaxIter: 15,
			LastTool: "Grep", StartedAt: now.Add(-42 * time.Second)},
		{ID: "sa-2", ParentChat: "chat-1", Title: "read docs", State: fleet.Done, StartedAt: now.Add(-time.Minute), FinishedAt: &done},
		{ID: "sa-3", ParentChat: "other", Title: "old chat", State: fleet.Failed},
	}
	block := m.renderAgentsBlock()
	for _, want := range []string{"Agents", "scan handlers", "3/15", "Grep", "read docs"} {
		if !strings.Contains(block, want) {
			t.Errorf("block lacks %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "old chat") {
		t.Error("a finished subagent of another chat is in the block")
	}
	if got := m.agentsStatus(); got != "1▶ 1✓" {
		t.Errorf("status %q", got)
	}
	// All finished: the block goes, the counter stays.
	m.fleetRecs[0].State = fleet.Done
	if m.renderAgentsBlock() != "" {
		t.Error("block stays after everything finished")
	}
	if got := m.agentsStatus(); got != "2✓" {
		t.Errorf("status %q", got)
	}
}

type tuiFakeLLM struct{}

func (tuiFakeLLM) Stream(context.Context, []llm.AIMessage, []map[string]any, llm.StreamCallbacks) (*llm.StreamResult, error) {
	return &llm.StreamResult{Content: "the handlers are in api/"}, nil
}

// /agents lists, opens the transcript, refuses to stop a finished one.
func TestAgentsCommand(t *testing.T) {
	i18n.SetLocale("en")
	m := &tuiModel{cfg: &config.Config{}, session: &sessions.Session{ID: "chat-1"}}
	m.fleet = fleet.New()
	m.agents = subagent.NewManager(m.fleet)
	if out := m.agentsCommand(""); !strings.Contains(out, "No subagents") {
		t.Errorf("empty list: %q", out)
	}
	r := m.agents.Runner(subagent.Deps{Client: tuiFakeLLM{}, Cwd: t.TempDir(), ParentChat: "chat-1", Model: "glm"})
	if _, err := r.Run(context.Background(), tools.SubagentRequest{Description: "find handlers", Prompt: "where?",
		Mode: "read", MaxIterations: 15}); err != nil {
		t.Fatal(err)
	}
	list := m.agentsCommand("")
	for _, want := range []string{"1.", "find handlers", "done", "glm", "the handlers are in api/"} {
		if !strings.Contains(list, want) {
			t.Errorf("list lacks %q:\n%s", want, list)
		}
	}
	open := m.agentsCommand("open 1")
	for _, want := range []string{"find handlers", "› where?", "● the handlers are in api/"} {
		if !strings.Contains(open, want) {
			t.Errorf("transcript lacks %q:\n%s", want, open)
		}
	}
	if out := m.agentsCommand("stop 1"); !strings.Contains(out, "already finished") {
		t.Errorf("stop finished: %q", out)
	}
	if out := m.agentsCommand("open 9"); !strings.Contains(out, "no agent 9") {
		t.Errorf("missing: %q", out)
	}
	if out := m.agentsCommand("frobnicate"); !strings.Contains(out, "usage") {
		t.Errorf("usage: %q", out)
	}
}

// The Task card shows the title, not raw JSON; its result line names it.
func TestTaskCard(t *testing.T) {
	i18n.SetLocale("en")
	args, _ := json.Marshal(map[string]any{"description": "review tests", "prompt": "long prompt", "mode": "write", "background": true})
	card := summaryForArgs("Task", args)
	if !strings.Contains(card, "review tests") || strings.Contains(card, "long prompt") || !strings.Contains(card, "may edit") {
		t.Errorf("card %q", card)
	}
	if got := taskCardResult("review tests", "", errors.New("boom")); !strings.Contains(got, "✗") || !strings.Contains(got, "boom") {
		t.Errorf("failed result %q", got)
	}
	if got := taskCardResult("review tests", "all good", nil); !strings.Contains(got, "✓") || !strings.Contains(got, "all good") {
		t.Errorf("result %q", got)
	}
}
