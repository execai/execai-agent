package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type WebFetchTool struct{}

func (*WebFetchTool) Spec() Spec {
	return Spec{
		Name: "WebFetch",
		Description: "Открыть страницу по URL. HTML превращается в читаемый текст, " +
			"а исходящие ссылки отдаются отдельным списком с абсолютными URL — " +
			"по ним можно переходить следующим вызовом WebFetch. JSON и обычный " +
			"текст возвращаются как есть. JS не выполняется. Работает всегда, " +
			"без аккаунта ExecAI; для поиска по интернету есть WebSearch.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":     map[string]any{"type": "string", "description": "Полный URL (http или https)."},
				"headers": map[string]any{"type": "object", "description": "Дополнительные заголовки (например {'Accept':'application/json'}).", "additionalProperties": map[string]any{"type": "string"}},
				"timeout": map[string]any{"type": "integer", "description": "Таймаут в секундах (по умолчанию 30)."},
				"raw":     map[string]any{"type": "boolean", "description": "Вернуть исходный HTML без извлечения текста (по умолчанию false)."},
				"links":   map[string]any{"type": "boolean", "description": "Показывать список ссылок со страницы (по умолчанию true)."},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		},
	}
}

// WebFetch with the user JWT does nothing dangerous, but requests to
// third-party servers can still be unpleasant (e.g. cache warming, IP leak).
// Default — no confirmation; change to true if you want to be more paranoid.
func (*WebFetchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Timeout int               `json:"timeout"`
		Raw     bool              `json:"raw"`
		Links   *bool             `json:"links"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	showLinks := p.Links == nil || *p.Links
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

	// Raw budget is generous because markup shrinks a lot once tags are gone;
	// the readable text is what gets capped.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	ct := resp.Header.Get("Content-Type")
	raw := string(body)

	var head strings.Builder
	fmt.Fprintf(&head, "HTTP %d %s\nContent-Type: %s\n", resp.StatusCode, resp.Status, ct)
	// The final URL differs from the requested one after redirects — the model
	// needs it to resolve relative links correctly.
	final := resp.Request.URL.String()
	if final != "" && final != p.URL {
		fmt.Fprintf(&head, "Итоговый URL (после редиректов): %s\n", final)
	}

	if p.Raw || !looksLikeHTML(ct, raw) {
		return wrapUntrusted(final, head.String()+"\n"+clampText(raw, 50_000)), nil
	}

	title, text, links := htmlToText(raw, final)
	if title != "" {
		fmt.Fprintf(&head, "Заголовок: %s\n", title)
	}
	out := head.String() + "\n" + clampText(text, 50_000)

	if showLinks && len(links) > 0 {
		const maxLinks = 60
		shown := links
		if len(shown) > maxLinks {
			shown = shown[:maxLinks]
		}
		var lb strings.Builder
		fmt.Fprintf(&lb, "\n\n=== Ссылки со страницы (%d", len(shown))
		if len(links) > len(shown) {
			fmt.Fprintf(&lb, " из %d", len(links))
		}
		lb.WriteString("), открыть можно через WebFetch ===\n")
		for _, l := range shown {
			fmt.Fprintf(&lb, "- %s → %s\n", l.Text, l.URL)
		}
		out += lb.String()
	}
	return wrapUntrusted(final, out), nil
}

func clampText(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	// Cut on a rune boundary so the tail is not a broken character.
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "\n…(обрезано, показано " + fmt.Sprint(cut) + " из " + fmt.Sprint(len(s)) + " байт)"
}
