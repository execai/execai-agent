package machine

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

var now = time.Date(2026, 11, 10, 6, 0, 0, 0, time.UTC)

func activePlan(code string) *plan.Plan {
	return &plan.Plan{TariffCode: code, Title: "Work", State: plan.StateActive, Accessible: true,
		ValidUntil: now.Add(72 * time.Hour), CheckedAt: now.Add(-time.Hour)}
}

// fresh — a machine that has just been installed: nothing is done.
func fresh() Inputs {
	return Inputs{
		APIBase: "https://api.execai.ru", Hostname: "box", OS: "linux", Version: "7.9",
		Subs:     &subscriptions.Store{Subscriptions: map[string]subscriptions.Subscription{}},
		Bindings: -1, Now: now,
	}
}

// full — everything is set up and the machine was called from the phone.
func full() Inputs {
	in := fresh()
	in.Creds = &config.Credentials{Token: "tok", Email: "alice@example.com", AgentID: "agent-1", Alias: "laptop"}
	in.Plan = activePlan(plan.TariffWeekWork)
	in.Subs = &subscriptions.Store{Active: "zai", Subscriptions: map[string]subscriptions.Subscription{
		"zai": {Provider: "zai", APIKey: "k", ModelsFetchedAt: now.Add(-2 * time.Hour)},
	}}
	in.Sync = &agentsync.State{Enabled: true, LastPull: now.Add(-time.Minute), LastPush: now.Add(-time.Minute),
		Blobs: map[string]agentsync.BlobState{
			"config/config":       {Version: 3},
			"memory/a":            {Version: 1},
			"memory/b":            {Version: 1},
			"memory/gone":         {Version: 2, Deleted: true},
			"project-memory/ws-1": {Version: 1},
		},
		Projects: map[string]string{"ws-1": "/src/app"}}
	in.Listener = &inbox.Owner{PID: 42, Kind: inbox.KindServe}
	in.Bindings = 1
	in.Marks = Marks{WebCallAt: now.Add(-time.Hour)}
	return in
}

func stepsDone(s Snapshot) map[string]bool {
	out := map[string]bool{}
	for _, st := range s.Tutorial.Steps {
		out[st.ID] = st.Done
	}
	return out
}

func TestBuild_FreshMachine(t *testing.T) {
	s := Build(fresh())
	m := s.Machine
	if m.LoggedIn || m.ID != "" || m.EmailMasked != "" || m.Plan != "" {
		t.Errorf("account of a fresh machine: %+v", m)
	}
	if m.Name != "box" || m.OS != "linux" || m.Version != "7.9" {
		t.Errorf("identity: %+v", m)
	}
	if len(m.Sources) != 1 || m.Sources[0].ID != "execai" || m.Sources[0].Alive || m.Sources[0].CheckedAt != "" {
		t.Errorf("sources: %+v", m.Sources)
	}
	if m.Sync.Enabled || m.Sync.Config || m.Sync.PersonalMemoryFiles != 0 || m.Sync.Projects != 0 || m.Sync.LastPushAt != "" {
		t.Errorf("sync: %+v", m.Sync)
	}
	if m.Sync.Mode != ModeSelected {
		t.Errorf("mode %q", m.Sync.Mode)
	}
	if m.Presence.ListenerRunning || m.Presence.ReachableFromWeb {
		t.Errorf("presence: %+v", m.Presence)
	}
	for id, done := range stepsDone(s) {
		if done {
			t.Errorf("step %s done on a fresh machine", id)
		}
	}
	if s.PhoneURL != "" {
		t.Errorf("phone url without an agent: %q", s.PhoneURL)
	}
}

