package chat

import (
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// TestApplySubscriptionSource_KimiAPI — /source kimi-api pulls MoonshotModels,
// primary model = kimi-latest.
func TestApplySubscriptionSource_KimiAPI(t *testing.T) {
	subs := &subscriptions.Store{
		Subscriptions: map[string]subscriptions.Subscription{
			subscriptions.SourceKimiAPI: {
				Provider: subscriptions.SourceKimiAPI,
				APIKey:   "sk-test",
				Plan:     "api",
			},
		},
	}
	m := &tuiModel{
		cfg:          &config.Config{APIBase: "https://api.execai.ru"},
		creds:        &config.Credentials{Token: "stub"},
		subs:         subs,
		models:       execAISnapshot(),
		execAIModels: execAISnapshot(),
	}
	_ = subs.Activate(subscriptions.SourceKimiAPI)
	m.applySubscriptionSource()

	if m.current.Provider != "kimi-api" {
		t.Errorf("provider: want kimi-api, got %q", m.current.Provider)
	}
	if m.current.ID != "kimi-latest" {
		t.Errorf("primary model: want kimi-latest, got %q", m.current.ID)
	}
	if len(m.models) == 0 {
		t.Fatalf("models catalog empty")
	}
	// Must be Moonshot ones (kimi-api provider).
	for _, mod := range m.models {
		if mod.Provider != "kimi-api" {
			t.Errorf("model %q has provider %q, want kimi-api", mod.ID, mod.Provider)
		}
	}
}

// TestMoonshotModels_ContainsExpected — sanity: MoonshotModels() returns what we expect.
func TestMoonshotModels_ContainsExpected(t *testing.T) {
	models := llm.MoonshotModels()
	want := []string{"kimi-latest", "moonshot-v1-128k", "moonshot-v1-32k"}
	got := map[string]bool{}
	for _, m := range models {
		got[m.ID] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("MoonshotModels missing %q", w)
		}
	}
	// Primary = kimi-latest.
	var primary string
	for _, m := range models {
		if m.IsPrimary {
			primary = m.ID
			break
		}
	}
	if primary != "kimi-latest" {
		t.Errorf("primary = %q, want kimi-latest", primary)
	}
}

// TestSourceLabel_KimiAPI — status bar for kimi-api.
func TestSourceLabel_KimiAPI(t *testing.T) {
	s := &subscriptions.Store{
		Active: subscriptions.SourceKimiAPI,
		Subscriptions: map[string]subscriptions.Subscription{
			subscriptions.SourceKimiAPI: {
				Provider: subscriptions.SourceKimiAPI,
				Plan:     "api",
			},
		},
	}
	label := s.SourceLabel()
	// Kimi API — plan="api" → "kimi-api (api)".
	if label != "kimi-api (api)" {
		t.Errorf("want 'kimi-api (api)', got %q", label)
	}
}
