package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/machine"
	"github.com/velesbsdllc/agent-vbai/internal/welcome"
)

// machineTestModel — a chat in its own config directory against a stub that
// lists the bindings of this agent.
func machineTestModel(t *testing.T, creds *config.Credentials, bindings string) *tuiModel {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	i18n.SetLocale("ru")
	t.Cleanup(func() { i18n.SetLocale("en") })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/agents-vbai/workspaces/bindings" {
			_, _ = w.Write([]byte(bindings))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	m := newTestModel()
	m.cfg.APIBase = srv.URL
	m.creds = creds
	m.textarea = textarea.New()
	if creds != nil {
		if err := config.SaveCredentials(creds); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func lastSegment(m *tuiModel, kind string) string {
	for i := len(m.uiSegments) - 1; i >= 0; i-- {
		if m.uiSegments[i].kind == kind {
			return m.uiSegments[i].body
		}
	}
	return ""
}

// /start — five steps, each with a mark by the real state, a line on why
// and the command to type.
func TestStartCommand_FiveSteps(t *testing.T) {
	m := machineTestModel(t, nil, `{"bindings":[]}`)
	enter(m, "/start")
	h := lastHint(m)
	for _, want := range []string{
		i18n.T("start.login.title"), i18n.T("start.models.title"), i18n.T("start.sync.title"),
		i18n.T("start.project.title"), i18n.T("start.phone.title"),
		"/login", "/connect", "/sync on", "/project", "/phone",
		"api_base", i18n.Tf("start.progress", 0, 5),
	} {
		if !strings.Contains(h, want) {
			t.Errorf("/start lacks %q:\n%s", want, h)
		}
	}
	if strings.Count(h, markTodo) != 5 || strings.Contains(h, markDone) {
		t.Errorf("marks of a fresh machine:\n%s", h)
	}
}

// Signed in, the folder bound on the server: two steps are done.
func TestStartCommand_MarksFromState(t *testing.T) {
	cr := &config.Credentials{Token: "tok", Email: "alice@example.com", AgentID: "agent-1"}
	m := machineTestModel(t, cr, `{"bindings":[{"agent_id":"agent-1","workspace_id":"w1","local_path":"/src"}]}`)
	m.subs.Subscriptions = nil
	enter(m, "/start")
	h := lastHint(m)
	if strings.Count(h, markDone) != 2 || !strings.Contains(h, i18n.Tf("start.progress", 2, 5)) {
		t.Errorf("login and project should be done:\n%s", h)
	}
}

// /machine — the card: the address is masked, every block is there.
func TestMachineCommand_Card(t *testing.T) {
	cr := &config.Credentials{Token: "tok", Email: "alice@example.com", AgentID: "agent-1", Alias: "laptop"}
	m := machineTestModel(t, cr, `{"bindings":[]}`)
	enter(m, "/machine")
	h := lastHint(m)
	if strings.Contains(h, "alice@example.com") || !strings.Contains(h, "a***@e***.com") {
		t.Errorf("email not masked:\n%s", h)
	}
	for _, want := range []string{
		i18n.T("machine.title"), "laptop", i18n.T("machine.label.models"), "ExecAI", "Z.ai Coding Plan",
		i18n.T("machine.label.sync"), i18n.T("machine.sync.off"),
		i18n.T("machine.label.presence"), i18n.T("machine.presence.noListener"),
	} {
		if !strings.Contains(h, want) {
			t.Errorf("/machine lacks %q:\n%s", want, h)
		}
	}
}

// /phone without a login explains what is missing; no code.
func TestPhoneCommand_NeedsLogin(t *testing.T) {
	m := machineTestModel(t, nil, `{"bindings":[]}`)
	enter(m, "/phone")
	if h := lastHint(m); !strings.Contains(h, i18n.T("phone.needLogin")) {
		t.Errorf("hint %q", h)
	}
	if lastSegment(m, "qr") != "" {
		t.Error("a QR code without an agent")
	}
	if !machine.LoadMarks().PhoneShownAt.IsZero() {
		t.Error("phone marked as shown without a link")
	}
}

// /phone: the code, the same link as text, and step 5 is done from now on.
func TestPhoneCommand_QRAndLink(t *testing.T) {
	cr := &config.Credentials{Token: "tok", Email: "a@b.c", AgentID: "agent-1", WebBase: "https://web.example.org"}
	m := machineTestModel(t, cr, `{"bindings":[]}`)
	enter(m, "/phone")
	qr := lastSegment(m, "qr")
	if qr == "" || !strings.Contains(qr, "▀") && !strings.Contains(qr, "▄") {
		t.Fatalf("no QR code: %q", qr)
	}
	if h := lastHint(m); !strings.Contains(h, "https://web.example.org/m/agent-1") {
		t.Errorf("link not shown as text:\n%s", h)
	}
	if machine.LoadMarks().PhoneShownAt.IsZero() {
		t.Error("phone not marked as shown")
	}
	enter(m, "/start")
	if h := lastHint(m); strings.Count(h, markDone) != 2 {
		t.Errorf("login and phone should be done:\n%s", h)
	}
}

// The first launch shows the tutorial instead of the old greeting — once.
func TestFirstLaunch_ShowsTutorialOnce(t *testing.T) {
	machineTestModel(t, nil, `{}`)
	if !welcome.FirstLaunch() {
		t.Fatal("first launch not detected")
	}
	if welcome.FirstLaunch() {
		t.Fatal("second launch detected as first")
	}
	m := newTestModel()
	m.creds = nil
	text := m.tutorialText(m.localSnapshot())
	if !strings.Contains(text, i18n.T("start.login.title")) || !strings.Contains(text, "/start") {
		t.Errorf("tutorial text:\n%s", text)
	}
}

// The three commands are in /help and in the suggestions, in every language.
func TestMachineCommands_Discoverable(t *testing.T) {
	for _, loc := range []string{"en", "ru", "de", "es", "zh"} {
		i18n.SetLocale(loc)
		h := i18n.T("help.body")
		for _, c := range []string{"/start", "/machine", "/phone"} {
			if !strings.Contains(h, c) {
				t.Errorf("%s: help lacks %s", loc, c)
			}
		}
	}
	i18n.SetLocale("en")
	found := map[string]bool{}
	for _, s := range allCommands {
		found[s.label] = true
	}
	for _, c := range []string{"/start", "/machine", "/machine check", "/phone"} {
		if !found[c] {
			t.Errorf("suggestions lack %s", c)
		}
	}
}

// Lines added before the program runs (the first-launch steps, the sign-in
// prompt) stay pending and are printed once it runs — not marked printed
// and lost, as they were in the default (non-classic) mode.
func TestStartupSegments_NotLostBeforeRun(t *testing.T) {
	prev := teaProgramRef
	teaProgramRef = nil
	t.Cleanup(func() { teaProgramRef = prev })
	m := newTestModel()
	m.uiSegments = append(m.uiSegments, segment{kind: "system_hint", body: "steps"})
	m.refreshViewport()
	if m.lastEmittedIdx != 0 {
		t.Fatalf("a startup line was marked printed before the program ran (idx %d)", m.lastEmittedIdx)
	}
}
