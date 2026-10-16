package chat

import (
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// TestApplySubscriptionSource_OpenAI — /source openai → the live list of the
// account (chat models only) + primary gpt-5.
func TestApplySubscriptionSource_OpenAI(t *testing.T) {
	subs := &subscriptions.Store{
		Subscriptions: map[string]subscriptions.Subscription{
			subscriptions.SourceOpenAI: {
				Provider: subscriptions.SourceOpenAI,
				APIKey:   "sk-proj-test",
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
	_ = subs.Activate(subscriptions.SourceOpenAI)
	m.applySubscriptionSource()

	if m.current.Provider != "openai" {
		t.Errorf("provider want openai, got %q", m.current.Provider)
	}
	if m.current.ID != "gpt-5" {
		t.Errorf("primary want gpt-5, got %q", m.current.ID)
	}
	for _, mod := range m.models {
		if mod.Provider != "openai" {
			t.Errorf("model %s provider = %s, want openai", mod.ID, mod.Provider)
		}
		if mod.ID == "text-embedding-3-large" {
			t.Error("embedding model offered as a chat model")
		}
	}
}
