package chat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/llmpick"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

const (
	planBodyWork = `{"tariff_code":"week_work","title":"Работа","state":"active",
		"valid_until":"2099-11-05T22:22:28Z","is_accessible":true}`
	planBodyNone = `{"user_id":"u1","subscription":null,"is_accessible":false}`
)

// fakeBilling answers GET /billing-vbai/my_plan with body and counts calls.
func fakeBilling(t *testing.T, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func gateModel(t *testing.T, body string) (*tuiModel, *atomic.Int32) {
	srv, calls := fakeBilling(t, body)
	m := newBurstTestModel()
	m.plan = plan.NewChecker(srv.URL, "", func() string { return "tok" })
	return m, calls
}

func lastHint(m *tuiModel) string {
	for i := len(m.uiSegments) - 1; i >= 0; i-- {
		if m.uiSegments[i].kind == "system_hint" {
			return m.uiSegments[i].body
		}
	}
	return ""
}

func TestPlanCommand_ShowsReport(t *testing.T) {
	i18n.SetLocale("en")
	m, _ := gateModel(t, planBodyWork)
	_, cmd := m.handleCommand("/plan")
	if cmd == nil {
		t.Fatal("/plan returned no command")
	}
	m.Update(cmd())
	out := lastHint(m)
	for _, want := range []string{"ExecAI plan", "Работа (week_work)", "every source",
		"Sync and web console: available"} {
		if !strings.Contains(out, want) {
			t.Errorf("/plan lacks %q:\n%s", want, out)
		}
	}
}

// The gate as a turn runs it: only the ExecAI source needs a plan. Every own
// source passes without a plan and without a login, and billing is not asked.
func TestPlanGate_Applies(t *testing.T) {
	i18n.SetLocale("en")
	oldCLI := llmpick.CLIAvailable
	llmpick.CLIAvailable = func(string) bool { return true }
	defer func() { llmpick.CLIAvailable = oldCLI }()

	m, _ := gateModel(t, planBodyNone)
	v := m.planGate()(context.Background())
	if v.Allowed || !strings.Contains(v.Message(), plan.SubscribeURL) {
		t.Errorf("execai source without a plan: %+v %q", v, v.Message())
	}

	own := []subscriptions.Subscription{
		{Provider: "zai", APIKey: "k"},
		{Provider: "kimi", APIKey: "k"},
		{Provider: "claude-cli"},
		{Provider: "codex-cli"},
		{Provider: "lm-studio", API: "openai", BaseURL: "http://localhost:1234/v1", Name: "LM Studio"},
		{Provider: "ollama", Plan: "local", BaseURL: "http://localhost:11434"},
		{Provider: "ollama", Plan: "cloud", APIKey: "k"},
	}
	for _, sub := range own {
		for _, token := range []string{"tok", ""} {
			srv, calls := fakeBilling(t, planBodyNone)
			tok := token
			m := newBurstTestModel()
			m.plan = plan.NewChecker(srv.URL, "", func() string { return tok })
			m.subs = &subscriptions.Store{Active: sub.Provider,
				Subscriptions: map[string]subscriptions.Subscription{sub.Provider: sub}}
			if v := m.planGate()(context.Background()); !v.Allowed {
				t.Errorf("%s (token %q) without a plan refused: %+v", sub.Provider, token, v)
			}
			if calls.Load() != 0 {
				t.Errorf("%s: billing asked for an own source", sub.Provider)
			}
		}
	}

	m, _ = gateModel(t, planBodyWork)
	if v := m.planGate()(context.Background()); !v.Allowed {
		t.Errorf("execai with an active plan refused: %+v", v)
	}
}

func TestPlanGateMsg_ReplacesTurn(t *testing.T) {
	m := newBurstTestModel()
	m.streaming = true
	m.loopActive = true
	m.Update(planGateMsg{text: "need a plan"})
	if m.streaming || m.loopActive {
		t.Errorf("after a refusal streaming=%v loop=%v", m.streaming, m.loopActive)
	}
	if lastHint(m) != "need a plan" {
		t.Errorf("refusal not shown: %q", lastHint(m))
	}
	if len(m.history) != 0 {
		t.Error("a refused turn must not touch the history")
	}
}

// The refusal is announced once after a refresh, not on every daily tick.
func TestAnnouncePlan_Once(t *testing.T) {
	i18n.SetLocale("en")
	m, _ := gateModel(t, planBodyNone)
	for i := 0; i < 2; i++ {
		m.Update(m.planRefreshCmd(false)())
	}
	n := 0
	for _, s := range m.uiSegments {
		if strings.Contains(s.body, plan.SubscribeURL) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("refusal announced %d times, want 1", n)
	}
}

func TestPlanAfterError_402(t *testing.T) {
	m, _ := gateModel(t, planBodyNone)
	if m.planAfterError(fmt.Errorf("turn: %w", llm.ErrPaymentRequired)) == nil {
		t.Error("402 did not re-ask the plan")
	}
	if m.planAfterError(errors.New("ответ 500")) != nil {
		t.Error("500 re-asked the plan")
	}
}

// execai run talks to the ExecAI models: signed out, it says how to sign in
// in the gate's words (not the old "agent-vbai login") and calls nothing.
func TestOnce_SignedOutSaysHowToSignIn(t *testing.T) {
	i18n.SetLocale("en")
	useStore(t, &subscriptions.Store{Active: "execai", Subscriptions: map[string]subscriptions.Subscription{}})
	cfg := &config.Config{APIBase: "http://127.0.0.1:1", Locale: "en"}
	err := Once(context.Background(), cfg, "hi")
	if err == nil || !strings.Contains(err.Error(), "/login") ||
		!strings.Contains(err.Error(), "ExecAI models") {
		t.Errorf("signed-out run: %v", err)
	}
}

// useStore makes st the active subscriptions.json of the (isolated) config
// directory and removes credentials: these runs are signed out.
func useStore(t *testing.T, st *subscriptions.Store) {
	t.Helper()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	_ = config.DeleteCredentials()
	t.Cleanup(func() {
		_ = (&subscriptions.Store{Subscriptions: map[string]subscriptions.Subscription{}}).Save()
	})
}

// Background refreshes do not ask billing while an own source is active:
// my_plan may renew (charge) an expired period.
func TestPlanRefresh_SkippedForOwnSource(t *testing.T) {
	m, calls := gateModel(t, planBodyWork)
	m.subs = &subscriptions.Store{Active: "zai", Subscriptions: map[string]subscriptions.Subscription{
		"zai": {Provider: "zai", APIKey: "k"}}}
	m.Update(m.planRefreshCmd(true)())
	if calls.Load() != 0 {
		t.Errorf("billing asked %d times on an own source", calls.Load())
	}
	m.subs = &subscriptions.Store{Subscriptions: map[string]subscriptions.Subscription{}}
	m.Update(m.planRefreshCmd(true)())
	if calls.Load() != 1 {
		t.Errorf("billing asked %d times on the ExecAI source, want 1", calls.Load())
	}
}

// Switching to the ExecAI models is when the plan is first needed: the
// switch asks billing (if nothing fresh is saved).
func TestSwitchToExecAI_EnsuresPlan(t *testing.T) {
	i18n.SetLocale("en")
	srv, calls := fakeBilling(t, planBodyNone)
	m := newTestModel()
	m.plan = plan.NewChecker(srv.URL, "", func() string { return "tok" })
	m.subs.Active = "zai"
	m.applySubscriptionSource()
	_, cmd := m.handleCommand("/source execai")
	if cmd == nil {
		t.Fatal("switch to execai did not ask for the plan")
	}
	m.Update(cmd())
	if calls.Load() != 1 {
		t.Errorf("billing asked %d times after the switch, want 1", calls.Load())
	}
	if !strings.Contains(lastHint(m), plan.SubscribeURL) {
		t.Errorf("no plan after the switch not announced: %q", lastHint(m))
	}
	_, cmd = m.handleCommand("/source zai")
	if cmd != nil {
		m.Update(cmd())
	}
	if calls.Load() != 1 {
		t.Errorf("switch to an own source asked billing: %d calls", calls.Load())
	}
}