func TestBuild_FullMachine(t *testing.T) {
	s := Build(full())
	m := s.Machine
	// The plan goes as the tariff code: the plugin and the web translate it.
	if !m.LoggedIn || m.ID != "agent-1" || m.Name != "laptop" || m.Plan != "week_work" || m.PlanTitle != "Work" {
		t.Errorf("account: %+v", m)
	}
	if m.EmailMasked == "" || strings.Contains(m.EmailMasked, "alice") || strings.Contains(m.EmailMasked, "example") {
		t.Errorf("email not masked: %q", m.EmailMasked)
	}
	if len(m.Sources) != 2 || !m.Sources[0].Alive || !m.Sources[1].Alive || m.Sources[1].ID != "zai" ||
		m.Sources[1].Title != "Z.ai Coding Plan" || !m.Sources[1].Active {
		t.Errorf("sources: %+v", m.Sources)
	}
	if m.Sources[1].CheckedAt != now.Add(-2*time.Hour).Format(time.RFC3339) {
		t.Errorf("checked_at %q", m.Sources[1].CheckedAt)
	}
	sy := m.Sync
	if !sy.Enabled || !sy.Config || sy.PersonalMemoryFiles != 2 || sy.Projects != 1 ||
		sy.LastPushAt != now.Add(-time.Minute).Format(time.RFC3339) {
		t.Errorf("sync: %+v", sy)
	}
	if !m.Presence.ListenerRunning || !m.Presence.ReachableFromWeb || len(m.Presence.Blockers) != 0 {
		t.Errorf("presence: %+v", m.Presence)
	}
	for id, done := range stepsDone(s) {
		if !done {
			t.Errorf("step %s not done on a set-up machine", id)
		}
	}
	if s.PhoneURL != "https://execai.ru/m/agent-1" {
		t.Errorf("phone url %q", s.PhoneURL)
	}
}

// The tutorial: the five steps in a fixed order and what marks each done.
func TestTutorial_Steps(t *testing.T) {
	ids := func(s Snapshot) []string {
		var out []string
		for _, st := range s.Tutorial.Steps {
			out = append(out, st.ID)
		}
		return out
	}
	if got := ids(Build(fresh())); !reflect.DeepEqual(got, []string{"login", "models", "sync", "project", "phone"}) {
		t.Fatalf("steps %v", got)
	}

	cases := []struct {
		name string
		edit func(*Inputs)
		step string
		want bool
	}{
		{"login: a token", func(in *Inputs) { in.Creds = &config.Credentials{Token: "t"} }, StepLogin, true},
		{"login: a credentials file without a token", func(in *Inputs) { in.Creds = &config.Credentials{Email: "a@b"} }, StepLogin, false},

		{"models: ExecAI with an active plan", func(in *Inputs) {
			in.Creds = &config.Credentials{Token: "t"}
			in.Plan = activePlan(plan.TariffWeekStart)
		}, StepModels, true},
		{"models: ExecAI signed in, no plan", func(in *Inputs) {
			in.Creds = &config.Credentials{Token: "t"}
			in.Plan = &plan.Plan{TariffCode: plan.TariffFlexible, CheckedAt: now}
		}, StepModels, false},
		{"models: an own key that answered, no login", func(in *Inputs) {
			in.Subs.Subscriptions["kimi"] = subscriptions.Subscription{Provider: "kimi", APIKey: "k", ModelsFetchedAt: now.Add(-48 * time.Hour)}
		}, StepModels, true},
		{"models: an own key never checked", func(in *Inputs) {
			in.Subs.Subscriptions["kimi"] = subscriptions.Subscription{Provider: "kimi", APIKey: "k"}
		}, StepModels, false},
		{"models: the latest check failed", func(in *Inputs) {
			in.Subs.Subscriptions["kimi"] = subscriptions.Subscription{Provider: "kimi", APIKey: "k", ModelsFetchedAt: now.Add(-48 * time.Hour)}
			in.Checks = Checks{"kimi": {OK: false, Status: 401, At: now.Add(-time.Hour)}}
		}, StepModels, false},
		{"models: a check older than the last answer does not count", func(in *Inputs) {
			in.Subs.Subscriptions["kimi"] = subscriptions.Subscription{Provider: "kimi", APIKey: "k", ModelsFetchedAt: now.Add(-time.Hour)}
			in.Checks = Checks{"kimi": {OK: false, Status: 0, At: now.Add(-48 * time.Hour)}}
		}, StepModels, true},
		{"models: a live check answered", func(in *Inputs) {
			in.Subs.Subscriptions["ollama"] = subscriptions.Subscription{Provider: "ollama"}
			in.Checks = Checks{"ollama": {OK: true, Status: 200, At: now}}
		}, StepModels, true},

		{"sync: on, exchanged", func(in *Inputs) {
			in.Sync = &agentsync.State{Enabled: true, LastPull: now}
		}, StepSync, true},
		{"sync: on, never exchanged", func(in *Inputs) {
			in.Sync = &agentsync.State{Enabled: true}
		}, StepSync, false},
		{"sync: paused by the plan", func(in *Inputs) {
			in.Sync = &agentsync.State{Enabled: true, LastPull: now, Paused: agentsync.PausedPlan}
		}, StepSync, false},
		{"sync: off", func(in *Inputs) {
			in.Sync = &agentsync.State{LastPull: now}
		}, StepSync, false},

		{"project: the server lists a binding", func(in *Inputs) { in.Bindings = 2 }, StepProject, true},
		{"project: the server lists none", func(in *Inputs) {
			in.Bindings = 0
			in.Sync = &agentsync.State{Projects: map[string]string{"ws": "/old"}}
		}, StepProject, false},
		{"project: offline, sync knows a bound project", func(in *Inputs) {
			in.Sync = &agentsync.State{Projects: map[string]string{"ws": "/src"}}
		}, StepProject, true},

		{"phone: called from the web", func(in *Inputs) { in.Marks.WebCallAt = now }, StepPhone, true},
		{"phone: /phone was shown", func(in *Inputs) { in.Marks.PhoneShownAt = now }, StepPhone, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := fresh()
			c.edit(&in)
			if got := stepsDone(Build(in))[c.step]; got != c.want {
				t.Errorf("%s done = %v, want %v", c.step, got, c.want)
			}
		})
	}
}

