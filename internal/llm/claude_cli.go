// ClaudeCLIClient delegates requests to a locally installed Claude Code,
// using the user's OAuth session. Billing is the Pro/Max subscription quota
// or Claude Code Plan; our agent is not involved.
//
// Architecture:
//  1. Serialize the history into one text prompt (roles via tags)
//  2. Send it as the single user message of
//     `claude -p --input-format stream-json --output-format stream-json
//     --permission-prompt-tool stdio`
//  3. Parse the JSONL line by line: stream_event carries Anthropic-style
//     deltas (text, thinking, usage); assistant/user messages carry the
//     tool_use and tool_result blocks of claude's OWN tools; control_request
//     can_use_tool is claude asking permission for an edit or a command
//  4. Answer permission requests on stdin through the agent's gate, close
//     stdin after the final `result`, return a StreamResult
//
// Why the permission channel. Without it `claude -p` has nobody to ask and
// silently denies every edit and command that its settings do not pre-allow:
// the agent becomes a read-only chat on any machine where ~/.claude is not
// configured to allow everything. With it, those requests go through the same
// approvals and security levels as execai's own tools.
//
// Limitations:
//   - claude runs ITS tools (Read/Edit/Bash of Claude Code), not execai's —
//     the agent only shows and gates them.
//   - The dialogue history is passed in full as a single prompt (no session-id) —
//     each request is self-contained. Claude sees the context from the prompt.
package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// ClaudeCLIClient is a wrapper around the local `claude` CLI.
type ClaudeCLIClient struct {
	Path    string // the claude binary (the newest install found, see findClaude)
	Version string // its version, "" if unknown
	Model   string // optional --model
}

// NewClaudeCLIClient picks the newest Claude Code on this machine. If there is
// none, returns nil + error.
func NewClaudeCLIClient(model string) (*ClaudeCLIClient, error) {
	bin, ok := findClaude()
	if !ok {
		return nil, fmt.Errorf("claude CLI не найден (PATH, ~/.local/bin, расширения VS Code/Cursor). Установка: https://docs.claude.com/en/docs/claude-code/quickstart")
	}
	return &ClaudeCLIClient{Path: bin.Path, Version: bin.Version, Model: model}, nil
}

// Describe names the binary in messages: "claude 2.1.292 (/path/to/claude)".
// An error like "version 2.1.280 or newer is required" is useless without
// knowing WHICH claude answered it.
func (c *ClaudeCLIClient) Describe() string {
	v := c.Version
	if v == "" {
		v = "?"
	}
	return fmt.Sprintf("claude %s (%s)", v, c.Path)
}

var _ StreamingLLM = (*ClaudeCLIClient)(nil)

