// Client for direct access to /aicore-vbai/agent-stream — an OpenAI-compatible
// SSE stream with tool_calls. Used by the agent loop (internal/agent/loop.go) for
// formal tool-use, unlike the old client.go which talks to
// /aichat-vbai/stream and parses the aichat format (text_delta only).
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/velesbsdllc/agent-vbai/internal/version"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AIMessage is the message format expected by aicore-vbai. Compatible with
// internal/tools (for tool_calls/tool_call_id) and with the OpenAI Chat API.
// AIMessage.Content is either a string (plain text) or []ContentBlock
// (multimodal: text + image_url). The aicore/anthropic/openai/zai/moonshot
// providers understand the list [{type:text,text:""}, {type:image_url,image_url:"data:..."}].
type AIMessage struct {
	Role       string      `json:"role"` // user | assistant | system | tool
	Content    interface{} `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Name       string      `json:"name,omitempty"`
}

// ContentBlock is a single block of a multimodal message. Compatible with OpenAI
// vision and our aicore (anthropic transforms image_url→base64 source automatically).
type ContentBlock struct {
	Type     string `json:"type"`                // "text" | "image_url"
	Text     string `json:"text,omitempty"`      // for type=text
	ImageURL string `json:"image_url,omitempty"` // for type=image_url — data:image/...;base64,...
}

// ContentText renders Content as flat text-only. string → as is, []ContentBlock
// → concatenation of text blocks + an [image] placeholder for each image.
// Used in the UI, sessions.Title, logs.
func ContentText(c interface{}) string {
	switch v := c.(type) {
	case nil:
		return ""
	case string:
		return v
	case []ContentBlock:
		var sb strings.Builder
		for _, b := range v {
			if b.Type == "text" {
				sb.WriteString(b.Text)
			} else if b.Type == "image_url" {
				sb.WriteString(" [image] ")
			}
		}
		return sb.String()
	case []interface{}:
		// When decoded from JSON into interface{}, each element is a map[string]any.
		var sb strings.Builder
		for _, item := range v {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			t, _ := m["type"].(string)
			if t == "text" {
				if s, ok := m["text"].(string); ok {
					sb.WriteString(s)
				}
			} else if t == "image_url" {
				sb.WriteString(" [image] ")
			}
		}
		return sb.String()
	}
	return fmt.Sprintf("%v", c)
}

type ToolCall struct {
	Index    int          `json:"index"` // present in delta chunks for all OpenAI-style providers
	ID       string       `json:"id"`
	Type     string       `json:"type"` // usually "function"
	Function ToolCallFunc `json:"function"`
}

type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string (accumulated across chunks)
}

// AICoreClient is an HTTP+SSE client for /aicore-vbai/agent-stream.
type AICoreClient struct {
	APIBase  string
	Token    string
	Model    string
	Provider string
	HTTP     *http.Client
}

func NewAICoreClient(apiBase, token, model, provider string) *AICoreClient {
	return &AICoreClient{
		APIBase:  apiBase,
		Token:    token,
		Model:    model,
		Provider: provider,
		HTTP:     &http.Client{Timeout: 10 * time.Minute},
	}
}

// StreamResult is the result of one loop iteration. If ToolCalls is non-empty,
// the agent executes them and continues the dialogue; if empty, this is the final answer.
type StreamResult struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}

// StreamCallbacks — UI callbacks (showing text deltas and tool_call starts).
type StreamCallbacks struct {
	OnText      func(string)      // increment of visible text
	OnToolCall  func(name string) // first time we see the name of a tool being called — UI may show "▶ Bash..."
	OnReasoning func(string)      // chain-of-thought (DeepSeek/o1) — may be hidden or dimmed
}

type streamChunk struct {
	// Standard OpenAI-style chunk fields
	Choices []struct {
		Delta struct {
			Content      string     `json:"content"`
			Reasoning    string     `json:"reasoning"`         // DeepSeek in our aicore
			ReasoningAlt string     `json:"reasoning_content"` // OpenAI o1 style
			ToolCalls    []ToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error any `json:"error"`

	// Typed events from aicore — e.g. billing_status (insufficient_funds /
	// rate_limit_exceeded / subscription_expired).
	Type            string `json:"type"`
	Status          string `json:"status"`
	Message         string `json:"message"`
	Window          string `json:"window"`
	WindowLabel     string `json:"window_label"`
	RetryAfterSec   int    `json:"retry_after_sec"`
	RetryAfterHuman string `json:"retry_after_human"`
}

// aicoreRequest mirrors the aicore-vbai Pydantic StreamRequest.
type aicoreRequest struct {
	Messages []AIMessage      `json:"messages"`
	Tools    []map[string]any `json:"tools"`
	System   []sysConfig      `json:"system"`
}

type sysConfig struct {
	Model         string `json:"model"`
	Provider      string `json:"provider"`
	InteractionID string `json:"interaction_id"`
}

// postStream POSTs the body with our headers to APIBase+path and returns the response.
// Reading the body is the caller's responsibility (defer resp.Body.Close).
func (c *AICoreClient) postStream(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIBase+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Accept", "text/event-stream")
	return c.HTTP.Do(req)
}

// Stream sends messages+tools to /aicore-vbai/agent-stream and accumulates the result
// (text + tool_calls). Returns a StreamResult.
func (c *AICoreClient) Stream(ctx context.Context, messages []AIMessage, tools []map[string]any, cb StreamCallbacks) (*StreamResult, error) {
	body, err := json.Marshal(aicoreRequest{
		Messages: messages,
		Tools:    tools,
		System: []sysConfig{{
			Model:         c.Model,
			Provider:      c.Provider,
			InteractionID: uuid.NewString(),
		}},
	})
	if err != nil {
		return nil, err
	}

	// Try /agent-stream (an alias for the CLI — provides separate tracing).
	// If the alias is not registered in Redis on this env (api-vbai returns
	// "Invalid Authorization"), fall back to /stream. Functionally
	// identical; only the separate tracing is lost.
	resp, err := c.postStream(ctx, "/aicore-vbai/agent-stream", body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(raw), "Invalid Authorization") {
			// alias not registered — fall back to /stream
			resp, err = c.postStream(ctx, "/aicore-vbai/stream", body)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("401 от /aicore-vbai/agent-stream — %s", truncateStr(string(raw), 300))
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("401 от /aicore-vbai — %s", truncateStr(string(raw), 300))
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ответ %d: %s", resp.StatusCode, truncateStr(string(raw), 600))
	}

	res, err := readAICoreSSE(resp.Body, cb)
	entry := reqLogEntry{
		Source:         "execai",
		BaseURL:        c.APIBase,
		ModelRequested: c.Model,
		ModelReturned:  c.Model, // aicore does not return model in SSE; assume ours
		Status:         "ok",
	}
	if err != nil {
		entry.Status = "error"
		entry.Err = err.Error()
	} else if res != nil {
		entry.ContentLen = len(res.Content)
		entry.ToolCalls = len(res.ToolCalls)
	}
	logRequest(entry)
	return res, err
}

func readAICoreSSE(r io.Reader, cb StreamCallbacks) (*StreamResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	res := &StreamResult{}
	// tool_calls arrive incrementally: index → accumulation.
	tcAcc := map[int]*ToolCall{}
	tcSeenNames := map[int]bool{}

	for scanner.Scan() {
		line := scanner.Text()
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
		var ch streamChunk
		if err := json.Unmarshal([]byte(payload), &ch); err != nil {
			continue
		}
		if ch.Error != nil {
			return res, fmt.Errorf("сервер вернул ошибку: %v", ch.Error)
		}

		// Typed events from aicore (billing_status etc.) — print in the TUI and end the stream.
		if ch.Type == "billing_status" {
			var label string
			switch ch.Status {
			case "rate_limit_exceeded":
				label = "⏱  ЛИМИТ ИСЧЕРПАН"
			case "insufficient_funds":
				label = "💰 НЕДОСТАТОЧНО СРЕДСТВ"
			default:
				label = "⚠  ТАРИФ НЕАКТИВЕН"
			}
			msg := strings.TrimSpace(ch.Message)
			if msg == "" {
				msg = "Сервис временно недоступен по биллингу."
			}
			// Extra hint about upgrading the plan. Без зашитого хоста: бинарь
			// один на все контуры, а dev-домен в подсказке у прод-юзера — и
			// утечка внутренней инфраструктуры, и просто враньё.
			hint := "\nСменить тариф: раздел «Тариф» в веб-кабинете ExecAI (execai.ru)"
			fullText := fmt.Sprintf("\n\n%s\n%s%s\n", label, msg, hint)
			if cb.OnText != nil {
				cb.OnText(fullText)
			}
			// Return as a "normal" completion so the TUI does not crash.
			return res, nil
		}
		for _, c := range ch.Choices {
			if c.Delta.Content != "" {
				res.Content += c.Delta.Content
				if cb.OnText != nil {
					cb.OnText(c.Delta.Content)
				}
			}
			rc := c.Delta.Reasoning
			if rc == "" {
				rc = c.Delta.ReasoningAlt
			}
			if rc != "" && cb.OnReasoning != nil {
				cb.OnReasoning(rc)
			}
			for _, tc := range c.Delta.ToolCalls {
				// The key is Index from the chunk itself: OpenAI always sends it,
				// and it is what determines which tool_call the chunked
				// arguments accumulation belongs to. If the array contains two
				// tool_calls with index=0 and index=1, these are TWO distinct calls, not one.
				key := tc.Index
				cur, ok := tcAcc[key]
				if !ok {
					cur = &ToolCall{Type: "function", Index: tc.Index}
					tcAcc[key] = cur
				}
				if tc.ID != "" {
					cur.ID = tc.ID
				}
				if tc.Type != "" {
					cur.Type = tc.Type
				}
				if tc.Function.Name != "" {
					// OpenAI/aicore: name arrives in the FIRST tool_call chunk and
					// is empty in subsequent ones. Some providers (DeepSeek via
					// aicore) may send the name again — in that case do not
					// concatenate (`Bash`+`Bash`=`BashBash`); ignore the repeat.
					if cur.Function.Name == "" {
						cur.Function.Name = tc.Function.Name
					}
					if !tcSeenNames[key] && cb.OnToolCall != nil {
						cb.OnToolCall(cur.Function.Name)
						tcSeenNames[key] = true
					}
				}
				if tc.Function.Arguments != "" {
					cur.Function.Arguments += tc.Function.Arguments
				}
			}
			if c.FinishReason != "" {
				res.FinishReason = c.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return res, err
	}

	// Serialize tool_calls by index.
	if len(tcAcc) > 0 {
		maxIdx := -1
		for i := range tcAcc {
			if i > maxIdx {
				maxIdx = i
			}
		}
		for i := 0; i <= maxIdx; i++ {
			if tc, ok := tcAcc[i]; ok && tc.Function.Name != "" {
				res.ToolCalls = append(res.ToolCalls, *tc)
			}
		}
	}
	return res, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