// Reachable from the web and the phone: signed in as an agent, a plan with
// the web console, and something on this machine accepting tasks.
func TestPresence_Blockers(t *testing.T) {
	in := full()
	in.Listener = nil
	p := Build(in).Machine.Presence
	if p.ListenerRunning || p.ReachableFromWeb || !reflect.DeepEqual(p.Blockers, []string{BlockListener}) {
		t.Errorf("no listener: %+v", p)
	}

	in = full()
	in.Plan = activePlan(plan.TariffWeekStart)
	p = Build(in).Machine.Presence
	if p.ReachableFromWeb || !reflect.DeepEqual(p.Blockers, []string{BlockPlan}) {
		t.Errorf("start plan: %+v", p)
	}

	in = fresh()
	p = Build(in).Machine.Presence
	if !reflect.DeepEqual(p.Blockers, []string{BlockLogin, BlockListener}) {
		t.Errorf("signed out: %+v", p)
	}

	// A browser token (no agent id) has no inbox.
	in = full()
	in.Creds.AgentID = ""
	if p = Build(in).Machine.Presence; p.ReachableFromWeb || p.Blockers[0] != BlockLogin {
		t.Errorf("browser token: %+v", p)
	}
}

// The wire contract the editor plugin and the web implement: exactly these
// keys, always present.
func TestSnapshot_JSONContract(t *testing.T) {
	for _, in := range []Inputs{fresh(), full()} {
		b, err := json.Marshal(Build(in))
		if err != nil {
			t.Fatal(err)
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(b, &raw); err != nil {
			t.Fatal(err)
		}
		assertKeys(t, "top", raw, "machine", "tutorial", "phone_url")
		var m map[string]json.RawMessage
		_ = json.Unmarshal(raw["machine"], &m)
		assertKeys(t, "machine", m, "id", "name", "os", "version", "logged_in", "email_masked", "plan", "sources", "sync", "presence")
		var sy, pr map[string]json.RawMessage
		_ = json.Unmarshal(m["sync"], &sy)
		assertKeys(t, "sync", sy, "enabled", "last_push_at", "config", "personal_memory_files", "projects", "mode")
		_ = json.Unmarshal(m["presence"], &pr)
		assertKeys(t, "presence", pr, "listener_running", "reachable_from_web")
		var srcs []map[string]json.RawMessage
		_ = json.Unmarshal(m["sources"], &srcs)
		for _, s := range srcs {
			assertKeys(t, "source", s, "id", "title", "alive", "checked_at")
		}
		var tut struct {
			Steps []map[string]json.RawMessage `json:"steps"`
		}
		_ = json.Unmarshal(raw["tutorial"], &tut)
		if len(tut.Steps) != 5 {
			t.Fatalf("steps %d", len(tut.Steps))
		}
		for _, s := range tut.Steps {
			assertKeys(t, "step", s, "id", "done")
		}
	}
}

func assertKeys(t *testing.T, what string, m map[string]json.RawMessage, want ...string) {
	t.Helper()
	var got []string
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s keys %v, want %v", what, got, want)
	}
}

