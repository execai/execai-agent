package chat

import (
	"os"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/connect"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// TestMain isolates the package from the person's machine.
//
// Two things leak otherwise. Source switching saves the subscription store,
// and without a private config directory a test run would overwrite the real
// ~/.config/execai/subscriptions.json. And the model lists now come from the
// providers, so a switch in a test would go to the network with a stub key —
// fakeProviders answers instead, the way each provider would.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "execai-chat-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CONFIG_HOME", dir)
	os.Setenv("HOME", dir) // macOS looks for the config under HOME
	connect.Fetch = fakeProviders
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fakeProviders — what each provider's model endpoint would report.
func fakeProviders(sub subscriptions.Subscription) ([]connect.ModelMeta, int) {
	list := func(ids ...string) []connect.ModelMeta {
		out := make([]connect.ModelMeta, 0, len(ids))
		for _, id := range ids {
			out = append(out, connect.ModelMeta{ID: id})
		}
		return out
	}
	if sub.IsCustom() {
		if strings.Contains(sub.BaseURL, "no-models") {
			return nil, 404
		}
		return list("qwen3-coder", "llama-4"), 200
	}
	switch sub.Provider {
	case subscriptions.SourceZAI, subscriptions.SourceZAIAPI:
		return list("glm-5.3", "glm-5.3-flash", "glm-5.2", "glm-4.7"), 200
	case subscriptions.SourceKimi:
		return []connect.ModelMeta{
			{ID: "k3", Name: "K3"}, {ID: "kimi-for-coding", Name: "K2.8 Preview"},
			{ID: "kimi-for-coding-highspeed", Name: "K2.7 Code Highspeed"},
		}, 200
	case subscriptions.SourceKimiAPI:
		return list("kimi-k2.6", "kimi-k3", "kimi-k2.7-code"), 200
	case subscriptions.SourceOpenAI:
		return list("gpt-4o", "gpt-5", "o3", "text-embedding-3-large"), 200
	case subscriptions.SourceOpenRouter:
		return list("anthropic/claude-sonnet-4.5", "openai/gpt-5"), 200
	case subscriptions.SourceAnthropic:
		return list("claude-opus-4-8", "claude-sonnet-4-6", "claude-haiku-4-5"), 200
	case subscriptions.SourceOllama:
		if sub.Plan == "local" || strings.Contains(sub.BaseURL, "localhost") {
			return list("llama3.2:latest", "qwen2.5:7b"), 200
		}
		return list("glm-5.2", "kimi-k3"), 200
	case subscriptions.SourceClaudeCLI:
		return []connect.ModelMeta{{ID: "", Name: "Default (recommended)", Default: true}, {ID: "sonnet", Name: "Sonnet"}}, 200
	case subscriptions.SourceCodexCLI:
		return []connect.ModelMeta{{ID: "gpt-5.6-sol", Default: true}, {ID: "gpt-5.6-luna"}}, 200
	}
	return nil, 0
}
