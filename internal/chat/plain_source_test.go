package chat

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llmpick"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// capture runs fn with os.Stdout / os.Stderr redirected and returns both.
func capture(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wo, we
	fn()
	os.Stdout, os.Stderr = oldOut, oldErr
	wo.Close()
	we.Close()
	o, _ := io.ReadAll(ro)
	e, _ := io.ReadAll(re)
	return string(o), string(e)
}

// execai run follows the active source like the TUI: an own key works
// without signing in, and nothing goes to the ExecAI backend.
func TestOnce_OwnSourceWithoutLogin(t *testing.T) {
	var calls atomic.Int32
	prov := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("unexpected provider path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer prov.Close()
	useStore(t, &subscriptions.Store{Active: "openai", Subscriptions: map[string]subscriptions.Subscription{
		"openai": {Provider: "openai", APIKey: "k", BaseURL: prov.URL + "/v1", AvailableModels: []string{"m1"}}}})

	cfg := &config.Config{APIBase: "http://127.0.0.1:1", Locale: "en"}
	var err error
	out, _ := capture(t, func() { err = Once(context.Background(), cfg, "ping") })
	if err != nil {
		t.Fatalf("own source without login: %v", err)
	}
	if calls.Load() != 1 || !strings.Contains(out, "pong") {
		t.Errorf("provider calls %d, output %q", calls.Load(), out)
	}
}

// A CLI source without its binary falls back to the ExecAI models: run says
// so in one line, and then the ExecAI rules apply (here: sign in).
func TestOnce_CLIFallbackSaysSo(t *testing.T) {
	old := llmpick.CLIAvailable
	llmpick.CLIAvailable = func(string) bool { return false }
	defer func() { llmpick.CLIAvailable = old }()
	useStore(t, &subscriptions.Store{Active: "claude-cli", Subscriptions: map[string]subscriptions.Subscription{
		"claude-cli": {Provider: "claude-cli"}}})

	cfg := &config.Config{APIBase: "http://127.0.0.1:1", Locale: "en"}
	var err error
	_, errOut := capture(t, func() { err = Once(context.Background(), cfg, "ping") })
	if err == nil || !strings.Contains(err.Error(), "/login") {
		t.Errorf("fallback without login: %v", err)
	}
	if !strings.Contains(errOut, "claude binary not found") {
		t.Errorf("fallback not announced: %q", errOut)
	}
}
