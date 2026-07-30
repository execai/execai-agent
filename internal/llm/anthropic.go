// Client for the Anthropic Messages API (https://docs.anthropic.com/en/api/messages).
// Used for:
//   - Z.ai Coding Plan via the Anthropic-compatible endpoint
//     https://api.z.ai/api/anthropic/v1/messages
//   - In the future — the real Anthropic API for Claude Pro/Max OAuth.
//
// Messages API specifics (vs OpenAI chat/completions):
//   * system is a separate field (not a message)
//   * content in messages is an array of blocks [{type:text,text} | {type:image_url}
//     | {type:tool_use, name, input} | {type:tool_result, tool_use_id, content}]
//   * tools is an array of {name, description, input_schema}
//   * SSE events: message_start / content_block_start / content_block_delta
//     / content_block_stop / message_delta / message_stop / ping
//   * Tool args arrive as partial_json in delta.input_json_delta (accumulated)
//
// Mapping onto our StreamingLLM contract:
//   * tool_use → ToolCall{ID, Function:{Name, Arguments=accumulated json string}}
//   * content text deltas → cb.OnText / StreamResult.Content
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
	"strings"
	"time"
)

// AnthropicClient is a Messages API client.
type AnthropicClient struct {
	BaseURL        string // https://api.z.ai/api/anthropic  or  https://api.anthropic.com
	APIKey         string
	Model          string
	ThinkingBudget int    // 0 = thinking off; >0 → send {type:enabled, budget_tokens: N}
	SourceLabel    string // what to write into the requests.log source field (zai/anthropic/...)
	HTTP           *http.Client
}

// NewAnthropicClient takes the base URL without /v1/messages. thinkingBudget=0 → no thinking.
// sourceLabel is the label for requests.log (if empty, "anthropic" is used).
func NewAnthropicClient(baseURL, apiKey, model string, thinkingBudget int) *AnthropicClient {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	// Auto-detect the source label from the baseURL host.
	source := "anthropic"
	if strings.Contains(baseURL, "z.ai") || strings.Contains(baseURL, "bigmodel") {
		source = "zai-anthropic"
	}
	return &AnthropicClient{
		BaseURL:        baseURL,
		APIKey:         apiKey,
		Model:          model,
		ThinkingBudget: thinkingBudget,
		SourceLabel:    source,
		HTTP:           &http.Client{Timeout: 10 * time.Minute},
	}
}

var _ StreamingLLM = (*AnthropicClient)(nil)

// anthropicTool — {name, description, input_schema}.
type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

// anthropicMessage — role + content blocks.
type anthropicMessage struct {
	Role    string `json:"role"`    // user | assistant
	Content any    `json:"content"` // string | []map[string]any
}

// anthropicRequest — POST /v1/messages.
type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream"`
	Thinking  *anthropicThinking `json:"thinking,omitempty"`
}

// anthropicThinking — {"type":"enabled","budget_tokens":N}. Enables chain-of-thought
// for Claude 3.7+ / Opus 4+ and GLM-5.2 via the Z.ai Anthropic-compat endpoint.
type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

// Stream sends messages+tools to /v1/messages and assembles the result as an OpenAI-style
// StreamResult (for compatibility with the agent loop).
func (c *AnthropicClient) Stream(ctx context.Context, messages []AIMessage, tools []map[string]any, cb StreamCallbacks) (*StreamResult, error) {
	systemText, msgs := convertMessagesToAnthropic(messages)
	req0 := anthropicRequest{
		Model:     c.Model,
		Messages:  msgs,
		System:    systemText,
		Tools:     convertToolsToAnthropic(tools),
		MaxTokens: 8192,
		Stream:    true,
	}
	if c.ThinkingBudget > 0 {
		req0.Thinking = &anthropicThinking{Type: "enabled", BudgetTokens: c.ThinkingBudget}
		// Anthropic requires max_tokens > budget_tokens.
		if req0.MaxTokens <= c.ThinkingBudget {
			req0.MaxTokens = c.ThinkingBudget + 4096
		}
	}
	body, err := json.Marshal(req0)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	// Z.ai Anthropic-compat accepts Bearer; the real Anthropic wants x-api-key. Send both.
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("401 от Anthropic API — неверный ключ. Проверь /subscriptions")
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Anthropic API %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}

	res, modelReturned, usage, err := readAnthropicSSEFull(resp.Body, cb)
	// Log which model actually responded (server-side mapping in Z.ai).
	src := c.SourceLabel
	if src == "" {
		src = "anthropic"
	}
	logEntry := reqLogEntry{
		Source:            src,
		BaseURL:           c.BaseURL,
		ModelRequested:    c.Model,
		ModelReturned:     modelReturned,
		Status:            "ok",
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CacheReadTokens:   usage.CacheReadTokens,
		CacheCreateTokens: usage.CacheCreateTokens,
	}
	if err != nil {
		logEntry.Status = "error"
		logEntry.Err = err.Error()
	} else if res != nil {
		logEntry.ContentLen = len(res.Content)
		logEntry.ToolCalls = len(res.ToolCalls)
	}
	logRequest(logEntry)
	return res, err
}

