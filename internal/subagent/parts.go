package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/fleet"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/sessions"
)

// readSystem is the system prompt of a read-only subagent. Short on purpose:
// it has no history, no memory of the conversation and no way to ask
// anything — its whole job is to answer the prompt it was given.
const readSystem = `You are a subagent of the execai coding agent, running a single self-contained
investigation task. Working directory: %s

Rules:
* You have READ-ONLY tools (Read, Grep, Glob, LS, Tree, WebFetch, WebSearch).
  You cannot write files or run shell commands — do not promise to.
* You do not see the user's conversation. Everything you need is in the task.
* You cannot ask questions. If something is ambiguous, investigate the most
  likely reading and say in your answer which one you assumed.
* Some reads and all network calls need the user's permission, which you cannot
  request. If a tool is refused, do not retry and do not work around it: state
  plainly in your result what you could not access and why, so the main agent
  can ask the user.
* Your final message is the ENTIRE result — it is handed to the main agent as
  one block. Be specific: file paths with line numbers, exact names, concrete
  findings. No preamble, no "I will now look at", no restating the task.
* Report what you actually found. "Not found" is a valid, useful answer;
  guessing is not.`

// writeSystem is the system prompt of a subagent that may act.
const writeSystem = `You are a subagent of the execai coding agent, doing one self-contained task
on the user's machine. Working directory: %s

Rules:
* You have the full toolset: read, search, Write, Edit, Bash. Stay inside the
  working directory unless the task says otherwise.
* Actions that need consent are asked from the user in the main agent's
  window, marked with your name. A refusal is final: do not retry it and do
  not work around it — say in your result what was refused.
* Other subagents may be working in the same project at the same time. Touch
  only the files your task is about.
* You do not see the user's conversation and cannot ask questions. If
  something is ambiguous, take the most likely reading and say which.
* Your final message is the ENTIRE result handed to the main agent: what you
  changed (files, commands and their outcome), what is left, what failed.
  Be specific and brief.`

func systemPrompt(write bool, cwd string) string {
	if write {
		return fmt.Sprintf(writeSystem, cwd)
	}
	return fmt.Sprintf(readSystem, cwd)
}

// streamer feeds a subagent's progress into its fleet record. Nothing goes
// to the parent's feed: the person follows subagents in the agents list.
type streamer struct {
	fleet *fleet.Fleet
	id    string

	mu  sync.Mutex
	cur strings.Builder // text of the current iteration — the preview
	all strings.Builder // all text — the answer when the last message has none
}

func (s *streamer) OnText(delta string) {
	s.mu.Lock()
	s.cur.WriteString(delta)
	s.all.WriteString(delta)
	preview := s.cur.String()
	s.mu.Unlock()
	s.fleet.Update(s.id, func(r *fleet.Record) { r.Preview = fleet.Tail(preview, fleet.PreviewLimit) })
}

func (s *streamer) OnReasoning(string)         {}
func (s *streamer) OnToolChunk(string, string) {}

func (s *streamer) OnToolCall(name string, _ json.RawMessage) {
	s.fleet.Update(s.id, func(r *fleet.Record) { r.LastTool = name })
}

func (s *streamer) OnToolResult(string, string, error) {}

func (s *streamer) OnIterationStart(n int) {
	s.mu.Lock()
	s.cur.Reset()
	s.mu.Unlock()
	s.fleet.Update(s.id, func(r *fleet.Record) { r.Iter = n })
}

func (s *streamer) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.all.String()
}

// denyAll refuses whatever it is asked: a read-only subagent has nobody to
// ask. Grants the person gave before (permissions.json) are checked before
// the approver, so an allowed directory or domain stays reachable; anything
// else is refused instead of being waved through — delegation must not be a
// way around the perimeter.
type denyAll struct{}

func (denyAll) AskApprove(string, json.RawMessage, string) agent.ApproveDecision {
	return agent.ApproveDeny
}

// attributed asks the parent's approver on behalf of a write subagent and
// shows "waiting for the person" in the subagent's record meanwhile.
type attributed struct {
	ctx    context.Context
	parent agent.Approver
	who    agent.Asker
	fleet  *fleet.Fleet
}

func (a *attributed) AskApprove(toolName string, args json.RawMessage, summary string) agent.ApproveDecision {
	if a.parent == nil {
		return agent.ApproveDeny
	}
	a.fleet.Update(a.who.ID, func(r *fleet.Record) { r.State = fleet.Waiting })
	defer a.fleet.Update(a.who.ID, func(r *fleet.Record) {
		if r.State == fleet.Waiting {
			r.State = fleet.Running
		}
	})
	return agent.AskAs(a.ctx, a.parent, a.who, toolName, args, summary)
}

// TranscriptMsg is one line of a subagent's transcript.
type TranscriptMsg struct {
	Role string // user | assistant | tool | tool_result
	Tool string
	Text string
}

// TranscriptOf turns a saved subagent session into lines for a viewer: the
// task, the subagent's text, its tool calls (as their short summary) and
// the tool results (cut to limit runes; 0 — whole).
func TranscriptOf(s *sessions.Session, limit int) []TranscriptMsg {
	var out []TranscriptMsg
	cut := func(t string) string {
		r := []rune(strings.TrimSpace(t))
		if limit > 0 && len(r) > limit {
			return string(r[:limit]) + "…"
		}
		return string(r)
	}
	for _, msg := range s.Messages {
		switch msg.Role {
		case "user":
			out = append(out, TranscriptMsg{Role: "user", Text: llm.ContentText(msg.Content)})
		case "assistant":
			if body := strings.TrimSpace(llm.ContentText(msg.Content)); body != "" {
				out = append(out, TranscriptMsg{Role: "assistant", Text: body})
			}
			for _, tc := range msg.ToolCalls {
				out = append(out, TranscriptMsg{Role: "tool", Tool: tc.Function.Name,
					Text: agent.SummaryFor(tc.Function.Name, json.RawMessage(tc.Function.Arguments))})
			}
		case "tool":
			out = append(out, TranscriptMsg{Role: "tool_result", Tool: msg.Name, Text: cut(llm.ContentText(msg.Content))})
		}
	}
	return out
}
