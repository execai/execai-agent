package chat

import (
	"context"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// Snapshot of the ExecAI catalog (as it would come from /billing-vbai/models_public).
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

// BUG-1 regression: /source zai → /source execai must not leave provider=zai/glm-5.2.
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

	// step 2: → execai (BUG-1 was here — the catalog and current stayed zai)
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

// /source anthropic → /source execai — same behavior.
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
	// claude-sonnet-4-6 EXISTS both in the Anthropic catalog and in the ExecAI snapshot — must be kept.
	// If the ID matches — current is not reset (modelInCatalog=true).
	if !modelInCatalog(m.execAIModels, m.current.ID) {
		t.Fatalf("после возврата current=%q не в ExecAI каталоге (был %q)", m.current.ID, wasAnthroID)
	}
}

// If current was a valid ExecAI model and it is absent from the new catalog — pickPrimary must apply.
func TestApplySubscriptionSource_PickPrimaryWhenCurrentMissing(t *testing.T) {
	m := newTestModel()
	m.current = llm.Model{ID: "gpt-5", Provider: "openai"} // not in GLMModels
	_ = m.subs.Activate("zai")
	m.applySubscriptionSource()
	if m.current.ID != "glm-5.2" {
		t.Fatalf("ждали fallback на glm-5.2 primary, имеем %q", m.current.ID)
	}
}

// The zai → anthropic → claude-cli → execai chain leaves no residue.
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
		{"execai", "anthropic"}, // claude-sonnet-4-6 — provider anthropic in the ExecAI catalog
	}
	for _, step := range transitions {
		if err := m.subs.Activate(step.target); err != nil {
			t.Fatalf("activate %s: %v", step.target, err)
		}
		m.applySubscriptionSource()
		if m.current.Provider != step.expectedProvider {
			// claude-cli may be missing from PATH in the test env → empty ID, empty provider.
			// This must not happen for the execai step.
			if step.target == "execai" {
				t.Fatalf("после /source %s ждали provider=%s, имеем %q (model=%q)",
					step.target, step.expectedProvider, m.current.Provider, m.current.ID)
			}
		}
	}
}

// Ollama: if the m.ollamaModels cache is populated, applySubscriptionSource
// uses it without hitting the network. Verify that the source
// switches correctly.
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

// Ollama → execai: the return must restore the ExecAI catalog, same as for the others.
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

// Kimi source: /source kimi must pull KimiModels + primary k3.
// /source execai afterwards — restore the catalog, current must not
// stay k3.
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

// /provider — полный синоним /source: одно и то же меню и одинаковое
// переключение. Алиас переписывается в /source на входе, поэтому любое
// расхождение в поведении — регрессия.
func TestProviderAlias_EqualsSource(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	a, b := newTestModel(), newTestModel()
	okA, listA := a.handleSubsCommand("/source")
	okB, listB := b.handleSubsCommand("/provider")
	if !okA || !okB {
		t.Fatalf("команда не распознана: /source=%v /provider=%v", okA, okB)
	}
	if listA != listB {
		t.Errorf("списки различаются:\n/source:\n%s\n/provider:\n%s", listA, listB)
	}

	m := newTestModel()
	if handled, _ := m.handleSubsCommand("/provider zai"); !handled {
		t.Fatal("/provider zai не распознан")
	}
	if m.current.Provider != "zai" || m.subs.Active != "zai" {
		t.Fatalf("после /provider zai ждали активный zai, имеем subs=%q model=%+v",
			m.subs.Active, m.current)
	}
}

// fakeLLM — детерминированный клиент для тестов сжатия.
type fakeLLM struct{ reply string }

func (f *fakeLLM) Stream(_ context.Context, _ []llm.AIMessage, _ []map[string]any,
	_ llm.StreamCallbacks) (*llm.StreamResult, error) {
	return &llm.StreamResult{Content: f.reply}, nil
}

// Автосжатие срабатывает строго за порогом, один раз, и не трогает короткую
// историю. Логика порога — единственное место, где решается «сжимать или нет».
func TestMaybeAutoCompact_Threshold(t *testing.T) {
	m := newTestModel()
	m.cli = &fakeLLM{reply: "пересказ"}
	for i := 0; i < 12; i++ {
		m.history = append(m.history, llm.AIMessage{Role: "user", Content: "x"})
	}

	m.ctxPct = 79
	if cmd := m.maybeAutoCompact(); cmd != nil {
		t.Error("79% при пороге 80 — сжатие запускаться не должно")
	}
	m.ctxPct = 81
	cmd := m.maybeAutoCompact()
	if cmd == nil {
		t.Fatal("81% при пороге 80 — сжатие обязано запуститься")
	}
	if !m.compacting {
		t.Error("флаг compacting не взведён — второй запуск не заблокирован")
	}
	if again := m.maybeAutoCompact(); again != nil {
		t.Error("повторный запуск при идущем сжатии")
	}
	if done, ok := cmd().(compactDoneMsg); !ok || done.err != nil {
		t.Errorf("сжатие не отработало: %+v", done)
	}

	// Выключатель уважается.
	m2 := newTestModel()
	m2.cli = &fakeLLM{reply: "s"}
	m2.cfg.AutoCompactPercent = -1
	m2.ctxPct = 99
	for i := 0; i < 12; i++ {
		m2.history = append(m2.history, llm.AIMessage{Role: "user", Content: "x"})
	}
	if cmd := m2.maybeAutoCompact(); cmd != nil {
		t.Error("автосжатие выключено, но запустилось")
	}
}
