package llmpick

import (
	"fmt"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

func store(provider string) *subscriptions.Store {
	return &subscriptions.Store{
		Active: provider,
		Subscriptions: map[string]subscriptions.Subscription{
			provider: {Provider: provider, APIKey: "stub-key", Plan: "coding"},
		},
	}
}

// Главное, ради чего пакет и появился: активная подписка должна выбираться
// одинаково и в TUI, и в фоне. Пока логика жила в TUI, `execai serve` всегда
// брал AICoreClient — задачи из веба шли через ExecAI мимо подписки.
func TestClient_FollowsActiveSubscription(t *testing.T) {
	cfg := &config.Config{APIBase: "https://api.execai.ru", ThinkingBudget: 4096}
	cases := []struct {
		provider string
		want     string
	}{
		{subscriptions.SourceZAI, "*llm.AnthropicClient"},
		{subscriptions.SourceKimi, "*llm.AnthropicClient"},
		{subscriptions.SourceAnthropic, "*llm.AnthropicClient"},
		{subscriptions.SourceZAIAPI, "*llm.GLMClient"},
		{subscriptions.SourceKimiAPI, "*llm.GLMClient"},
		{subscriptions.SourceOpenAI, "*llm.GLMClient"},
	}
	for _, c := range cases {
		t.Run(c.provider, func(t *testing.T) {
			got := typeName(Client(cfg, store(c.provider), "model-x", "prov", "jwt"))
			if got != c.want {
				t.Errorf("подписка %s → %s, ожидался %s (фон ушёл бы мимо подписки)",
					c.provider, got, c.want)
			}
		})
	}
}

// Нет подписки — работаем через ExecAI. Это и есть путь по умолчанию.
func TestClient_FallsBackToExecAI(t *testing.T) {
	cfg := &config.Config{APIBase: "https://api.execai.ru"}
	for _, s := range []*subscriptions.Store{nil, {}} {
		if got := typeName(Client(cfg, s, "m", "p", "jwt")); got != "*llm.AICoreClient" {
			t.Errorf("без подписки получили %s, ожидался *llm.AICoreClient", got)
		}
	}
}

// Уровень рассуждения — тоже локальная настройка, и в фоне он обязан доезжать.
func TestClient_PassesThinkingBudget(t *testing.T) {
	cfg := &config.Config{APIBase: "https://api.execai.ru", ThinkingBudget: 8192}
	cli, ok := Client(cfg, store(subscriptions.SourceKimi), "k", "p", "jwt").(*llm.AnthropicClient)
	if !ok {
		t.Fatal("ожидался AnthropicClient")
	}
	if cli.ThinkingBudget != 8192 {
		t.Errorf("ThinkingBudget=%d, ожидалось 8192 — /effort в фоне игнорируется", cli.ThinkingBudget)
	}
}

func TestIsOllamaCloud(t *testing.T) {
	cases := []struct {
		base, plan string
		want       bool
	}{
		{"https://ollama.com", "", true},
		{"", "cloud", true},
		{"http://localhost:11434", "", false},
		{"http://192.168.1.10:11434", "local", false},
	}
	for _, c := range cases {
		if got := IsOllamaCloud(c.base, c.plan); got != c.want {
			t.Errorf("IsOllamaCloud(%q,%q)=%v, ожидалось %v — облако и локальный "+
				"сервер говорят по разным протоколам", c.base, c.plan, got, c.want)
		}
	}
}

func typeName(v any) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T", v)
}
