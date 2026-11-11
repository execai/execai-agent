package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// WebSearchTool queries the web through our own gateway (browser-vbai), which
// fronts Perplexity / OpenAI web_search, returns a written answer plus cited
// sources, and books the spend against the user's ExecAI billing.
//
// Requires an ExecAI account: the request is authorised with the user's JWT.
// Without a login the tool stays registered on purpose — it answers with a
// localized explanation so the model can tell the user why search is limited
// and fall back to WebFetch, which works offline from any subscription.
type WebSearchTool struct{}

// searchPath is the gateway alias; browser-vbai registers it itself at startup
// (accessType Internal — a user JWT is enough).
const searchPath = "/browser-vbai/input"

func (*WebSearchTool) Spec() Spec {
	return Spec{
		Name: "WebSearch",
		Description: "Поиск в интернете с ответом и списком источников. " +
			"Используй, когда нужны свежие данные, документация, разбор ошибки, " +
			"сравнение версий — то, чего нет в контексте. Возвращает связный ответ " +
			"и пронумерованные источники с URL: любой из них можно затем открыть " +
			"целиком через WebFetch. Требуется аккаунт ExecAI.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Поисковый запрос на естественном языке.",
				},
				"mode": map[string]any{
					"type": "string",
					"enum": []string{"fast", "deep"},
					"description": "fast (по умолчанию) — обычный поиск. " +
						"deep — глубокое исследование: заметно дороже и дольше, " +
						"брать только под явно исследовательские задачи.",
				},
				"domains": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Ограничить поиск доменами, например [\"pkg.go.dev\"].",
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
	}
}

