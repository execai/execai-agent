package connect

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// Каждый провайдер спрашивается на своём диалекте: перепутанный заголовок или
// путь — это молчащий каталог и «новых моделей не видно».
func TestFetchModelsFor_Dialects(t *testing.T) {
	t.Run("anthropic: x-api-key и версия", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-api-key") != "sk-ant-x" || r.Header.Get("anthropic-version") == "" {
				w.WriteHeader(401)
				return
			}
			if r.URL.Path != "/v1/models" {
				w.WriteHeader(404)
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-opus-5"},{"id":"claude-sonnet-4-6"}]}`))
		}))
		defer srv.Close()
		ids, st := FetchModelsFor(subscriptions.SourceAnthropic, srv.URL, "sk-ant-x")
		if st != 200 || len(ids) != 2 || ids[0] != "claude-opus-5" {
			t.Fatalf("status=%d ids=%v", st, ids)
		}
	})

	t.Run("ollama: /api/tags без ключа", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/tags" {
				w.WriteHeader(404)
				return
			}
			_, _ = w.Write([]byte(`{"models":[{"model":"llama3.2:latest"},{"name":"qwen3:8b"}]}`))
		}))
		defer srv.Close()
		ids, st := FetchModelsFor(subscriptions.SourceOllama, srv.URL, "")
		if st != 200 || len(ids) != 2 || ids[0] != "llama3.2:latest" || ids[1] != "qwen3:8b" {
			t.Fatalf("status=%d ids=%v", st, ids)
		}
	})

	t.Run("openai-совместимые: bearer и /v1/models", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer sk-or-x" || r.URL.Path != "/v1/models" {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"anthropic/claude-sonnet-4.5"}]}`))
		}))
		defer srv.Close()
		ids, st := FetchModelsFor(subscriptions.SourceOpenRouter, srv.URL+"/v1", "sk-or-x")
		if st != 200 || len(ids) != 1 {
			t.Fatalf("status=%d ids=%v", st, ids)
		}
	})
}

// Names travel with the IDs: Anthropic and Kimi send display_name, OpenRouter
// and Z.ai send name. The picker shows them instead of derived names.
func TestFetch_KeepsProviderNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"kimi-for-coding","display_name":"K2.8 Preview"},{"id":"glm-5.3","name":"GLM 5.3"},{"id":"plain"},{"id":"same","name":"same"}]}`))
	}))
	defer srv.Close()
	ms, st := Fetch(subscriptions.Subscription{Provider: subscriptions.SourceKimi, BaseURL: srv.URL, APIKey: "k"})
	if st != 200 || len(ms) != 4 {
		t.Fatalf("status=%d ms=%v", st, ms)
	}
	if ms[0].Name != "K2.8 Preview" || ms[1].Name != "GLM 5.3" || ms[2].Name != "" || ms[3].Name != "" {
		t.Errorf("имена: %+v", ms)
	}
}

// A base with its own version segment is used as is. Z.ai's open platform
// lives on /api/paas/v4: appending /v1 asked a different, undocumented route.
func TestFetchOpenAI_VersionedBase(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"data":[{"id":"glm-5.3"}]}`))
	}))
	defer srv.Close()
	for base, want := range map[string]string{
		srv.URL + "/api/paas/v4": "/api/paas/v4/models",
		srv.URL + "/v1":          "/v1/models",
		srv.URL + "/coding":      "/coding/v1/models",
		srv.URL:                  "/v1/models",
	} {
		if _, st := FetchModels(base, "k"); st != 200 || path != want {
			t.Errorf("%s → %s (status %d), ожидался %s", base, path, st, want)
		}
	}
}

