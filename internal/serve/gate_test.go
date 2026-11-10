package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// fakeAPI is billing's my_plan plus the agents-vbai result endpoint.
type fakeAPI struct {
	srv       *httptest.Server
	planCalls atomic.Int32
	mu        sync.Mutex
	results   []map[string]string
}

func newFakeAPI(t *testing.T, planBody string) *fakeAPI {
	f := &fakeAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/billing-vbai/my_plan":
			f.planCalls.Add(1)
			_, _ = w.Write([]byte(planBody))
		case strings.HasPrefix(r.URL.Path, "/agents-vbai/tasks/") && strings.HasSuffix(r.URL.Path, "/result"):
			var body map[string]string
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			f.mu.Lock()
			f.results = append(f.results, body)
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func isolateConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
}

// No plan, ExecAI source: the task gets the gate's text as its result and
// the model is never reached.
func TestGateTask_RefusedTaskGetsResult(t *testing.T) {
	isolateConfig(t)
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	f := newFakeAPI(t, `{"user_id":"u","subscription":null,"is_accessible":false}`)
	cfg := &config.Config{APIBase: f.srv.URL}
	gate := plan.NewChecker(f.srv.URL, "", func() string { return "tok" })

	if gateTask(context.Background(), cfg, "tok", Task{ID: "task-1"}, gate) {
		t.Fatal("task without a plan let through")
	}
	if len(f.results) != 1 {
		t.Fatalf("results posted: %d, want 1", len(f.results))
	}
	r := f.results[0]
	if r["chunk_type"] != "error" || !strings.Contains(r["data"], plan.SubscribeURL) ||
		!strings.Contains(r["data"], "Модели ExecAI доступны по подписке") ||
		!strings.Contains(r["data"], "/source ollama") {
		t.Errorf("result %+v", r)
	}
}

func TestGateTask_ActivePlanRuns(t *testing.T) {
	isolateConfig(t)
	f := newFakeAPI(t, `{"tariff_code":"week_start","state":"active",
		"valid_until":"2099-01-01T00:00:00Z","is_accessible":true}`)
	cfg := &config.Config{APIBase: f.srv.URL}
	gate := plan.NewChecker(f.srv.URL, "", func() string { return "tok" })
	if !gateTask(context.Background(), cfg, "tok", Task{ID: "task-2"}, gate) {
		t.Fatal("active plan refused")
	}
	if len(f.results) != 0 {
		t.Errorf("a result was posted for an allowed task: %+v", f.results)
	}
}

// Own sources run without a plan: the gate only guards the ExecAI models.
func TestGateTask_OwnSourcesRunWithoutPlan(t *testing.T) {
	for _, sub := range []subscriptions.Subscription{
		{Provider: "zai", APIKey: "k"},
		{Provider: "openai", APIKey: "k"},
		{Provider: "claude-cli"},
		{Provider: "corp", API: "anthropic", BaseURL: "https://llm.corp.local"},
		{Provider: "ollama", Plan: "local", BaseURL: "http://localhost:11434"},
		{Provider: "ollama", Plan: "cloud", APIKey: "k"},
	} {
		isolateConfig(t)
		st := &subscriptions.Store{Active: sub.Provider,
			Subscriptions: map[string]subscriptions.Subscription{sub.Provider: sub}}
		if err := st.Save(); err != nil {
			t.Fatal(err)
		}
		f := newFakeAPI(t, `{"user_id":"u","subscription":null,"is_accessible":false}`)
		cfg := &config.Config{APIBase: f.srv.URL}
		gate := plan.NewChecker(f.srv.URL, "", func() string { return "tok" })
		if !gateTask(context.Background(), cfg, "tok", Task{ID: "task-own"}, gate) {
			t.Errorf("%s without a plan refused", sub.Provider)
		}
		if f.planCalls.Load() != 0 || len(f.results) != 0 {
			t.Errorf("%s: billing asked %d times, results %v", sub.Provider, f.planCalls.Load(), f.results)
		}
	}
}

// Tasks from the site come with the Work and Max plans: on another plan the
// task is answered with the refusal, whatever model would run it.
func TestBoardTask_NeedsWorkOrMax(t *testing.T) {
	isolateConfig(t)
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	start := newFakeAPI(t, `{"tariff_code":"week_start","state":"active",
		"valid_until":"2099-01-01T00:00:00Z","is_accessible":true}`)
	cfg := &config.Config{APIBase: start.srv.URL}
	gate := plan.NewChecker(start.srv.URL, "", func() string { return "tok" })
	if boardTask(context.Background(), cfg, "tok", Task{ID: "t-start"}, gate) {
		t.Fatal("week_start let a task from the site through")
	}
	if len(start.results) != 1 || start.results[0]["chunk_type"] != "error" ||
		!strings.Contains(start.results[0]["data"], "«Работа» и «Макс»") ||
		!strings.Contains(start.results[0]["data"], plan.SubscribeURL) {
		t.Errorf("result %+v", start.results)
	}

	work := newFakeAPI(t, `{"tariff_code":"week_work","state":"active",
		"valid_until":"2099-01-01T00:00:00Z","is_accessible":true}`)
	cfg = &config.Config{APIBase: work.srv.URL}
	gate = plan.NewChecker(work.srv.URL, "", func() string { return "tok" })
	if !boardTask(context.Background(), cfg, "tok", Task{ID: "t-work"}, gate) {
		t.Fatal("week_work refused")
	}
	if len(work.results) != 0 {
		t.Errorf("a result was posted for an allowed task: %+v", work.results)
	}
}
