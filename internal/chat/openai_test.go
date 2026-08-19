package chat

import (
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// TestApplySubscriptionSource_OpenAI — /source openai → OpenAIModels + primary gpt-5.
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
	}
}
func TestOpenAIModels_ContainsExpected(t *testing.T) {
	models := llm.OpenAIModels()
	got := map[string]bool{}
	var primary string
	for _, m := range models {
		got[m.ID] = true
		if m.IsPrimary {
			primary = m.ID
		}
	}
	for _, want := range []string{"gpt-5", "gpt-4o", "o3"} {
		if !got[want] {
			t.Errorf("OpenAIModels missing %q", want)
		}
	}
	if primary != "gpt-5" {
		t.Errorf("primary want gpt-5, got %q", primary)
	}
}
