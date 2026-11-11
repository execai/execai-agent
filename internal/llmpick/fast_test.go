package llmpick

import (
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// "fast" is the cheap model of the ACTIVE source, never one of another.
func TestFast_CheapestOfTheActiveSource(t *testing.T) {
	execai := []llm.Model{
		{ID: "claude-opus", Provider: "anthropic", Tier: "flagship", HasTools: true, IsPrimary: true},
		{ID: "glm-5.3-flash", Provider: "zhipu", Tier: "lite", HasTools: true},
		{ID: "embed-small", Provider: "x", Tier: "lite", HasTools: false},
	}
	if id, p := Fast(nil, execai); id != "glm-5.3-flash" || p != "zhipu" {
		t.Errorf("execai: %q %q", id, p)
	}
	st := &subscriptions.Store{Active: "openai", Subscriptions: map[string]subscriptions.Subscription{
		"openai": {Provider: "openai", APIKey: "k", AvailableModels: []string{"gpt-5.5", "gpt-5.5-mini", "gpt-5.5-nano"}},
	}}
	if id, p := Fast(st, execai); id != "gpt-5.5-nano" || p != "" {
		t.Errorf("subscription: %q %q", id, p)
	}
	// Nothing looks cheap: the source's usual model.
	plain := &subscriptions.Store{Active: "zai", Subscriptions: map[string]subscriptions.Subscription{
		"zai": {Provider: "zai", APIKey: "k", AvailableModels: []string{"glm-5.3", "glm-5.2"}},
	}}
	if id, _ := Fast(plain, execai); id != "glm-5.3" {
		t.Errorf("fallback: %q", id)
	}
}

func TestPicker_RefusesForeignModel(t *testing.T) {
	st := &subscriptions.Store{Active: "zai", Subscriptions: map[string]subscriptions.Subscription{
		"zai": {Provider: "zai", APIKey: "k", AvailableModels: []string{"glm-5.3", "glm-5.3-flash"}},
	}}
	p := NewPicker(&config.Config{APIBase: "http://x"}, st, []llm.Model{{ID: "claude-opus", HasTools: true}}, "tok")
	// The live store changes after the snapshot: the picker keeps its copy.
	st.Active = "execai"
	if _, _, err := p.Pick("claude-opus"); err == nil {
		t.Error("a model of another source was accepted")
	}
	cli, id, err := p.Pick("glm-5.3")
	if err != nil || cli == nil || id != "glm-5.3" {
		t.Errorf("own model: %v %q %v", cli, id, err)
	}
	if _, id, err := p.Pick("fast"); err != nil || id != "glm-5.3-flash" {
		t.Errorf("fast: %q %v", id, err)
	}
}
