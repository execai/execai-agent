// HTTP-клиент к /aichat-vbai/stream. SSE-стрим в OpenAI-совместимом формате:
//   event: message
//   data: {"choices":[{"delta":{"content":"..."}}]}
//   ...
//   data: [DONE]
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var debug = os.Getenv("AGENT_VBAI_DEBUG") != ""

func debugln(prefix, line string) {
	if debug {
		fmt.Fprintln(os.Stderr, prefix, line)
	}
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

// Message — упрощённая форма DataCreate из aichat-vbai (только то, что нужно для альфа).
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// systemInfo соответствует SystemInfo в aichat-vbai/app/schemas/chat.py.
type systemInfo struct {
	Model    string `json:"model"`
	Provider string `json:"provider"`
}

// aiRequest — то, что принимает POST /aichat-vbai/stream.
type aiRequest struct {
	Messages       []Message    `json:"messages"`
	System         []systemInfo `json:"system"`
	Tools          []any        `json:"tools"`
	McpConnections []any        `json:"mcp_connections"`
	UserName       string       `json:"user_name"` // подменится сервером из JWT
}

type Client struct {
	APIBase  string
	Token    string
	Model    string
	Provider string
	HTTP     *http.Client
}

func New(apiBase, token, model, provider string) *Client {
	return &Client{
		APIBase:  apiBase,
		Token:    token,
		Model:    model,
		Provider: provider,
		HTTP:     &http.Client{Timeout: 5 * time.Minute},
	}
}

// Stream шлёт messages и в onDelta передаёт каждый прирост текста ответа модели.
// Возвращает полный накопленный текст ответа, либо ошибку.
func (c *Client) Stream(ctx context.Context, msgs []Message, onDelta func(string)) (string, error) {
	body, err := json.Marshal(aiRequest{
		Messages:       msgs,
		System:         []systemInfo{{Model: c.Model, Provider: c.Provider}},
		Tools:          []any{},
		McpConnections: []any{},
		UserName:       "",
	})
	if err != nil {
		return "", err
	}
	debugln("[req body]", string(body))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIBase+"/aichat-vbai/stream", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	debugln("[resp]", fmt.Sprintf("HTTP %d  Content-Type=%s", resp.StatusCode, resp.Header.Get("Content-Type")))

	if resp.StatusCode == http.StatusUnauthorized {
		return "", errors.New("401 от /aichat-vbai/stream — токен истёк, нужно agent-vbai login")
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ответ %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}

	return readSSE(resp.Body, onDelta)
}

// readSSE читает SSE-поток и собирает дельты OpenAI-стиля.
func readSSE(r io.Reader, onDelta func(string)) (string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var sb strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		debugln("[sse]", line)
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" || payload == "[AICORE_DONE]" {
			break
		}
		// Формат aichat-vbai: {"type":"text_delta","content":"..."} | session_init | message_id |
		//                    iteration_init | done | error.
		// Совместимость с OpenAI: {"choices":[{"delta":{"content":"..."}}]}.
		var chunk struct {
			Type    string `json:"type"`
			Content string `json:"content"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error any `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return sb.String(), fmt.Errorf("ошибка от сервера в SSE: %v", chunk.Error)
		}
		switch chunk.Type {
		case "text_delta":
			if chunk.Content != "" {
				sb.WriteString(chunk.Content)
				if onDelta != nil {
					onDelta(chunk.Content)
				}
			}
		case "done":
			return sb.String(), nil
		case "", "session_init", "message_id", "iteration_init":
			// служебные события — пропускаем; пустой Type = OpenAI-формат, обрабатываем ниже
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				sb.WriteString(ch.Delta.Content)
				if onDelta != nil {
					onDelta(ch.Delta.Content)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return sb.String(), err
	}
	return sb.String(), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
