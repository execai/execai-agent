// OllamaClient — прямой клиент к локальному Ollama runner (ollama.com).
// Ollama слушает http://localhost:11434 и умеет OpenAI-совместимый endpoint
// /v1/chat/completions — используем его же SSE-парсер что и для Z.ai/aicore.
//
// Особенности:
//   * API-key не нужен (по умолчанию доступ без auth к localhost).
//   * Каталог моделей динамический — берём через GET /api/tags что сейчас
//     установлено локально (`ollama pull`). Никакого хардкода.
//   * Tools поддерживаются моделями которые их реально умеют
//     (llama3.1+, qwen2.5+, mistral-nemo, ...). Проставляем HasTools=true
//     по умолчанию — если модель не умеет, Ollama просто игнорирует.
//
// Билинг: 0 ₽ (локально). Ограничение — только скорость железа.
package llm

import (
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

const defaultOllamaBaseURL = "http://localhost:11434"

// OllamaCloudModels — известные модели на ollama.com (турbo/cloud tier).
// Если Ollama Cloud добавит новую — сюда добавляем руками. Каталог
// небольшой и меняется редко.
func OllamaCloudModels() []Model {
	return []Model{
		{ID: "glm-5.2", Provider: "ollama", Name: "GLM-5.2", Description: "Z.ai flagship coding (MoE 753B/40B)", Tier: "flagship", IsPrimary: true, HasTools: true},
		{ID: "qwen3-coder:480b-cloud", Provider: "ollama", Name: "Qwen3 Coder 480B", Description: "Alibaba coder MoE", Tier: "flagship", HasTools: true},
		{ID: "kimi-k2:1t-cloud", Provider: "ollama", Name: "Kimi K2 1T", Description: "Moonshot 1T-параметров", Tier: "flagship", HasTools: true},
		{ID: "deepseek-v3.1:671b-cloud", Provider: "ollama", Name: "DeepSeek V3.1", Description: "DeepSeek flagship", Tier: "flagship", HasTools: true},
		{ID: "gpt-oss:120b-cloud", Provider: "ollama", Name: "GPT-OSS 120B", Description: "OpenAI open-weights", Tier: "standard", HasTools: true},
	}
}

// OllamaClient — клиент к локальному Ollama.
type OllamaClient struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

// NewOllamaClient — конструктор. Пустой baseURL → localhost:11434.
func NewOllamaClient(baseURL, model string) *OllamaClient {
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	return &OllamaClient{
		BaseURL: baseURL,
		Model:   model,
		HTTP:    &http.Client{Timeout: 30 * time.Minute}, // локальные модели могут генерить долго
	}
}

var _ StreamingLLM = (*OllamaClient)(nil)

// Stream — POST /v1/chat/completions с stream=true. Ollama возвращает
// стандартный OpenAI SSE, парсим тем же readGLMSSE.
func (c *OllamaClient) Stream(ctx context.Context, messages []AIMessage, tools []map[string]any, cb StreamCallbacks) (*StreamResult, error) {
	body, err := json.Marshal(glmRequest{
		Model:    c.Model,
		Messages: messages,
		Tools:    tools,
		Stream:   true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ollama %s недоступен: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama: модель %q не установлена (ollama pull %s?). %s",
			c.Model, c.Model, truncate(string(raw), 200))
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama ответ %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}

	res, err := readGLMSSE(resp.Body, cb)
	if res != nil {
		logRequest(reqLogEntry{
			Source:         "ollama",
			BaseURL:        c.BaseURL,
			ModelRequested: c.Model,
			ModelReturned:  c.Model,
			Status:         "ok",
			ContentLen:     len(res.Content),
			ToolCalls:      len(res.ToolCalls),
		})
	}
	return res, err
}

// FetchOllamaModels — GET /api/tags. Возвращает список установленных
// локально моделей. Формат:
//
//	{"models":[{"name":"llama3.2:latest","size":..., "details":{...}}, ...]}
func FetchOllamaModels(ctx context.Context, baseURL string) ([]Model, error) {
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	// Короткий таймаут — localhost должен отвечать мгновенно.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ollama %s недоступен: %w. Запусти: ollama serve", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama /api/tags вернул %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	var body struct {
		Models []struct {
			Name    string `json:"name"`
			Details struct {
				ParameterSize string `json:"parameter_size"`
				Family        string `json:"family"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("Ollama /api/tags парсинг: %w", err)
	}
	if len(body.Models) == 0 {
		return nil, errors.New("в Ollama нет моделей. Установи что-нибудь: `ollama pull llama3.2`")
	}

	out := make([]Model, 0, len(body.Models))
	for i, m := range body.Models {
		desc := m.Details.Family
		if m.Details.ParameterSize != "" {
			if desc != "" {
				desc += " · "
			}
			desc += m.Details.ParameterSize
		}
		if desc == "" {
			desc = "локальная модель Ollama"
		}
		out = append(out, Model{
			ID:        m.Name,
			Provider:  "ollama",
			Name:      m.Name,
			Description: desc,
			Tier:      "standard",
			IsPrimary: i == 0, // первая (обычно latest pulled) — primary
			HasTools:  true,
		})
	}
	return out, nil
}
