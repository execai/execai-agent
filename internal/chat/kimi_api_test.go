package chat

import (
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// TestApplySubscriptionSource_KimiAPI — /source kimi-api pulls the live
// Moonshot list; primary = kimi-k3 (preference, not the server's order).
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
	if m.current.ID != "kimi-k3" {
		t.Errorf("primary model: want kimi-k3, got %q", m.current.ID)
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
	// The status bar shows the readable name, not the id: "kimi-api" said
	// nothing about being the Moonshot platform rather than the Coding Plan.
	if label != "Moonshot (api)" {
		t.Errorf("want 'Moonshot (api)', got %q", label)
	}
}
