// Лог-файл LLM-запросов: ~/.config/execai/requests.log
// Формат строки JSON-lines: {ts, source, base, model_requested, model_returned, status, content_len, tool_calls, err}
//
// Используется чтобы убедиться какая модель ОТВЕТИЛА (не та что мы запросили,
// если провайдер делает server-side mapping типа Z.ai Coding Plan).
package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// reqLogEntry — одна запись.
type reqLogEntry struct {
	Ts             string `json:"ts"`
	Source         string `json:"source"`           // execai | zai | ...
	BaseURL        string `json:"base_url"`
	ModelRequested string `json:"model_requested"`
	ModelReturned  string `json:"model_returned,omitempty"` // что сервер прислал в message_start (Anthropic API)
	Status         string `json:"status"`           // ok | error | ...
	ContentLen     int    `json:"content_len"`
	ToolCalls      int    `json:"tool_calls"`
	// Anthropic возвращает usage в message_delta. Для Z.ai позволяет посчитать
	// сколько токенов потрачено через подписку (хотя бы grosso modo).
	InputTokens       int    `json:"input_tokens,omitempty"`
	OutputTokens      int    `json:"output_tokens,omitempty"`
	CacheReadTokens   int    `json:"cache_read_tokens,omitempty"`
	CacheCreateTokens int    `json:"cache_create_tokens,omitempty"`
	Err               string `json:"err,omitempty"`
}

var reqLogMu sync.Mutex

// logRequest пишет 1 строку в log. Молчит при ошибках (не критичный путь).
func logRequest(e reqLogEntry) {
	if e.Ts == "" {
		e.Ts = time.Now().UTC().Format(time.RFC3339)
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		dir = filepath.Join(home, ".config")
	}
	dir = filepath.Join(dir, "execai")
	_ = os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, "requests.log")

	reqLogMu.Lock()
	defer reqLogMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	if data, err := json.Marshal(e); err == nil {
		_, _ = f.Write(data)
		_, _ = f.Write([]byte("\n"))
	}
}