// Stream delegates to the claude CLI and parses stream-json.
func (c *ClaudeCLIClient) Stream(ctx context.Context, messages []AIMessage, tools []map[string]any, cb StreamCallbacks) (*StreamResult, error) {
	prompt := flattenMessagesToPrompt(messages)
	args := []string{
		"--print",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--permission-prompt-tool", "stdio",
	}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	cmd := exec.CommandContext(ctx, c.Path, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &tailBuffer{max: 4096}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	w := &lineWriter{w: stdin}
	// A long history may not fit into the pipe buffer: writing it from this
	// goroutine before reading stdout could block both processes.
	go w.send(map[string]any{
		"type":               "user",
		"message":            map[string]any{"role": "user", "content": prompt},
		"parent_tool_use_id": nil,
		"session_id":         "",
	})

	run := parseClaudeStreamJSON(stdout, cb, w)
	w.close()
	waitErr := cmd.Wait()

	entry := reqLogEntry{
		Source:            "claude-cli",
		BaseURL:           c.Path,
		ModelRequested:    c.Model,
		ModelReturned:     run.model,
		Status:            "ok",
		ContentLen:        len(run.res.Content),
		InputTokens:       run.usage.InputTokens,
		OutputTokens:      run.usage.OutputTokens,
		CacheReadTokens:   run.usage.CacheReadTokens,
		CacheCreateTokens: run.usage.CacheCreateTokens,
	}
	var runErr error
	switch {
	case run.isError:
		// Errors arrive as a normal `result` with is_error: before, they were
		// printed as if they were the model's answer.
		msg := strings.TrimSpace(run.resultText)
		if msg == "" {
			msg = run.subtype
		}
		runErr = fmt.Errorf("%s: %s", c.Describe(), msg)
	case !run.gotResult && waitErr != nil && run.res.Content == "":
		runErr = fmt.Errorf("%s: %w%s", c.Describe(), waitErr, stderr.suffix())
	case !run.gotResult && run.res.Content == "":
		runErr = errors.New(c.Describe() + ": пустой результат" + stderr.suffix())
	}
	if runErr != nil {
		entry.Status = "error"
		entry.Err = runErr.Error()
		logRequest(entry)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, runErr
	}
	logRequest(entry)
	run.res.PromptTokens = run.usage.InputTokens + run.usage.CacheReadTokens + run.usage.CacheCreateTokens
	return run.res, nil
}

// flattenMessagesToPrompt serializes the history into a single prompt string for claude -p.
// A simple format with role tags: claude -p takes one user message, not a
// structured history.
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

// claudeRun is everything one claude -p invocation produced.
type claudeRun struct {
	res        *StreamResult
	model      string
	usage      anthropicUsage
	gotResult  bool
	isError    bool
	subtype    string
	resultText string
}

// parseClaudeStreamJSON reads the JSONL stream from the claude CLI. w answers
// control requests and is closed once the final `result` arrives — in
// stream-json input mode claude keeps waiting for more input until stdin
// closes.
func parseClaudeStreamJSON(r io.Reader, cb StreamCallbacks, w *lineWriter) claudeRun {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // claude may send very long lines
	var content strings.Builder
	run := claudeRun{}
	stopReason := ""
	newMessage := false
	tools := &toolSequencer{cb: cb}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg struct {
			Type      string          `json:"type"`
			Subtype   string          `json:"subtype"`
			Event     json.RawMessage `json:"event"`
			Message   json.RawMessage `json:"message"`
			Parent    *string         `json:"parent_tool_use_id"`
			Result    string          `json:"result"`
			IsError   bool            `json:"is_error"`
			RequestID string          `json:"request_id"`
			Request   json.RawMessage `json:"request"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		// Messages of a Task subagent: its text is not our answer, and its
		// tools are covered by the Task card. Its permission requests still
		// come through control_request below.
		sub := msg.Parent != nil && *msg.Parent != ""
		switch msg.Type {
		case "stream_event":
			if sub {
				continue
			}
			parseClaudeInnerEvent(msg.Event, cb, &content, &run.model, &run.usage, &stopReason, &newMessage)
		case "assistant":
			if !sub {
				tools.fromAssistant(msg.Message)
			}
		case "user":
			if !sub {
				tools.fromUser(msg.Message)
			}
		case "control_request":
			w.send(answerControlRequest(msg.RequestID, msg.Request, cb))
		case "result":
			run.gotResult = true
			run.isError = msg.IsError || (msg.Subtype != "" && msg.Subtype != "success")
			run.subtype = msg.Subtype
			run.resultText = msg.Result
			// If for some reason no content was accumulated from deltas, take the final result.
			if !run.isError && content.Len() == 0 && msg.Result != "" {
				content.WriteString(msg.Result)
				if cb.OnText != nil {
					cb.OnText(msg.Result)
				}
			}
			w.close()
		}
	}
	tools.finish()

	run.res = &StreamResult{
		Content:      content.String(),
		FinishReason: stopReason,
	}
	return run
}

func parseClaudeInnerEvent(raw json.RawMessage, cb StreamCallbacks, content *strings.Builder, modelReturned *string, usage *anthropicUsage, stopReason *string, newMessage *bool) {
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
			Type       string `json:"type"`
			Text       string `json:"text"`
			Thinking   string `json:"thinking"`
			StopReason string `json:"stop_reason"`
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
		*newMessage = true
	case "content_block_delta":
		switch ev.Delta.Type {
		case "text_delta":
			if ev.Delta.Text == "" {
				return
			}
			// One turn of claude is several API messages (text → tool → text).
			// Without a break between them the answer read "I'll read the
			// file first.Done — ...".
			text := ev.Delta.Text
			if *newMessage && content.Len() > 0 && !strings.HasSuffix(content.String(), "\n") {
				text = "\n\n" + text
			}
			*newMessage = false
			content.WriteString(text)
			if cb.OnText != nil {
				cb.OnText(text)
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

// answerControlRequest builds the reply to a control_request from claude.
// Only can_use_tool is understood; anything else gets an error reply, never
// silence — claude would wait for an answer forever.
func answerControlRequest(id string, raw json.RawMessage, cb StreamCallbacks) map[string]any {
	var req struct {
		Subtype  string          `json:"subtype"`
		ToolName string          `json:"tool_name"`
		Input    json.RawMessage `json:"input"`
	}
	_ = json.Unmarshal(raw, &req)
	if req.Subtype != "can_use_tool" {
		return map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "error", "request_id": id, "error": "unsupported control request: " + req.Subtype,
		}}
	}
	input := req.Input
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	allow, reason := false, "в execai нет канала подтверждения для этого вызова"
	if cb.ApproveDelegatedTool != nil {
		allow, reason = cb.ApproveDelegatedTool(req.ToolName, input)
	}
	decision := map[string]any{"behavior": "allow", "updatedInput": input}
	if !allow {
		if reason == "" {
			reason = "Пользователь отклонил выполнение этого инструмента."
		}
		decision = map[string]any{"behavior": "deny", "message": reason}
	}
	return map[string]any{"type": "control_response", "response": map[string]any{
		"subtype": "success", "request_id": id, "response": decision,
	}}
}

// toolSequencer turns claude's tool_use/tool_result blocks into strict
// call → result pairs. Interfaces track one "current" tool card: with claude's
// parallel calls (several tool_use, then several results) a naive relay would
// attach every result to the last card. So a call is announced only after the
// previous one got its result.
type toolSequencer struct {
	cb    StreamCallbacks
	queue []*delegatedCall
	seen  map[string]bool
}

type delegatedCall struct {
	id, name  string
	input     json.RawMessage
	announced bool
	done      bool
	result    string
	isError   bool
}

func (s *toolSequencer) fromAssistant(raw json.RawMessage) {
	var m struct {
		Content []struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	for _, b := range m.Content {
		if b.Type != "tool_use" || b.ID == "" {
			continue
		}
		if s.seen == nil {
			s.seen = map[string]bool{}
		}
		if s.seen[b.ID] {
			continue
		}
		s.seen[b.ID] = true
		s.queue = append(s.queue, &delegatedCall{id: b.ID, name: b.Name, input: b.Input})
	}
	s.flush()
}

func (s *toolSequencer) fromUser(raw json.RawMessage) {
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	var blocks []struct {
		Type      string          `json:"type"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
		IsError   bool            `json:"is_error"`
	}
	if json.Unmarshal(m.Content, &blocks) != nil {
		return // plain-string content: a user turn, not tool results
	}
	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
		}
		for _, c := range s.queue {
			if c.id == b.ToolUseID {
				c.done, c.result, c.isError = true, toolResultText(b.Content), b.IsError
			}
		}
	}
	s.flush()
}

