package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
)

// machinePlanModel — a chat signed in as an agent, without a saved plan
// answer (as right after the first sign-in), against an API whose billing
// answers planStatus/planBody.
func machinePlanModel(t *testing.T, planStatus int, planBody, webBase string) (*tuiModel, *atomic.Int32) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	i18n.SetLocale("en")
	t.Cleanup(func() { i18n.SetLocale("en") })
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/billing-vbai/my_plan":
			calls.Add(1)
			w.WriteHeader(planStatus)
			_, _ = w.Write([]byte(planBody))
		case "/agents-vbai/workspaces/bindings":
			_, _ = w.Write([]byte(`{"bindings":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	cr := &config.Credentials{Token: "tok", Email: "alice@example.com", AgentID: "agent-1", WebBase: webBase}
	if err := config.SaveCredentials(cr); err != nil {
		t.Fatal(err)
	}
	m := newTestModel()
	m.cfg.APIBase = srv.URL
	m.creds = cr
	m.plan = plan.NewChecker(srv.URL, "", func() string { return "tok" })
	return m, &calls
}

func runMachine(m *tuiModel, cmd string) machineMsg {
	return m.handleMachineCommand(cmd)().(machineMsg)
}

// Right after signing in the plan is not known yet: /machine asks billing
// before the card is shown, and an active plan is shown as such — not «no
// ExecAI plan» until /machine check.
func TestMachineCard_AsksForThePlanBeforeShowing(t *testing.T) {
	m, calls := machinePlanModel(t, http.StatusOK, planBodyWork, "")
	card := runMachine(m, "/machine").before
	if calls.Load() == 0 || strings.Contains(card, "no ExecAI plan") || !strings.Contains(card, "plan: Работа") {
		t.Errorf("billing asked %d times; card:\n%s", calls.Load(), card)
	}
	if strings.Contains(card, i18n.T("machine.block.plan")) {
		t.Errorf("an active plan reported as missing:\n%s", card)
	}
	// Known now and fresh: the next card does not ask again.
	runMachine(m, "/machine")
	if calls.Load() != 1 {
		t.Errorf("billing asked %d times for two cards", calls.Load())
	}
}

// Billing cannot be reached and nothing is saved: the plan is «being
// checked», not «no plan».
func TestMachineCard_PlanUnknownIsNotNoPlan(t *testing.T) {
	m, _ := machinePlanModel(t, http.StatusBadGateway, "down", "")
	card := runMachine(m, "/machine").before
	if strings.Contains(card, "no ExecAI plan") || strings.Contains(card, i18n.T("machine.block.plan")) {
		t.Errorf("unknown plan shown as missing:\n%s", card)
	}
	if !strings.Contains(card, i18n.T("machine.block.planPending")) ||
		!strings.Contains(card, i18n.Tf("machine.account.planPending", "a***@e***.com")) {
		t.Errorf("card does not say the plan is being checked:\n%s", card)
	}
}

// The link to pick a plan leads to the web of this installation (the one
// the sign-in was confirmed on), as the /m/<id> link does — not to the
// production site.
func TestPhone_PaymentLinkOnThisInstallation(t *testing.T) {
	m, _ := machinePlanModel(t, http.StatusOK, planBodyNone, "https://web.example.org")
	out := runMachine(m, "/phone")
	if !strings.Contains(out.after, "https://web.example.org/m/agent-1") {
		t.Fatalf("phone link:\n%s", out.after)
	}
	if !strings.Contains(out.after, "https://web.example.org/settings/payment?") || strings.Contains(out.after, "chat.execai.ru") {
		t.Errorf("payment link not on this installation:\n%s", out.after)
	}
}
