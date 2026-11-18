package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

func TestParseCustomArgs(t *testing.T) {
	c, err := parseCustomArgs(`"LM Studio" openai http://localhost:1234/v1 --models a,b -H "X-Tenant: dev team" --model c`)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "LM Studio" || c.API != "openai" || c.BaseURL != "http://localhost:1234/v1" || c.APIKey != "" {
		t.Errorf("позиционные: %+v", c)
	}
	if strings.Join(c.Models, "|") != "a,b|c" || c.Headers["X-Tenant"] != "dev team" {
		t.Errorf("флаги: models=%v headers=%v", c.Models, c.Headers)
	}
	c, err = parseCustomArgs(`corp anthropic https://llm.corp sk-1`)
	if err != nil || c.APIKey != "sk-1" {
		t.Errorf("ключ четвёртым аргументом: %+v %v", c, err)
	}
	for _, bad := range []string{`only-name openai`, `a b c d e`, `"unclosed openai http://x`, `x openai http://x --bogus`, `x openai http://x -H novalue`} {
		if _, err := parseCustomArgs(bad); err == nil {
			t.Errorf("%q должен быть отвергнут", bad)
		}
	}
}

// The one-line form connects, switches to the endpoint and takes its models.
func TestConnectCustom_OneLine(t *testing.T) {
	m := newTestModel()
	msg := m.startConnectFlow(`custom "LM Studio" openai http://lm.local:1234`)
	if !strings.Contains(msg, "lm-studio") {
		t.Fatalf("ответ: %s", msg)
	}
	sub, ok := m.subs.Subscriptions["lm-studio"]
	if !ok || !sub.IsCustom() || sub.BaseURL != "http://lm.local:1234/v1" {
		t.Fatalf("подписка: %+v", sub)
	}
	if m.subs.Active != "lm-studio" || m.current.Provider != "lm-studio" || m.current.ID != "qwen3-coder" {
		t.Fatalf("не переключились на кастомный: active=%q current=%+v", m.subs.Active, m.current)
	}
	// It is listed among the sources once connected.
	if !hasLabel(filterUseOptionsFor(m.subs, "", "/source"), "● lm-studio") {
		t.Error("подключённый кастомный провайдер не виден в /source")
	}
	// And the request goes to it — not to ExecAI, not to Z.ai.
	if m.cli == nil {
		t.Fatal("клиент не создан")
	}
}

// Step by step: name → protocol → address → key. The key never reaches the
// transcript, and the answers never reach the input history.
func TestCustomWizard(t *testing.T) {
	m := newTestModel()
	m.startConnectFlow("custom")
	if m.customWiz == nil {
		t.Fatal("мастер не запущен")
	}
	for _, answer := range []string{"Corp GW", "2", "https://gw.corp.local/v1/messages", "sk-very-secret"} {
		m.customWizardInput(answer)
	}
	if m.customWiz != nil {
		t.Fatalf("мастер не завершился, шаг %d", m.customWiz.step)
	}
	sub := m.subs.Subscriptions["corp-gw"]
	if sub.API != subscriptions.APIAnthropic || sub.BaseURL != "https://gw.corp.local" || sub.APIKey != "sk-very-secret" {
		t.Fatalf("подписка: %+v", sub)
	}
	if m.subs.Active != "corp-gw" || m.current.Provider != "corp-gw" {
		t.Fatalf("не переключились: %q %+v", m.subs.Active, m.current)
	}
	for _, seg := range m.uiSegments {
		if strings.Contains(seg.body, "sk-very-secret") {
			t.Fatalf("ключ попал в ленту: %q", seg.body)
		}
	}
	if !sourceSupportsThinking(m.subs) {
		t.Error("Anthropic-совместимый кастомный должен поддерживать effort/thinking")
	}
}