func TestMaskEmail(t *testing.T) {
	cases := map[string]string{
		"alice@example.com": "a***@e***.com",
		"Bo@mail.co.uk":     "B***@m***.co.uk",
		"vk_123@vk.oauth":   "v***@v***.oauth",
		"x@localhost":       "x***@l***",
		"not-an-email":      "n***",
		"":                  "",
		"ёлка@почта.рф":     "ё***@п***.рф",
	}
	for in, want := range cases {
		if got := MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWebBase(t *testing.T) {
	cases := []struct{ api, saved, want string }{
		{"https://api.execai.ru", "", "https://execai.ru"},
		{"https://api.execai.ru/", "", "https://execai.ru"},
		{"https://chat.example.org/api", "", "https://chat.example.org"},
		{"https://chat.example.org/api/", "", "https://chat.example.org"},
		{"http://localhost:8080", "", "http://localhost:8080"},
		{"https://api.execai.ru", "https://web.example.org/", "https://web.example.org"},
	}
	for _, c := range cases {
		if got := WebBase(c.api, c.saved); got != c.want {
			t.Errorf("WebBase(%q, %q) = %q, want %q", c.api, c.saved, got, c.want)
		}
	}
	if got := PhoneURL("https://execai.ru", "a b/c"); got != "https://execai.ru/m/a%20b%2Fc" {
		t.Errorf("phone url %q", got)
	}
	if got := PhoneURL("https://execai.ru", ""); got != "" {
		t.Errorf("phone url without an id %q", got)
	}
	// The web base the login saw wins over a guess from api_base.
	in := full()
	in.Creds.WebBase = "https://web.example.org"
	if got := Build(in).PhoneURL; got != "https://web.example.org/m/agent-1" {
		t.Errorf("saved web base: %q", got)
	}
}

// The page to pick a plan: on the web of this installation, like /m/<id>;
// the production API without a saved web base keeps the production address.
func TestPaymentURL(t *testing.T) {
	cases := []struct{ api, saved, want string }{
		{"https://api.execai.ru", "", plan.SubscribeURL},
		{"https://api.execai.ru", "https://chat.execai.ru", "https://chat.execai.ru/settings/payment?utm_source=agent&utm_medium=cli&utm_campaign=gate"},
		{"https://apidev.example.com", "https://vbai-dev.example.com/", "https://vbai-dev.example.com/settings/payment?utm_source=agent&utm_medium=cli&utm_campaign=gate"},
		{"https://chat.example.org/api", "", "https://chat.example.org/settings/payment?utm_source=agent&utm_medium=cli&utm_campaign=gate"},
	}
	for _, c := range cases {
		if got := PaymentURL(c.api, c.saved); got != c.want {
			t.Errorf("PaymentURL(%q, %q) = %q, want %q", c.api, c.saved, got, c.want)
		}
	}
}
