package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// stubAPI answers every request by the current mode.
type stubAPI struct {
	mu   sync.Mutex
	mode string
	srv  *httptest.Server
}

func newStubAPI(t *testing.T) *stubAPI {
	s := &stubAPI{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		mode := s.mode
		s.mu.Unlock()
		sse := func(line string) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: " + line + "\n\ndata: [DONE]\n\n"))
		}
		switch mode {
		case "401":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("nope"))
		case "401-alias":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Invalid Authorization"))
		case "402", "429", "500", "404":
			code := map[string]int{"402": 402, "429": 429, "500": 500, "404": 404}[mode]
			w.WriteHeader(code)
			_, _ = w.Write([]byte("server text"))
		case "sse-error":
			sse(`{"error":{"message":"boom"}}`)
		case "limit", "funds", "inactive":
			status := map[string]string{"limit": "rate_limit_exceeded", "funds": "insufficient_funds", "inactive": "plan_inactive"}[mode]
			sse(`{"type":"billing_status","status":"` + status + `","message":""}`)
		case "aichat-error":
			sse(`{"type":"error","error":"boom"}`)
		case "not-json":
			_, _ = w.Write([]byte("<html>"))
		case "no-models":
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "one-model":
			_, _ = w.Write([]byte(`{"models":[{"name":"m1"}]}`))
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubAPI) set(mode string) {
	s.mu.Lock()
	s.mode = mode
	s.mu.Unlock()
}

// Every error and notice of the model clients that the person sees — in
// the chat, the panel or a task's answer on the site — is in the
// interface language: the ExecAI backend (401, 402, 429, a server error,
// the plan running out), the web chat stream, a provider key (Anthropic,
// OpenAI-compatible), Ollama, a missing claude or codex CLI, a delegated
// tool left without a result. A 401 is recognised by its type, not by its
// words.
func TestModelErrors_InTheInterfaceLanguage(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("EXECAI_CLAUDE_BIN", "")
	api := newStubAPI(t)
	ctx := context.Background()
	msgs := []AIMessage{{Role: "user", Content: "hi"}}
	check := func(what, text string) {
		t.Helper()
		if text == "" || cyrillicRe.MatchString(text) {
			t.Errorf("%s: %q", what, text)
		}
	}
	errText := func(err error) string {
		if err == nil {
			return ""
		}
		return err.Error()
	}

	ai := NewAICoreClient(api.srv.URL, "tok", "m", "p")
	for _, mode := range []string{"401", "401-alias", "402", "429", "500", "sse-error"} {
		api.set(mode)
		_, err := ai.Stream(ctx, msgs, nil, StreamCallbacks{})
		check("aicore "+mode, errText(err))
		if (mode == "401" || mode == "401-alias") && !errors.Is(err, ErrUnauthorized) {
			t.Errorf("aicore %s: %v is not ErrUnauthorized", mode, err)
		}
		if mode == "402" && !errors.Is(err, ErrPaymentRequired) {
			t.Errorf("aicore 402: %v is not ErrPaymentRequired", err)
		}
	}
	for _, mode := range []string{"limit", "funds", "inactive"} {
		api.set(mode)
		var shown string
		if _, err := ai.Stream(ctx, msgs, nil, StreamCallbacks{OnText: func(s string) { shown += s }}); err != nil {
			t.Fatalf("billing %s: %v", mode, err)
		}
		check("billing "+mode, shown)
	}

	web := New(api.srv.URL, "tok", "m", "p")
	for _, mode := range []string{"401", "500", "aichat-error"} {
		api.set(mode)
		_, err := web.Stream(ctx, []Message{{Role: "user", Content: "hi"}}, func(string) {})
		check("aichat "+mode, errText(err))
		if mode == "401" && !errors.Is(err, ErrUnauthorized) {
			t.Errorf("aichat 401: %v is not ErrUnauthorized", err)
		}
	}

	api.set("401")
	_, err := NewAnthropicClient(api.srv.URL, "k", "m", 0).Stream(ctx, msgs, nil, StreamCallbacks{})
	check("anthropic 401", errText(err))
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("anthropic 401: %v", err)
	}
	glm := NewGLMClient(api.srv.URL, "k", "m")
	for _, mode := range []string{"401", "500"} {
		api.set(mode)
		_, err := glm.Stream(ctx, msgs, nil, StreamCallbacks{})
		check("openai-compatible "+mode, errText(err))
		if mode == "401" && !errors.Is(err, ErrUnauthorized) {
			t.Errorf("openai-compatible 401: %v", err)
		}
	}

	_, err = NewOllamaClient("http://127.0.0.1:1", "m").Stream(ctx, msgs, nil, StreamCallbacks{})
	check("ollama down", errText(err))
	for _, mode := range []string{"404", "500"} {
		api.set(mode)
		_, err := NewOllamaClient(api.srv.URL, "m").Stream(ctx, msgs, nil, StreamCallbacks{})
		check("ollama "+mode, errText(err))
	}
	_, err = FetchOllamaModelsAuth(ctx, "http://127.0.0.1:1", "")
	check("ollama tags down", errText(err))
	for _, mode := range []string{"500", "not-json", "no-models"} {
		api.set(mode)
		_, err := FetchOllamaModelsAuth(ctx, api.srv.URL, "")
		check("ollama tags "+mode, errText(err))
	}
	api.set("one-model")
	if models, err := FetchOllamaModelsAuth(ctx, api.srv.URL, ""); err != nil || len(models) != 1 {
		t.Fatalf("ollama models: %v %v", models, err)
	} else {
		check("ollama model description", models[0].Description)
	}

	_, err = NewClaudeCLIClient("")
	check("claude missing", errText(err))
	_, err = ListClaudeCLIModels(ctx)
	check("claude models, claude missing", errText(err))
	_, err = NewCodexCLIClient("")
	check("codex missing", errText(err))
	_, err = ListCodexCLIModels(ctx)
	check("codex models, codex missing", errText(err))

	var result string
	seq := &toolSequencer{cb: StreamCallbacks{OnDelegatedToolResult: func(_, r string, _ bool) { result = r }}}
	raw, _ := json.Marshal(map[string]any{"content": []map[string]any{{"type": "tool_use", "id": "t1", "name": "Bash", "input": map[string]any{}}}})
	seq.fromAssistant(raw)
	seq.finish()
	check("delegated tool without a result", result)
}

// The plan notice in the model stream names this installation's web, not
// the production site.
func TestBillingNotice_ThisInstallationsWeb(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	if err := config.SaveCredentials(&config.Credentials{Token: "t", WebBase: "https://web.example.org"}); err != nil {
		t.Fatal(err)
	}
	api := newStubAPI(t)
	api.set("limit")
	var shown string
	if _, err := NewAICoreClient(api.srv.URL, "tok", "m", "p").Stream(context.Background(), []AIMessage{{Role: "user", Content: "hi"}}, nil,
		StreamCallbacks{OnText: func(s string) { shown += s }}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(shown, "https://web.example.org") || strings.Contains(shown, "execai.ru") {
		t.Errorf("plan notice: %q", shown)
	}
}
