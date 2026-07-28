package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type WebFetchTool struct{}

func (*WebFetchTool) Spec() Spec {
	return Spec{
		Name:        "WebFetch",
		Description: "GET-запрос к указанному URL. Возвращает ответ как текст (с обрезкой до 50KB). Не выполняет JS, не рендерит браузер. Полезно для чтения README на GitHub, JSON-эндпоинтов, документации.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":     map[string]any{"type": "string", "description": "Полный URL (http или https)."},
				"headers": map[string]any{"type": "object", "description": "Дополнительные заголовки (например {'Accept':'application/json'}).", "additionalProperties": map[string]any{"type": "string"}},
				"timeout": map[string]any{"type": "integer", "description": "Таймаут в секундах (по умолчанию 30)."},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		},
	}
}

// WebFetch с user-JWT не делает ничего опасного, но запросы к посторонним
// серверам всё равно могут быть неприятны (например прогрев кэша, утечка IP).
// Default — без подтверждения; если хочешь параноидальнее, поменяй на true.
func (*WebFetchTool) RequiresApproval(json.RawMessage) bool { return false }

func (*WebFetchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Timeout int               `json:"timeout"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	if p.URL == "" {
		return "", fmt.Errorf("url обязателен")
	}
	if !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
		return "", fmt.Errorf("ожидается http:// или https://")
	}
	to := p.Timeout
	if to <= 0 {
		to = 30
	}
	if to > 120 {
		to = 120
	}
	rctx, cancel := context.WithTimeout(ctx, time.Duration(to)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "execai-agent/0.0 (+https://execai.ru)")
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}
	cli := &http.Client{}
	resp, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 100_000))
	out := string(body)
	if len(out) > 50_000 {
		out = out[:50_000] + "\n...(ответ обрезан до 50KB)"
	}
	return fmt.Sprintf("HTTP %d %s\nContent-Type: %s\n\n%s",
		resp.StatusCode, resp.Status, resp.Header.Get("Content-Type"), out), nil
}
