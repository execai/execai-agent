package connect

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