// Anthropic pages its list; a second page must not be lost.
func TestFetchAnthropic_Pages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("after_id") == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"a","display_name":"A"}],"has_more":true,"last_id":"a"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"b"}],"has_more":false,"last_id":"b"}`))
	}))
	defer srv.Close()
	ms, st := Fetch(subscriptions.Subscription{Provider: subscriptions.SourceAnthropic, BaseURL: srv.URL, APIKey: "k"})
	if st != 200 || len(ms) != 2 || ms[0].Name != "A" || ms[1].ID != "b" {
		t.Fatalf("status=%d ms=%+v", st, ms)
	}
}

func TestNormalizeCustomBase(t *testing.T) {
	cases := []struct{ api, in, want string }{
		{subscriptions.APIOpenAI, "http://localhost:1234/v1", "http://localhost:1234/v1"},
		{subscriptions.APIOpenAI, "localhost:1234", "http://localhost:1234"},
		{subscriptions.APIOpenAI, "https://api.deepseek.com/", "https://api.deepseek.com"},
		{subscriptions.APIOpenAI, "https://gw.corp/llm/v1/chat/completions", "https://gw.corp/llm/v1"},
		{subscriptions.APIOpenAI, "https://open.bigmodel.cn/api/paas/v4", "https://open.bigmodel.cn/api/paas/v4"},
		{subscriptions.APIAnthropic, "https://llm.corp.local/v1", "https://llm.corp.local"},
		{subscriptions.APIAnthropic, "https://llm.corp.local/v1/messages", "https://llm.corp.local"},
		{subscriptions.APIAnthropic, "https://api.deepseek.com/anthropic", "https://api.deepseek.com/anthropic"},
	}
	for _, c := range cases {
		got, err := NormalizeCustomBase(c.api, c.in)
		if err != nil || got != c.want {
			t.Errorf("%s %q → %q (%v), ожидался %q", c.api, c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "ftp://x", "http://"} {
		if _, err := NormalizeCustomBase(subscriptions.APIOpenAI, bad); err == nil {
			t.Errorf("%q должен быть отвергнут", bad)
		}
	}
}

// A custom endpoint is asked in its protocol, with its key and headers; a
// local server without a key gets no Authorization header at all.
func TestPrepareCustom(t *testing.T) {
	var gotAuth, gotTenant, gotXKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotTenant, gotXKey = r.Header.Get("Authorization"), r.Header.Get("X-Tenant"), r.Header.Get("x-api-key")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"qwen3-coder"},{"id":"llama-4"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	sub, st, err := PrepareCustom(Custom{Name: "LM Studio", API: "1", BaseURL: srv.URL})
	if err != nil || st != 200 {
		t.Fatalf("err=%v status=%d", err, st)
	}
	if sub.Provider != "lm-studio" || sub.Name != "LM Studio" || sub.API != subscriptions.APIOpenAI || !sub.IsCustom() {
		t.Errorf("подписка: %+v", sub)
	}
	if sub.BaseURL != srv.URL+"/v1" || len(sub.AvailableModels) != 2 || len(sub.Models) != 2 {
		t.Errorf("адрес/модели: %s %v", sub.BaseURL, sub.AvailableModels)
	}
	if gotAuth != "" {
		t.Errorf("без ключа Authorization не нужен, пришло %q", gotAuth)
	}

	sub, st, err = PrepareCustom(Custom{Name: "corp", API: "anthropic", BaseURL: srv.URL + "/v1/messages",
		APIKey: "sk-c", Headers: map[string]string{"X-Tenant": "dev"}, Models: []string{"a, b", "a"}})
	if err != nil || st != 200 {
		t.Fatalf("anthropic: err=%v status=%d", err, st)
	}
	if sub.BaseURL != srv.URL || gotXKey != "sk-c" || gotTenant != "dev" {
		t.Errorf("anthropic: base=%s x-api-key=%q tenant=%q", sub.BaseURL, gotXKey, gotTenant)
	}
	if strings.Join(sub.ManualModels, ",") != "a,b" {
		t.Errorf("ручные модели: %v", sub.ManualModels)
	}

	for _, bad := range []Custom{
		{Name: "kimi", API: "openai", BaseURL: srv.URL},    // a built-in id
		{Name: "Шлюз", API: "openai", BaseURL: srv.URL},    // no latin letters
		{Name: "x", API: "gemini", BaseURL: srv.URL},       // unknown protocol
		{Name: "x", API: "openai", BaseURL: "not a url::"}, // address
	} {
		if _, _, err := PrepareCustom(bad); err == nil {
			t.Errorf("%+v должен быть отвергнут", bad)
		}
	}
}

// Keyless sources (the CLIs, a local Ollama, custom endpoints) refresh too —
// RefreshModels used to skip every subscription without an API key.
func TestNeedsKey(t *testing.T) {
	for p, want := range map[string]bool{
		subscriptions.SourceClaudeCLI: false, subscriptions.SourceCodexCLI: false,
		subscriptions.SourceOllama: false, subscriptions.SourceKimi: true, subscriptions.SourceOpenAI: true,
	} {
		if got := NeedsKey(subscriptions.Subscription{Provider: p}); got != want {
			t.Errorf("%s: NeedsKey=%v", p, got)
		}
	}
	if NeedsKey(subscriptions.Subscription{Provider: "lm", API: subscriptions.APIOpenAI}) {
		t.Error("кастомный провайдер может работать без ключа")
	}
}

func TestStale_LegacyIDOnlyListIsRefetched(t *testing.T) {
	s := subscriptions.Subscription{AvailableModels: []string{"a"}, ModelsFetchedAt: time.Now()}
	if !Stale(s) {
		t.Error("список без имён (старая сборка) должен перезапроситься")
	}
	s.SetModels([]subscriptions.ModelMeta{{ID: "a"}}, time.Now())
	if Stale(s) {
		t.Error("свежий список не должен считаться устаревшим")
	}
}

// Dated lists come newest first, whatever order the provider used.
func TestFetch_NewestFirstWhenDated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"glm-4.5","created":1753632000},{"id":"glm-5.3","created":1787745612},{"id":"glm-4.7","created":1766332800}]}`))
	}))
	defer srv.Close()
	ids, _ := FetchModels(srv.URL+"/v4", "k")
	if strings.Join(ids, ",") != "glm-5.3,glm-4.7,glm-4.5" {
		t.Errorf("порядок %v", ids)
	}
}

