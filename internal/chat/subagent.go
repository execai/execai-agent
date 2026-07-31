// Subagent runner: a nested agent loop with an isolated context and a
// read-only toolset. Wired into the Task tool at startup (see initModel).
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// Subagents get a much tighter iteration budget than the main agent (40).
// A subagent that needs more than this has been given a task too big to
// delegate — and every iteration is billed to the user's provider quota.
const subagentMaxIterations = 15

// subagentTimeout is a backstop against a subagent that keeps finding one more
// file to read. The user is waiting on this call.
const subagentTimeout = 5 * time.Minute

// subagentSystem is the subagent's system prompt. It is deliberately short:
// the subagent has no history, no memory of the conversation and no way to ask
// anything — its whole job is to answer the prompt it was given.
const subagentSystem = `You are a subagent of the execai coding agent, running a single self-contained
investigation task.

Rules:
* You have READ-ONLY tools (Read, Grep, Glob, LS, Tree, WebFetch, WebSearch).
  You cannot write files or run shell commands — do not promise to.
* You do not see the user's conversation. Everything you need is in the task.
* You cannot ask questions. If something is ambiguous, investigate the most
  likely reading and say in your answer which one you assumed.
* Your final message is the ENTIRE result — it is handed to the main agent as
  one block. Be specific: file paths with line numbers, exact names, concrete
  findings. No preamble, no "I will now look at", no restating the task.
* Report what you actually found. "Not found" is a valid, useful answer;
  guessing is not.`

// subagentStreamer swallows the subagent's stream: the user should see the task
// and its result, not a second conversation interleaved with the main one.
// Text is kept only so the final answer can be recovered if the model ends
// without a proper last message.
type subagentStreamer struct {
	lastText strings.Builder
	tools    []string
}

func (s *subagentStreamer) OnText(delta string)        { s.lastText.WriteString(delta) }
func (s *subagentStreamer) OnReasoning(string)         {}
func (s *subagentStreamer) OnToolChunk(string, string) {}
func (s *subagentStreamer) OnIterationStart(int)       {}
func (s *subagentStreamer) OnToolCall(name string, _ json.RawMessage) {
	s.tools = append(s.tools, name)
}
func (s *subagentStreamer) OnToolResult(string, string, error) {}

// subagentApprover denies everything. It should never be reached — the
// read-only registry has no tool that asks — so reaching it means a tool was
// added to ReadOnly() without thinking, and denying is the safe answer.
type subagentApprover struct{}

func (subagentApprover) AskApprove(string, json.RawMessage, string) agent.ApproveDecision {
	return agent.ApproveDeny
}

// runSubagent executes one delegated task and returns its final text.
func (m *tuiModel) runSubagent(ctx context.Context, description, prompt string) (string, error) {
	if m.cli == nil {
		return "", tools.ErrSubagentUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, subagentTimeout)
	defer cancel()

	st := &subagentStreamer{}
	sub := agent.New(m.cli, tools.ReadOnly(getCWDForBoot()), subagentSystem, subagentApprover{}, st)
	sub.MaxIterations = subagentMaxIterations

	history, err := sub.Run(ctx, nil, prompt)
	if err != nil {
		return "", fmt.Errorf("%s: %w", description, err)
	}

	answer := lastAssistantText(history)
	if answer == "" {
		answer = strings.TrimSpace(st.lastText.String())
	}
	if answer == "" {
		return "", fmt.Errorf("%s: %s", description, i18n.T("subagent.emptyResult"))
	}
	return answer, nil
}

// lastAssistantText returns the text of the last assistant message that carried
// any — the subagent's conclusion. Trailing messages with only tool_calls are
// skipped.
func lastAssistantText(history []llm.AIMessage) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role != "assistant" {
			continue
		}
		if body := strings.TrimSpace(llm.ContentText(history[i].Content)); body != "" {
			return body
		}
	}
	return ""
}