// convertMessagesToAnthropic — our AIMessage → (system string, []anthropicMessage).
// system messages are concatenated into a single system field.
// tool-result messages become user-role with content=[{type:tool_result, ...}].
func convertMessagesToAnthropic(messages []AIMessage) (string, []anthropicMessage) {
	var systemSB strings.Builder
	var out []anthropicMessage

	for _, m := range messages {
		switch m.Role {
		case "system":
			if systemSB.Len() > 0 {
				systemSB.WriteString("\n\n")
			}
			systemSB.WriteString(ContentText(m.Content))

		case "user":
			// Content may be a string or []ContentBlock with image_url.
			// Mapping:
			//   string → as is (Anthropic accepts a string too)
			//   []ContentBlock → []map[string]any with text/image types
			content := convertContentToAnthropic(m.Content)
			out = append(out, anthropicMessage{Role: "user", Content: content})

		case "assistant":
			// If there are tool_calls, content must include tool_use blocks.
			blocks := []map[string]any{}
			if text := ContentText(m.Content); text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": text})
			}
			for _, tc := range m.ToolCalls {
				var input any
				if tc.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
				}
				if input == nil {
					input = map[string]any{}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": input,
				})
			}
			if len(blocks) == 0 {
				blocks = append(blocks, map[string]any{"type": "text", "text": ""})
			}
			out = append(out, anthropicMessage{Role: "assistant", Content: blocks})

		case "tool":
			// Anthropic treats tool_result as a user message with a tool_result block.
			out = append(out, anthropicMessage{
				Role: "user",
				Content: []map[string]any{{
					"type":         "tool_result",
					"tool_use_id":  m.ToolCallID,
					"content":      ContentText(m.Content),
				}},
			})
		}
	}
	return systemSB.String(), out
}

// convertContentToAnthropic handles user messages that may contain images.
func convertContentToAnthropic(c any) any {
	switch v := c.(type) {
	case nil:
		return ""
	case string:
		return v
	case []ContentBlock:
		blocks := make([]map[string]any, 0, len(v))
		for _, b := range v {
			if b.Type == "text" {
				blocks = append(blocks, map[string]any{"type": "text", "text": b.Text})
			} else if b.Type == "image_url" {
				// Anthropic expects {type:image, source:{type:base64, media_type, data}}
				// when the url is data:image/...;base64,...
				if strings.HasPrefix(b.ImageURL, "data:") {
					if idx := strings.Index(b.ImageURL, ";base64,"); idx > 5 {
						mediaType := strings.TrimPrefix(b.ImageURL[:idx], "data:")
						data := b.ImageURL[idx+len(";base64,"):]
						blocks = append(blocks, map[string]any{
							"type": "image",
							"source": map[string]any{
								"type":       "base64",
								"media_type": mediaType,
								"data":       data,
							},
						})
					}
				}
			}
		}
		return blocks
	default:
		return fmt.Sprintf("%v", c)
	}
}

// convertToolsToAnthropic — OpenAI tools schema → Anthropic tools schema.
// IN:  [{type:function, function:{name, description, parameters: JSONSchema}}]
// OUT: [{name, description, input_schema: JSONSchema}]
func convertToolsToAnthropic(tools []map[string]any) []anthropicTool {
	out := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		fn, _ := t["function"].(map[string]any)
		if fn == nil {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]any)
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, anthropicTool{
			Name:        name,
			Description: desc,
			InputSchema: params,
		})
	}
	return out
}

// readAnthropicSSE is the Messages API event-stream parser.
// Returns a StreamResult with the accumulated text + tool_calls.
// AnthropicStreamResult is an extended result carrying the server's actual model.
type AnthropicStreamResult struct {
	*StreamResult
	ModelReturned string
}