func (s *toolSequencer) flush() {
	for len(s.queue) > 0 {
		head := s.queue[0]
		if !head.announced {
			head.announced = true
			if s.cb.OnDelegatedTool != nil {
				s.cb.OnDelegatedTool(head.name, head.input)
			}
		}
		if !head.done {
			return
		}
		if s.cb.OnDelegatedToolResult != nil {
			s.cb.OnDelegatedToolResult(head.name, head.result, head.isError)
		}
		s.queue = s.queue[1:]
	}
}

// finish closes cards whose result never came (claude killed, turn cancelled):
// an announced call without a result would spin in the interface forever.
func (s *toolSequencer) finish() {
	for _, c := range s.queue {
		if !c.done {
			c.done, c.result, c.isError = true, "нет результата: claude завершился раньше", true
		}
	}
	s.flush()
}

// toolResultText flattens tool_result content: a string or a list of blocks.
func toolResultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// lineWriter writes JSON lines to claude's stdin from two places (the initial
// message and control replies), so writes are serialized and a write after
// close is a no-op rather than a panic or an EPIPE in the log.
type lineWriter struct {
	mu     sync.Mutex
	w      io.WriteCloser
	closed bool
}

func (l *lineWriter) send(v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	_, _ = l.w.Write(append(raw, '\n'))
}

func (l *lineWriter) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closed {
		l.closed = true
		_ = l.w.Close()
	}
}

// tailBuffer keeps the last max bytes of stderr: enough for the reason of a
// crash, without letting a chatty process grow memory.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) suffix() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := strings.TrimSpace(string(t.buf))
	if s == "" {
		return ""
	}
	return " — " + s
}
