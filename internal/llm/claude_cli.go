// ClaudeCLIClient — делегирует запросы в локально установленный Claude Code
// (`claude` в PATH), используя OAuth-сессию юзера. Билинг — квота Pro/Max-
// подписки или Claude Code Plan, наш агент в этом не участвует.
//
// Архитектура:
//   1. Сериализуем history в один текстовый промт (роли через теги)
//   2. Пайпим в `claude -p --output-format stream-json --include-partial-messages`
//   3. Парсим JSONL построчно. Внутри есть stream_event с Anthropic-style
//      event (message_start, content_block_delta, message_delta, message_stop)
//   4. Возвращаем StreamResult совместимый с agent loop
//
// Ограничения:
//   * Tools execai (Bash/Read/Write) не работают — claude CLI запускает СВОИ
//     tools и сам с разрешениями (своя ToolPermissions). Если user'у нужен
//     execai tools — используй ExecAI или другой source.
//   * История диалога передаётся целиком как один промт (без session-id) —
//     каждый запрос self-contained. Claude видит контекст из промта.
//   * Для смены модели — параметр --model (мы не делаем, пользуем claude config).
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

// ClaudeCLIClient — обёртка над локальным `claude` CLI.
type ClaudeCLIClient struct {
	Path  string // путь к claude binary (резолвится из PATH в New)
	Model string // optional --model
}

// NewClaudeCLIClient — резолвит `claude` в PATH. Если нет — возвращает nil + error.
func NewClaudeCLIClient(model string) (*ClaudeCLIClient, error) {
	path, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude CLI не найден в PATH. Установка: https://docs.claude.com/en/docs/claude-code/quickstart")
	}
	return &ClaudeCLIClient{Path: path, Model: model}, nil
}

var _ StreamingLLM = (*ClaudeCLIClient)(nil)

// Stream — делегирует в claude CLI, парсит stream-json.
func (c *ClaudeCLIClient) Stream(ctx context.Context, messages []AIMessage, tools []map[string]any, cb StreamCallbacks) (*StreamResult, error) {
	// Собираем единый промт из истории.
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
	cmd.Stderr = nil // claude CLI кидает в stderr — но это шумно
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	res, modelReturned, usage := parseClaudeStreamJSON(stdout, cb)

	waitErr := cmd.Wait()
	if waitErr != nil {
		if res == nil || res.Content == "" {
			return res, fmt.Errorf("claude CLI: %w", waitErr)
		}
		// если успели получить ответ — не возвращаем ошибку
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

// flattenMessagesToPrompt — сериализует history в один промт-строку для claude -p.
// Простой формат с тегами ролей, т.к. claude CLI не принимает structured messages
// через --input-format text.
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

// parseClaudeStreamJSON — читает JSONL поток от claude CLI и собирает StreamResult.
// Каждая строка — JSON-объект с полем "type". Нам интересны:
//   * type=stream_event, event.type=message_start → запоминаем модель + usage
//   * type=stream_event, event.type=content_block_delta, delta.type=text_delta → cb.OnText
//   * type=stream_event, event.type=content_block_delta, delta.type=thinking_delta → cb.OnReasoning
//   * type=stream_event, event.type=message_delta, usage.output_tokens
//   * type=result → финальный usage + result text
func parseClaudeStreamJSON(r interface{ Read([]byte) (int, error) }, cb StreamCallbacks) (*StreamResult, string, anthropicUsage) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // claude может слать длинные строки
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
			// Внутреннее событие в Anthropic-формате (message_start, content_block_delta, ...).
			parseClaudeInnerEvent(msg.Event, cb, &content, &modelReturned, &usage, &stopReason)
		case "result":
			// Если по каким-то причинам не накопили content из delta'ов — берём финальный result.
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
