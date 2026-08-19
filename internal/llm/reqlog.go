// LLM request log file: ~/.config/execai/requests.log
// JSON-lines format per line: {ts, source, base, model_requested, model_returned, status, content_len, tool_calls, err}
//
// Used to verify which model RESPONDED (not the one we requested,
// when the provider does server-side mapping like the Z.ai Coding Plan).
package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// reqLogEntry is a single record.
type reqLogEntry struct {
	Ts             string `json:"ts"`
	Source         string `json:"source"`           // execai | zai | ...
	BaseURL        string `json:"base_url"`
	ModelRequested string `json:"model_requested"`
	ModelReturned  string `json:"model_returned,omitempty"` // what the server sent in message_start (Anthropic API)
	Status         string `json:"status"`           // ok | error | ...
	ContentLen     int    `json:"content_len"`
	ToolCalls      int    `json:"tool_calls"`
	// Anthropic returns usage in message_delta. For Z.ai it lets us count
	// how many tokens were spent via the subscription (at least grosso modo).
	InputTokens       int    `json:"input_tokens,omitempty"`
	OutputTokens      int    `json:"output_tokens,omitempty"`
	CacheReadTokens   int    `json:"cache_read_tokens,omitempty"`
	CacheCreateTokens int    `json:"cache_create_tokens,omitempty"`
	Err               string `json:"err,omitempty"`
}

var reqLogMu sync.Mutex

// logRequest writes 1 line to the log. Silent on errors (non-critical path).
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