// anthropicUsage — usage from message_start.message.usage and message_delta.usage.
type anthropicUsage struct {
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
}

func readAnthropicSSE(r io.Reader, cb StreamCallbacks) (*StreamResult, error) {
	res, _, _, err := readAnthropicSSEFull(r, cb)
	return res, err
}

// readAnthropicSSEFull is the same as readAnthropicSSE but also returns model_returned + usage.
func readAnthropicSSEFull(r io.Reader, cb StreamCallbacks) (*StreamResult, string, anthropicUsage, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var content strings.Builder
	// Accumulate tool_use blocks by their index.
	type toolAcc struct {
		id   string
		name string
		args strings.Builder
	}
	tools := map[int]*toolAcc{}
	stopReason := ""
	modelReturned := ""
	var usage anthropicUsage

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var ev struct {
			Type    string `json:"type"`
			Index   int    `json:"index"`
			Message struct {
				ID    string `json:"id"`
				Model string `json:"model"` // the actual model the server served
				Usage struct {
					InputTokens              int `json:"input_tokens"`
					OutputTokens             int `json:"output_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				InputTokens              int `json:"input_tokens"`
				OutputTokens             int `json:"output_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			} `json:"usage"`
			ContentBlock struct {
				Type  string         `json:"type"`
				ID    string         `json:"id"`
				Name  string         `json:"name"`
				Input map[string]any `json:"input"`
				Text  string         `json:"text"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`         // text_delta | input_json_delta | thinking_delta
				Text        string `json:"text"`         // for text_delta
				PartialJSON string `json:"partial_json"` // for input_json_delta
				Thinking    string `json:"thinking"`     // for thinking_delta
				StopReason  string `json:"stop_reason"`  // in message_delta
			} `json:"delta"`
			Error any `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		if ev.Error != nil {
			return nil, modelReturned, usage, fmt.Errorf("Anthropic SSE error: %v", ev.Error)
		}

		switch ev.Type {
		case "message_start":
			if ev.Message.Model != "" {
				modelReturned = ev.Message.Model
			}
			if ev.Message.Usage.InputTokens > 0 {
				usage.InputTokens = ev.Message.Usage.InputTokens
				usage.CacheReadTokens = ev.Message.Usage.CacheReadInputTokens
				usage.CacheCreateTokens = ev.Message.Usage.CacheCreationInputTokens
			}
		case "content_block_start":
			if ev.ContentBlock.Type == "tool_use" {
				tools[ev.Index] = &toolAcc{
					id:   ev.ContentBlock.ID,
					name: ev.ContentBlock.Name,
				}
				if cb.OnToolCall != nil {
					cb.OnToolCall(ev.ContentBlock.Name)
				}
				// If there is already initial input, record it.
				if len(ev.ContentBlock.Input) > 0 {
					if raw, err := json.Marshal(ev.ContentBlock.Input); err == nil {
						tools[ev.Index].args.Write(raw)
					}
				}
			}

		case "content_block_delta":
			switch ev.Delta.Type {
			case "text_delta":
				if ev.Delta.Text != "" {
					content.WriteString(ev.Delta.Text)
					if cb.OnText != nil {
						cb.OnText(ev.Delta.Text)
					}
				}
			case "input_json_delta":
				if t, ok := tools[ev.Index]; ok {
					t.args.WriteString(ev.Delta.PartialJSON)
				}
			case "thinking_delta":
				if cb.OnReasoning != nil && ev.Delta.Thinking != "" {
					cb.OnReasoning(ev.Delta.Thinking)
				}
			}

		case "message_delta":
			if ev.Delta.StopReason != "" {
				stopReason = ev.Delta.StopReason
			}
			// usage in message_delta carries the final output_tokens.
			if ev.Usage.OutputTokens > 0 {
				usage.OutputTokens = ev.Usage.OutputTokens
			}

		case "message_stop":
			// Final event — exit the loop after processing what remains.
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, modelReturned, usage, err
	}

	out := &StreamResult{
		Content:      content.String(),
		FinishReason: stopReason,
	}
	for idx, t := range tools {
		args := t.args.String()
		if args == "" {
			args = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			Index: idx,
			ID:    t.id,
			Type:  "function",
			Function: ToolCallFunc{
				Name:      t.name,
				Arguments: args,
			},
		})
	}
	return out, modelReturned, usage, nil
}