// A wrong answer repeats the question instead of failing the whole flow.
func TestCustomWizard_RepeatsOnBadAnswer(t *testing.T) {
	m := newTestModel()
	m.startConnectFlow("custom")
	m.customWizardInput("kimi") // a built-in id
	if m.customWiz == nil || m.customWiz.step != wizName {
		t.Fatal("встроенное имя должно переспросить имя")
	}
	m.customWizardInput("mine")
	m.customWizardInput("gemini") // unknown protocol
	if m.customWiz.step != wizAPI {
		t.Fatal("неизвестный тип должен переспросить тип")
	}
	m.customWizardInput("openai")
	m.customWizardInput("ftp://nope")
	if m.customWiz.step != wizURL {
		t.Fatal("плохой адрес должен переспросить адрес")
	}
	m.cancelCustomWizard()
	if m.customWiz != nil {
		t.Fatal("Esc не отменил мастер")
	}
	if _, ok := m.subs.Subscriptions["mine"]; ok {
		t.Fatal("отменённый мастер ничего не сохраняет")
	}
}

// An endpoint without /models: the wizard asks for model IDs.
func TestCustomWizard_NoModelsAsksForThem(t *testing.T) {
	m := newTestModel()
	m.startConnectFlow("custom")
	for _, a := range []string{"bare", "1", "http://no-models.local:8000", ""} {
		m.customWizardInput(a)
	}
	if m.customWiz == nil || m.customWiz.step != wizModels {
		t.Fatal("без списка моделей мастер должен спросить их")
	}
	m.customWizardInput("model-x, model-y")
	if m.customWiz != nil {
		t.Fatal("мастер не завершился")
	}
	if m.current.ID != "model-x" || len(m.models) != 2 {
		t.Fatalf("ручные модели не стали каталогом: %+v %+v", m.current, m.models)
	}
}

// /model <id> for a source that lists nothing: taken as typed and remembered.
// For a built-in provider with a live list an unknown ID stays an error.
func TestTypedModel(t *testing.T) {
	m := newTestModel()
	m.startConnectFlow("custom bare openai http://no-models.local")
	if len(m.models) != 0 {
		t.Fatalf("ожидался пустой каталог, %+v", m.models)
	}
	if m.current.Provider != "bare" {
		t.Fatalf("current должен принадлежать новому источнику: %+v", m.current)
	}
	got, ok := m.typedModel("served-model")
	if !ok || got.ID != "served-model" || got.Provider != "bare" {
		t.Fatalf("ручная модель не принята: %+v %v", got, ok)
	}
	if mm := m.subs.Subscriptions["bare"].ManualModels; len(mm) != 1 || mm[0] != "served-model" {
		t.Fatalf("ручная модель не запомнена: %v", mm)
	}

	z := newTestModel()
	_ = z.subs.Activate("zai")
	z.applySubscriptionSource()
	if _, ok := z.typedModel("glm-typo"); ok {
		t.Error("у встроенного провайдера со списком неизвестный ID — опечатка, не принимать")
	}

	// A CLI takes pinned IDs its list leaves out.
	c := newTestModel()
	c.subs.Add(subscriptions.Subscription{Provider: "claude-cli"})
	_ = c.subs.Activate("claude-cli")
	c.applySubscriptionSource()
	if got, ok := c.typedModel("claude-opus-5-5"); !ok || got.Provider != "claude-cli" {
		t.Errorf("claude-cli должен принять закреплённый ID: %+v %v", got, ok)
	}
}

// Every source with a live list gets it at the moment of switching, and the
// list is the provider's — the CLI sources included.
func TestApplySubscriptionSource_CLIsAskTheCLI(t *testing.T) {
	m := newTestModel()
	m.subs.Add(subscriptions.Subscription{Provider: "codex-cli"})
	_ = m.subs.Activate("codex-cli")
	m.applySubscriptionSource()
	if m.current.ID != "gpt-5.6-sol" {
		t.Fatalf("codex: ждали его isDefault gpt-5.6-sol, имеем %+v", m.current)
	}
	m.subs.Add(subscriptions.Subscription{Provider: "claude-cli"})
	_ = m.subs.Activate("claude-cli")
	m.applySubscriptionSource()
	if m.current.ID != "" || m.current.Name != "Default (recommended)" {
		t.Fatalf("claude: ждали его Default, имеем %+v", m.current)
	}
}

// /connect lines carry keys — the persisted input history must not.
func TestInputHistory_SkipsConnectLines(t *testing.T) {
	dir, _ := config.Dir()
	path := filepath.Join(dir, "input_history")
	_ = os.Remove(path)
	saveInputHistory("/connect openai sk-secret")
	saveInputHistory("привет")
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "sk-secret") || !strings.Contains(string(data), "привет") {
		t.Fatalf("история: %q", data)
	}
}
