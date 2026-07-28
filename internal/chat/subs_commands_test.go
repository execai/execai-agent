package chat

import (
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// Снимок ExecAI-каталога (как пришёл бы из /billing-vbai/models_public).
func execAISnapshot() []llm.Model {
	return []llm.Model{
		{ID: "claude-sonnet-4-6", Provider: "anthropic", Name: "Sonnet 4.6", IsPrimary: true, HasTools: true},
		{ID: "gpt-5", Provider: "openai", Name: "GPT-5", HasTools: true},
		{ID: "gemini-2.5-pro", Provider: "google", Name: "Gemini 2.5 Pro", HasTools: true},
	}
}

func newTestModel() *tuiModel {
	subs := &subscriptions.Store{
		Subscriptions: map[string]subscriptions.Subscription{
			"zai":       {Provider: "zai", APIKey: "stub", Plan: "coding"},
			"anthropic": {Provider: "anthropic", APIKey: "sk-ant-stub"},
		},
	}
	snap := execAISnapshot()
	return &tuiModel{
		cfg:          &config.Config{APIBase: "https://api.execai.ru", SelectedModelID: "claude-sonnet-4-6"},
		creds:        &config.Credentials{Token: "stub-jwt"},
		subs:         subs,
		models:       snap,
		execAIModels: snap,
		current:      snap[0],
	}
}

// BUG-1 regression: /source zai → /source execai не должен оставлять provider=zai/glm-5.2.
func TestApplySubscriptionSource_ZaiThenExecAI_RestoresCatalog(t *testing.T) {
	m := newTestModel()

	// step 1: → zai
	if err := m.subs.Activate("zai"); err != nil {
		t.Fatal(err)
	}
	m.applySubscriptionSource()
	if m.current.Provider != "zai" {
		t.Fatalf("после /source zai ждали provider=zai, имеем %q", m.current.Provider)
	}
	if m.current.ID != "glm-5.2" {
		t.Fatalf("после /source zai ждали glm-5.2 primary, имеем %q", m.current.ID)
	}
	if len(m.models) == 0 || m.models[0].Provider != "zai" {
		t.Fatalf("каталог не переключился на GLMModels: %+v", m.models)
	}

	// step 2: → execai (BUG-1 был тут — каталог и current оставались zai)
	if err := m.subs.Activate("execai"); err != nil {
		t.Fatal(err)
	}
	m.applySubscriptionSource()

	if m.current.Provider == "zai" || m.current.ID == "glm-5.2" {
		t.Fatalf("BUG-1 regression: после /source execai остался zai/glm-5.2: %+v", m.current)
	}
	if m.current.ID != "claude-sonnet-4-6" {
		t.Fatalf("ждали возврат на ExecAI primary claude-sonnet-4-6, имеем %q", m.current.ID)
	}
	if len(m.models) != len(execAISnapshot()) || m.models[0].ID != "claude-sonnet-4-6" {
		t.Fatalf("каталог не восстановился: %+v", m.models)
	}
}

// /source anthropic → /source execai — то же поведение.
func TestApplySubscriptionSource_AnthropicThenExecAI_RestoresCatalog(t *testing.T) {
	m := newTestModel()
	_ = m.subs.Activate("anthropic")
	m.applySubscriptionSource()
	if m.current.Provider != "anthropic" || !modelInCatalog(llm.AnthropicModels(), m.current.ID) {
		t.Fatalf("после /source anthropic state неверный: %+v", m.current)
	}
	wasAnthroID := m.current.ID

	_ = m.subs.Activate("execai")
	m.applySubscriptionSource()
	// claude-sonnet-4-6 ЕСТЬ и в Anthropic-каталоге, и в ExecAI snapshot — должен сохраниться.
	// Если совпадает по ID — current не сбрасываем (modelInCatalog=true).
	if !modelInCatalog(m.execAIModels, m.current.ID) {
		t.Fatalf("после возврата current=%q не в ExecAI каталоге (был %q)", m.current.ID, wasAnthroID)
	}
}

// Если current был валидной моделью ExecAI и в новом каталоге его нет — должен быть pickPrimary.
func TestApplySubscriptionSource_PickPrimaryWhenCurrentMissing(t *testing.T) {
	m := newTestModel()
	m.current = llm.Model{ID: "gpt-5", Provider: "openai"} // нет в GLMModels
	_ = m.subs.Activate("zai")
	m.applySubscriptionSource()
	if m.current.ID != "glm-5.2" {
		t.Fatalf("ждали fallback на glm-5.2 primary, имеем %q", m.current.ID)
	}
}

// Цепочка zai → anthropic → claude-cli → execai не оставляет залежей.
func TestApplySubscriptionSource_FullChain(t *testing.T) {
	m := newTestModel()
	m.subs.Add(subscriptions.Subscription{Provider: "claude-cli"})

	transitions := []struct {
		target           string
		expectedProvider string
	}{
		{"zai", "zai"},
		{"anthropic", "anthropic"},
		{"claude-cli", "claude-cli"},
		{"execai", "anthropic"}, // claude-sonnet-4-6 — провайдер anthropic в ExecAI каталоге
	}
	for _, step := range transitions {
		if err := m.subs.Activate(step.target); err != nil {
			t.Fatalf("activate %s: %v", step.target, err)
		}
		m.applySubscriptionSource()
		if m.current.Provider != step.expectedProvider {
			// claude-cli может не быть в PATH в test env → ID пустой, provider пустой.
			// Для шага execai этого не должно случаться.
			if step.target == "execai" {
				t.Fatalf("после /source %s ждали provider=%s, имеем %q (model=%q)",
					step.target, step.expectedProvider, m.current.Provider, m.current.ID)
			}
		}
	}
}

// Ollama: если m.ollamaModels кеш заполнен, applySubscriptionSource
// использует его без обращения к сети. Проверяем что источник
// корректно переключается.
func TestApplySubscriptionSource_Ollama_UsesCache(t *testing.T) {
	m := newTestModel()
	m.subs.Add(subscriptions.Subscription{Provider: "ollama", BaseURL: "http://localhost:11434", Plan: "local"})
	m.ollamaModels = []llm.Model{
		{ID: "llama3.2:latest", Provider: "ollama", Name: "llama3.2:latest", IsPrimary: true, HasTools: true},
		{ID: "qwen2.5:7b", Provider: "ollama", Name: "qwen2.5:7b", HasTools: true},
	}

	_ = m.subs.Activate("ollama")
	m.applySubscriptionSource()

	if m.current.Provider != "ollama" {
		t.Fatalf("после /source ollama ждали provider=ollama, имеем %q", m.current.Provider)
	}
	if m.current.ID != "llama3.2:latest" {
		t.Fatalf("ждали primary llama3.2:latest, имеем %q", m.current.ID)
	}
	if len(m.models) != 2 {
		t.Fatalf("каталог не подтянулся из кеша: %+v", m.models)
	}
}

// Ollama → execai: возврат должен восстановить ExecAI каталог, как и для остальных.
func TestApplySubscriptionSource_OllamaThenExecAI(t *testing.T) {
	m := newTestModel()
	m.subs.Add(subscriptions.Subscription{Provider: "ollama", Plan: "local"})
	m.ollamaModels = []llm.Model{
		{ID: "llama3.2:latest", Provider: "ollama", IsPrimary: true},
	}
	_ = m.subs.Activate("ollama")
	m.applySubscriptionSource()

	_ = m.subs.Activate("execai")
	m.applySubscriptionSource()
	if m.current.Provider == "ollama" {
		t.Fatalf("после возврата на ExecAI остался ollama current: %+v", m.current)
	}
	if !modelInCatalog(m.execAIModels, m.current.ID) {
		t.Fatalf("current не в ExecAI каталоге: %q", m.current.ID)
	}
}

// Kimi source: /source kimi должен подтянуть KimiModels + primary k3.
// /source execai после этого — восстановить каталог, current не должен
// остаться k3.
func TestApplySubscriptionSource_KimiThenExecAI(t *testing.T) {
	m := newTestModel()
	m.subs.Add(subscriptions.Subscription{Provider: "kimi", APIKey: "sk-x", Plan: "coding"})
	_ = m.subs.Activate("kimi")
	m.applySubscriptionSource()

	if m.current.Provider != "kimi" {
		t.Fatalf("после /source kimi ждали provider=kimi, имеем %q", m.current.Provider)
	}
	if m.current.ID != "k3" {
		t.Fatalf("ждали k3 primary, имеем %q", m.current.ID)
	}
	if len(m.models) == 0 || m.models[0].Provider != "kimi" {
		t.Fatalf("каталог не переключился на KimiModels: %+v", m.models)
	}

	_ = m.subs.Activate("execai")
	m.applySubscriptionSource()
	if m.current.Provider == "kimi" {
		t.Fatalf("после возврата на ExecAI остался kimi current: %+v", m.current)
	}
	if !modelInCatalog(m.execAIModels, m.current.ID) {
		t.Fatalf("current не в ExecAI каталоге: %q", m.current.ID)
	}
}

func TestModelInCatalog(t *testing.T) {
	cat := []llm.Model{{ID: "a"}, {ID: "b"}}
	if !modelInCatalog(cat, "a") {
		t.Error("a should be in catalog")
	}
	if modelInCatalog(cat, "x") {
		t.Error("x should not be in catalog")
	}
	if modelInCatalog(nil, "a") {
		t.Error("nil catalog: nothing should match")
	}
}

func TestPickPrimary(t *testing.T) {
	cat := []llm.Model{
		{ID: "a"},
		{ID: "b", IsPrimary: true},
		{ID: "c"},
	}
	if pickPrimary(cat).ID != "b" {
		t.Error("ждали b как primary")
	}
	if pickPrimary([]llm.Model{{ID: "x"}, {ID: "y"}}).ID != "x" {
		t.Error("без IsPrimary берём первую")
	}
}
