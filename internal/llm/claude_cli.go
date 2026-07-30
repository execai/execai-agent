// ClaudeCLIClient delegates requests to a locally installed Claude Code
// (`claude` in PATH), using the user's OAuth session. Billing is the Pro/Max
// subscription quota or Claude Code Plan; our agent is not involved.
//
// Architecture:
//   1. Serialize the history into one text prompt (roles via tags)
//   2. Pipe it into `claude -p --output-format stream-json --include-partial-messages`
//   3. Parse the JSONL line by line. It contains stream_event with an Anthropic-style
//      event (message_start, content_block_delta, message_delta, message_stop)
//   4. Return a StreamResult compatible with the agent loop
//
// Limitations:
//   * execai tools (Bash/Read/Write) do not work — the claude CLI runs its OWN
//     tools with its own permissions (its own ToolPermissions). If the user needs
//     execai tools, use ExecAI or another source.
//   * The dialogue history is passed in full as a single prompt (no session-id) —
//     each request is self-contained. Claude sees the context from the prompt.
//   * To switch models — the --model parameter (we don't; we rely on claude config).
package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ClaudeCLIClient is a wrapper around the local `claude` CLI.
type ClaudeCLIClient struct {
	Path  string // path to the claude binary (resolved from PATH in New)
	Model string // optional --model
}

// NewClaudeCLIClient resolves `claude` in PATH. If absent, returns nil + error.
func NewClaudeCLIClient(model string) (*ClaudeCLIClient, error) {
	path, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude CLI не найден в PATH. Установка: https://docs.claude.com/en/docs/claude-code/quickstart")
	}
	return &ClaudeCLIClient{Path: path, Model: model}, nil
}

var _ StreamingLLM = (*ClaudeCLIClient)(nil)

// Stream delegates to the claude CLI and parses stream-json.
func (c *ClaudeCLIClient) Stream(ctx context.Context, messages []AIMessage, tools []map[string]any, cb StreamCallbacks) (*StreamResult, error) {
	// Build a single prompt from the history.
	prompt := flattenMessagesToPrompt(messages)
	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
	}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	cmd := exec.CommandContext(ctx, c.Path, args...)
	cmd.Stdin = strings.NewReader(prompt)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil // claude CLI writes to stderr — but it is noisy
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	res, modelReturned, usage := parseClaudeStreamJSON(stdout, cb)

	waitErr := cmd.Wait()
	if waitErr != nil {
		if res == nil || res.Content == "" {
			return res, fmt.Errorf("claude CLI: %w", waitErr)
		}
		// if we managed to get a response, do not return an error
	}
	if res == nil {
		return nil, errors.New("claude CLI: пустой результат")
	}

	logRequest(reqLogEntry{
		Source:            "claude-cli",
		BaseURL:           c.Path,
		ModelRequested:    c.Model,
		ModelReturned:     modelReturned,
		Status:            "ok",
		ContentLen:        len(res.Content),
		ToolCalls:         len(res.ToolCalls),
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CacheReadTokens:   usage.CacheReadTokens,
		CacheCreateTokens: usage.CacheCreateTokens,
	})
	return res, nil
}

// flattenMessagesToPrompt serializes the history into a single prompt string for claude -p.
// A simple format with role tags, since the claude CLI does not accept structured messages
// via --input-format text.
func flattenMessagesToPrompt(messages []AIMessage) string {
	var b strings.Builder
	for _, m := range messages {
		switch m.Role {
		case "system":
			fmt.Fprintf(&b, "<system>\n%s\n</system>\n\n", ContentText(m.Content))
		case "user":
			fmt.Fprintf(&b, "<user>\n%s\n</user>\n\n", ContentText(m.Content))
		case "assistant":
			text := ContentText(m.Content)
			if text != "" {
				fmt.Fprintf(&b, "<assistant>\n%s\n</assistant>\n\n", text)
			}
		case "tool":
			fmt.Fprintf(&b, "<tool_result name=\"%s\">\n%s\n</tool_result>\n\n", m.Name, ContentText(m.Content))
		}
	}
	return b.String()
}

// parseClaudeStreamJSON reads the JSONL stream from the claude CLI and builds a StreamResult.
// Each line is a JSON object with a "type" field. We care about:
//   * type=stream_event, event.type=message_start → remember model + usage
//   * type=stream_event, event.type=content_block_delta, delta.type=text_delta → cb.OnText
//   * type=stream_event, event.type=content_block_delta, delta.type=thinking_delta → cb.OnReasoning
//   * type=stream_event, event.type=message_delta, usage.output_tokens
//   * type=result → final usage + result text
func parseClaudeStreamJSON(r interface{ Read([]byte) (int, error) }, cb StreamCallbacks) (*StreamResult, string, anthropicUsage) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // claude may send very long lines
	var content strings.Builder
	modelReturned := ""
	var usage anthropicUsage
	stopReason := ""

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg struct {
			Type    string          `json:"type"`
			Subtype string          `json:"subtype"`
			Event   json.RawMessage `json:"event"`
			Result  string          `json:"result"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "stream_event":
			// Inner event in the Anthropic format (message_start, content_block_delta, ...).
			parseClaudeInnerEvent(msg.Event, cb, &content, &modelReturned, &usage, &stopReason)
		case "result":
			// If for some reason no content was accumulated from deltas, take the final result.
			if content.Len() == 0 && msg.Result != "" {
				content.WriteString(msg.Result)
				if cb.OnText != nil {
					cb.OnText(msg.Result)
				}
			}
		}
	}

	out := &StreamResult{
		Content:      content.String(),
		FinishReason: stopReason,
	}
	return out, modelReturned, usage
}

func parseClaudeInnerEvent(raw json.RawMessage, cb StreamCallbacks, content *strings.Builder, modelReturned *string, usage *anthropicUsage, stopReason *string) {
	var ev struct {
		Type    string `json:"type"`
		Message struct {
			Model string `json:"model"`
			Usage struct {
				InputTokens              int `json:"input_tokens"`
				OutputTokens             int `json:"output_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			Thinking    string `json:"thinking"`
			StopReason  string `json:"stop_reason"`
		} `json:"delta"`
		Usage struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return
	}
	switch ev.Type {
	case "message_start":
		if ev.Message.Model != "" {
			*modelReturned = ev.Message.Model
		}
		usage.InputTokens = ev.Message.Usage.InputTokens
		usage.CacheReadTokens = ev.Message.Usage.CacheReadInputTokens
		usage.CacheCreateTokens = ev.Message.Usage.CacheCreationInputTokens
	case "content_block_delta":
		switch ev.Delta.Type {
		case "text_delta":
			if ev.Delta.Text != "" {
				content.WriteString(ev.Delta.Text)
				if cb.OnText != nil {
					cb.OnText(ev.Delta.Text)
				}
			}
		case "thinking_delta":
			if cb.OnReasoning != nil && ev.Delta.Thinking != "" {
				cb.OnReasoning(ev.Delta.Thinking)
			}
		}
	case "message_delta":
		if ev.Delta.StopReason != "" {
			*stopReason = ev.Delta.StopReason
		}
		if ev.Usage.OutputTokens > 0 {
			usage.OutputTokens = ev.Usage.OutputTokens
		}
	}
}