// Whether an OpenAI-style address needs "/v1" is settled by asking: the model
// list and the chat must use the same base (live run: "https://openrouter.ai/api"
// listed models from /api/v1 but chatted on /api/chat/completions → 404).
func TestPrepareCustom_PicksTheBaseThatAnswers(t *testing.T) {
	root := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" { // a Perplexity-like server: no version segment
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"sonar"}]}`))
	}))
	defer root.Close()
	sub, st, err := PrepareCustom(Custom{Name: "root", API: "openai", BaseURL: root.URL})
	if err != nil || st != 200 || sub.BaseURL != root.URL {
		t.Errorf("корневой сервер: base=%s status=%d err=%v", sub.BaseURL, st, err)
	}

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" { // OpenRouter-like
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"x"}]}`))
	}))
	defer api.Close()
	sub, st, _ = PrepareCustom(Custom{Name: "or", API: "openai", BaseURL: api.URL + "/api"})
	if st != 200 || sub.BaseURL != api.URL+"/api/v1" {
		t.Errorf("/api: base=%s status=%d", sub.BaseURL, st)
	}

	// Nothing answers: a bare host gets /v1, a longer path stays as typed.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer dead.Close()
	if sub, _, _ := PrepareCustom(Custom{Name: "d", API: "openai", BaseURL: dead.URL}); sub.BaseURL != dead.URL+"/v1" {
		t.Errorf("голый хост без ответа: %s", sub.BaseURL)
	}
	if sub, _, _ := PrepareCustom(Custom{Name: "d", API: "openai", BaseURL: dead.URL + "/gw"}); sub.BaseURL != dead.URL+"/gw" {
		t.Errorf("путь без ответа: %s", sub.BaseURL)
	}
}
