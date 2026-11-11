package llmpick

import (
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// The background daemon used to send the selected ExecAI model ID to the
// person's own subscription. Model keeps the choice inside the active
// source's catalog, exactly as the terminal and the editor do.
func TestModel_StaysWithinTheActiveSource(t *testing.T) {
	execai := []llm.Model{{ID: "glm-flash", Provider: "zhipu"}, {ID: "sonnet", Provider: "anthropic"}}

	// No subscription: the selected ExecAI model, with its provider.
	if id, p := Model("sonnet", &subscriptions.Store{}, execai); id != "sonnet" || p != "anthropic" {
		t.Errorf("execai selected: %q %q", id, p)
	}
	// Unknown selection: the first (primary) ExecAI model.
	if id, _ := Model("gone", nil, execai); id != "glm-flash" {
		t.Errorf("execai fallback: %q", id)
	}
	// Nothing at all.
	if id, _ := Model("x", nil, nil); id != "" {
		t.Errorf("empty catalog gave %q", id)
	}

	// A subscription: the ExecAI selection is NOT carried over.
	st := &subscriptions.Store{Active: "zai", Subscriptions: map[string]subscriptions.Subscription{
		"zai": {Provider: "zai", APIKey: "k", AvailableModels: []string{"glm-5.3-flashx", "glm-5.3"}},
	}}
	id, p := Model("sonnet", st, execai)
	if id == "sonnet" || id == "" || p != "" {
		t.Errorf("subscription got %q (provider %q)", id, p)
	}
	// The selected model of the subscription itself is kept.
	if id, _ := Model("glm-5.3", st, execai); id != "glm-5.3" {
		t.Errorf("subscription selection lost: %q", id)
	}
}
