// Direct client for the Z.ai GLM API (https://api.z.ai/api/paas/v4/chat/completions).
// OpenAI-compatible format (chat/completions), streaming via SSE.
// Used when the user has connected the Z.ai Coding Plan via `/connect zai`
// and switched with `/use zai`.
//
// Authentication: Bearer api_key
// Default base URL: https://api.z.ai/api/paas/v4 (international)
//                   https://open.bigmodel.cn/api/paas/v4 (CN)
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
)

const defaultZAIBaseURL = "https://api.z.ai/api/paas/v4"

// GLMClient is a generic OpenAI-compatible client (originally written for the
// Z.ai GLM API, but also used for Moonshot Platform / other providers).
// ProviderLabel appears in error messages so the user knows where the request went.
type GLMClient struct {
	BaseURL       string
	APIKey        string
	Model         string
	ProviderLabel string // "Z.ai" / "Moonshot" / etc — for the error prefix.
	HTTP          *http.Client
}

// NewGLMClient creates a client. Empty baseURL → default api.z.ai (intl).
// ProviderLabel is derived from BaseURL (moonshot → Moonshot, z.ai → Z.ai).
func NewGLMClient(baseURL, apiKey, model string) *GLMClient {
	if baseURL == "" {
		baseURL = defaultZAIBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	label := "Z.ai"
	switch {
	case strings.Contains(baseURL, "moonshot") || strings.Contains(baseURL, "kimi"):
		label = "Moonshot"
	case strings.Contains(baseURL, "openai.com"):
		label = "OpenAI"
	}
	return &GLMClient{
		BaseURL:       baseURL,
		APIKey:        apiKey,
		Model:         model,
		ProviderLabel: label,
		HTTP:          &http.Client{Timeout: 10 * time.Minute},
	}
}

// glmRequest is an OpenAI-compatible payload for chat/completions.
type glmRequest struct {
	Model    string             `json:"model"`
	Messages []AIMessage        `json:"messages"`
	Tools    []map[string]any   `json:"tools,omitempty"`
	Stream   bool               `json:"stream"`
}

// Stream is the StreamingLLM implementation for GLM. Streams the response like
// aicore so the agent loop stays provider-agnostic.
func (c *GLMClient) Stream(ctx context.Context, messages []AIMessage, tools []map[string]any, cb StreamCallbacks) (*StreamResult, error) {
	body, err := json.Marshal(glmRequest{
		Model:    c.Model,
		Messages: messages,
		Tools:    tools,
		Stream:   true,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	label := c.ProviderLabel
	if label == "" {
		label = "OpenAI-compat"
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("401 от %s — неверный API-ключ. Проверь /subscriptions и /connect", label)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s ответ %d: %s", label, resp.StatusCode, truncate(string(raw), 400))
	}

	return readGLMSSE(resp.Body, cb)
}

// readGLMSSE is an OpenAI-style SSE parser (same as aicore, for compatibility).
func readGLMSSE(r io.Reader, cb StreamCallbacks) (*StreamResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var content strings.Builder
	toolByIdx := map[int]*ToolCall{}
	finishReason := ""

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string     `json:"content"`
					Reasoning string     `json:"reasoning_content"`
					ToolCalls []ToolCall `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Error any `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return nil, fmt.Errorf("SSE error: %v", chunk.Error)
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				content.WriteString(ch.Delta.Content)
				if cb.OnText != nil {
					cb.OnText(ch.Delta.Content)
				}
			}
			if ch.Delta.Reasoning != "" && cb.OnReasoning != nil {
				cb.OnReasoning(ch.Delta.Reasoning)
			}
			for _, tc := range ch.Delta.ToolCalls {
				idx := tc.Index
				existing, ok := toolByIdx[idx]
				if !ok {
					existing = &ToolCall{Index: idx, ID: tc.ID, Type: tc.Type, Function: tc.Function}
					toolByIdx[idx] = existing
					if cb.OnToolCall != nil && tc.Function.Name != "" {
						cb.OnToolCall(tc.Function.Name)
					}
				} else {
					// Accumulate the arguments deltas.
					if tc.ID != "" {
						existing.ID = tc.ID
					}
					if tc.Function.Name != "" {
						existing.Function.Name = tc.Function.Name
						if cb.OnToolCall != nil {
							cb.OnToolCall(tc.Function.Name)
						}
					}
					existing.Function.Arguments += tc.Function.Arguments
				}
			}
			if ch.FinishReason != "" {
				finishReason = ch.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	out := &StreamResult{
		Content:      content.String(),
		FinishReason: finishReason,
	}
	for _, tc := range toolByIdx {
		out.ToolCalls = append(out.ToolCalls, *tc)
	}
	return out, nil
}

// Ensure GLMClient implements StreamingLLM.
var _ StreamingLLM = (*GLMClient)(nil)

// GLMModels — models available via the Z.ai Coding Plan (Anthropic-compatible
// endpoint https://api.z.ai/api/anthropic). Per the Z.ai docs, this endpoint's
// server-side mapping goes to:
//   - glm-4.7 (default)
//   - glm-5.2 (latest, optionally with the [1m] suffix for a 1M context)
//
// Billed via the subscription (Coding Plan key), not pay-per-token.
func GLMModels() []Model {
	return []Model{
		{ID: "glm-5.2", Provider: "zai", Name: "GLM-5.2", Description: "Flagship coding (MoE 753B/40B, dual thinking)", Tier: "flagship", IsPrimary: true, HasTools: true},
		{ID: "glm-5.2[1m]", Provider: "zai", Name: "GLM-5.2 [1M ctx]", Description: "GLM-5.2 c 1M контекстом (если нужен очень большой ввод)", Tier: "flagship", HasTools: true},
		{ID: "glm-4.7", Provider: "zai", Name: "GLM-4.7", Description: "Дефолтная мапа Coding Plan; быстрее и дешевле 5.2", Tier: "standard", HasTools: true},
	}
}
