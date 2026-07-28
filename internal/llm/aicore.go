// Клиент для прямого доступа в /aicore-vbai/agent-stream — это OpenAI-совместимый
// SSE-стрим с tool_calls. Используется агент-циклом (internal/agent/loop.go) для
// formal tool-use, в отличие от старого client.go который ходит в
// /aichat-vbai/stream и парсит aichat-формат (только text_delta).
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AIMessage — формат сообщения, который ждёт aicore-vbai. Совместим с
// internal/tools (для tool_calls/tool_call_id) и с OpenAI Chat API.
// AIMessage.Content — либо строка (обычный текст), либо []ContentBlock
// (multimodal: text + image_url). aicore/anthropic/openai/zai/moonshot
// провайдеры понимают список [{type:text,text:""}, {type:image_url,image_url:"data:..."}].
type AIMessage struct {
	Role       string      `json:"role"` // user | assistant | system | tool
	Content    interface{} `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Name       string      `json:"name,omitempty"`
}

// ContentBlock — один блок multimodal-сообщения. Совместим с OpenAI vision и
// нашим aicore (anthropic трансформирует image_url→base64 source автоматически).
type ContentBlock struct {
	Type     string `json:"type"`                // "text" | "image_url"
	Text     string `json:"text,omitempty"`      // для type=text
	ImageURL string `json:"image_url,omitempty"` // для type=image_url — data:image/...;base64,...
}

// ContentText — плоский text-only вид Content. string → как есть, []ContentBlock
// → конкатенация text-блоков + плейсхолдер [image] на каждую картинку.
// Используется в UI, sessions.Title, логах.
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
		// Когда раскодировано из JSON в interface{} — каждый элемент map[string]any.
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
	Index    int          `json:"index"` // в delta-чанках присутствует у всех провайдеров OpenAI-стиля
	ID       string       `json:"id"`
	Type     string       `json:"type"` // обычно "function"
	Function ToolCallFunc `json:"function"`
}

type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-строка (накапливается по чанкам)
}

// AICoreClient — HTTP+SSE клиент для /aicore-vbai/agent-stream.
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

// StreamResult — результат одной итерации цикла. Если ToolCalls не пуст,
// агент выполняет их и продолжает диалог; если пуст — это финальный ответ.
type StreamResult struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}

// StreamCallbacks — колбэки для UI (показ дельт текста и старта tool_call'ов).
type StreamCallbacks struct {
	OnText      func(string)        // приращение видимого текста
	OnToolCall  func(name string)   // первый раз увидели имя вызываемого tool — UI может показать "▶ Bash..."
	OnReasoning func(string)        // chain-of-thought (DeepSeek/o1) — можно не показывать или dim
}

type streamChunk struct {
	// Стандартные поля OpenAI-style chunk
	Choices []struct {
		Delta struct {
			Content      string     `json:"content"`
			Reasoning    string     `json:"reasoning"`         // DeepSeek в нашем aicore
			ReasoningAlt string     `json:"reasoning_content"` // OpenAI o1-стиль
			ToolCalls    []ToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error any `json:"error"`

	// Typed events от aicore — например billing_status (insufficient_funds /
	// rate_limit_exceeded / subscription_expired).
	Type             string `json:"type"`
	Status           string `json:"status"`
	Message          string `json:"message"`
	Window           string `json:"window"`
	WindowLabel      string `json:"window_label"`
	RetryAfterSec    int    `json:"retry_after_sec"`
	RetryAfterHuman  string `json:"retry_after_human"`
}

// aicoreRequest — Pydantic StreamRequest aicore-vbai.
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

// postStream — POST body с нашими headers на APIBase+path, возвращает response.
// Читатель body — на вызывающей стороне (defer resp.Body.Close).
func (c *AICoreClient) postStream(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIBase+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	return c.HTTP.Do(req)
}

// Stream шлёт messages+tools в /aicore-vbai/agent-stream и накапливает результат
// (текст + tool_calls). Возвращает StreamResult.
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

	// Пробуем /agent-stream (alias для CLI — даёт отдельную трассировку).
	// Если на env alias не зарегистрирован в Redis (api-vbai возвращает
	// "Invalid Authorization") — фолбэкаемся на /stream. Функционально
	// идентично, теряется только раздельная трассировка.
	resp, err := c.postStream(ctx, "/aicore-vbai/agent-stream", body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(raw), "Invalid Authorization") {
			// alias не зарегистрирован — фолбэк на /stream
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
		ModelReturned:  c.Model, // aicore не возвращает model в SSE, считаем что наша
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
	// tool_calls приходят инкрементально: index → накопление.
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

		// Typed events от aicore (billing_status и пр.) — выводим в TUI и завершаем стрим.
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
			// Дополнительная подсказка про апгрейд тарифа.
			hint := "\nПодробнее: execai-dev.velesbsd.com/plans (Сменить тариф)"
			fullText := fmt.Sprintf("\n\n%s\n%s%s\n", label, msg, hint)
			if cb.OnText != nil {
				cb.OnText(fullText)
			}
			// Возвращаем как «нормальное» завершение, чтобы TUI не падал.
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
				// Ключ — Index из самого chunk'а: OpenAI всегда передаёт его,
				// и именно он определяет к какому tool_call относится накопление
				// arguments по чанкам. Если в массиве пришло два tool_call с
				// index=0 и index=1 — это ДВА разных вызова, не один.
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
					// OpenAI/aicore: name приходит в ПЕРВОМ tool_call chunk и
					// пустой в последующих. Некоторые провайдеры (DeepSeek через
					// aicore) могут прислать имя повторно — в этом случае не
					// склеиваем (`Bash`+`Bash`=`BashBash`), а игнорируем повтор.
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

	// Сериализуем tool_calls по индексам.
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