// Search is read-only and billed to the user's own account — no confirmation.
func (t *WebSearchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Query   string   `json:"query"`
		Mode    string   `json:"mode"`
		Domains []string `json:"domains"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	p.Query = strings.TrimSpace(p.Query)
	if p.Query == "" {
		return "", fmt.Errorf("query обязателен")
	}
	if p.Mode != "deep" {
		p.Mode = "fast"
	}

	// Credentials are read on every call, not captured at startup: the user may
	// log in mid-session (no-login mode → /login), and search must light up
	// right away without a restart.
	base, token := searchEndpoint()
	if token == "" {
		return i18n.T("tool.websearch.noLogin"), nil
	}

	body := map[string]any{"text": p.Query, "mode": p.Mode}
	if len(p.Domains) > 0 {
		body["allowed_domains"] = p.Domains
	}
	// conversation_id correlates the spend with a CLI session on the billing side;
	// without it browser-vbai logs a warning and the charge lands unattributed.
	if sid := currentSessionID(); sid != "" {
		body["conversation_id"] = sid
	}
	payload, _ := json.Marshal(body)

	// deep mode legitimately runs for minutes.
	timeout := 90 * time.Second
	if p.Mode == "deep" {
		timeout = 300 * time.Second
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(rctx, http.MethodPost, base+searchPath, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", fmt.Errorf("поиск недоступен: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return i18n.T("tool.websearch.noLogin"), nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("поиск вернул HTTP %d", resp.StatusCode)
	}

	text, sources, err := parseSearchStream(resp.Body)
	if err != nil {
		return "", err
	}
	if text == "" && len(sources) == 0 {
		return i18n.T("tool.websearch.empty"), nil
	}

	var out strings.Builder
	out.WriteString(text)
	if len(sources) > 0 {
		out.WriteString("\n\n" + i18n.T("tool.websearch.sources") + "\n")
		for i, s := range sources {
			title := s.Title
			if title == "" {
				title = s.Domain
			}
			fmt.Fprintf(&out, "[%d] %s — %s\n", i+1, title, s.URL)
		}
	}
	return out.String(), nil
}

// searchEndpoint returns the gateway base URL and the user's JWT ("" if not logged in).
func searchEndpoint() (string, string) {
	base := "https://api.execai.ru"
	if cfg, err := config.Load(); err == nil && cfg != nil && cfg.APIBase != "" {
		base = strings.TrimRight(cfg.APIBase, "/")
	}
	cr, err := config.LoadCredentials()
	if err != nil || cr == nil {
		return base, ""
	}
	return base, cr.Token
}

// currentSessionID is wired by the chat layer so search spend can be tied to a
// CLI session; empty is acceptable (billing just records it unattributed).
var currentSessionID = func() string { return "" }

// SetSessionIDFunc lets the chat layer expose the active session id to the tools.
func SetSessionIDFunc(f func() string) {
	if f != nil {
		currentSessionID = f
	}
}

type searchSource struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	Domain string `json:"domain"`
}

// parseSearchStream decodes the SSE dialect of browser-vbai:
//
//	data: [FUNCTION_START]
//	data: {"function_result":"function_name","name":"browse_web"}
//	data: {"function_result":"output","content":"<base64 chunk>"}
//	data: {"function_result":"source","url":…,"title":…,"domain":…}
//	data: {"function_result":"status","exit_code":0}
//	data: [FUNCTION_END]
//
// Two server-side quirks are compensated here (see docs in the memo):
//   - chunks are base64 of a byte slice, and a multi-byte character can be split
//     across two chunks — so bytes are accumulated and decoded once at the end,
//     never per chunk;
//   - the answer arrives TWICE (streamed deltas, then the full text again).
func parseSearchStream(r io.Reader) (string, []searchSource, error) {
	var raw bytes.Buffer
	var sources []searchSource
	seen := map[string]bool{}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || strings.HasPrefix(payload, "[") {
			continue // [FUNCTION_START] / [FUNCTION_END]
		}
		var ev struct {
			Kind    string `json:"function_result"`
			Content string `json:"content"`
			searchSource
		}
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue // a malformed event must not kill the whole answer
		}
		switch ev.Kind {
		case "output":
			if b, err := base64.StdEncoding.DecodeString(ev.Content); err == nil {
				raw.Write(b)
			}
		case "source":
			if ev.URL != "" && !seen[ev.URL] {
				seen[ev.URL] = true
				sources = append(sources, ev.searchSource)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", nil, fmt.Errorf("обрыв потока поиска: %w", err)
	}
	return cleanSearchText(raw.String()), sources, nil
}

// cleanSearchText strips the server's "[Поиск] <query>" banner (hardcoded in
// Russian upstream, while our UI speaks five languages) and undoes the doubled
// answer described above.
func cleanSearchText(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[Поиск]") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = strings.TrimSpace(s[i+1:])
		} else {
			s = ""
		}
	}
	return stripSourceListing(dedupeDoubledText(s))
}

// dedupeDoubledText undoes the server sending the answer twice.
//
// Observed shape: streamed deltas produce one copy, then the final message
// repeats the whole answer and appends its own plain-text source listing:
//
//	ANSWER ANSWER  → Title \n    URL  → Title \n    URL …
//
// So the second copy is not necessarily the same length as the first — the test
// is "everything from the cut point onwards begins with what came before it".
func dedupeDoubledText(s string) string {
	if len(s) < 80 {
		return s
	}
	if half := len(s) / 2; len(s)%2 == 0 && s[:half] == s[half:] {
		return s[:half]
	}
	head := s
	if len(head) > 120 {
		head = head[:120]
	}
	if j := strings.Index(s[len(head):], head); j >= 0 {
		cut := len(head) + j
		// The tail must be at least as long as the copy it repeats — otherwise
		// a merely periodic text (a table of similar rows) could be truncated.
		if len(s)-cut >= cut && strings.HasPrefix(s[cut:], s[:cut]) {
			return strings.TrimSpace(s[:cut])
		}
	}
	return s
}

// stripSourceListing drops the trailing "  → Title / URL" block: the same links
// arrive as structured `source` events and are rendered by the caller, so
// keeping the plain-text copy only burns context.
func stripSourceListing(s string) string {
	lines := strings.Split(s, "\n")
	cut := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		ln := strings.TrimSpace(lines[i])
		if ln == "" || strings.HasPrefix(ln, "→ ") || strings.HasPrefix(ln, "http://") || strings.HasPrefix(ln, "https://") {
			cut = i
			continue
		}
		break
	}
	if cut == len(lines) {
		return s
	}
	// The listing may start on the same line the answer ends on.
	if idx := strings.Index(lines[cut], "  → "); idx > 0 {
		lines[cut] = lines[cut][:idx]
		cut++
	}
	return strings.TrimSpace(strings.Join(lines[:cut], "\n"))
}
