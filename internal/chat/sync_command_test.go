package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

// syncBackend — just enough of billing and /agents-vbai/sync for the chat
// side of /sync: a plan, an empty server that accepts writes.
type syncBackend struct {
	mu       gosync.Mutex
	tariff   string
	identity []byte
	blobs    int
	// put — kind/name of every blob written; created/bound — projects.
	put      []string
	created  []string
	bindings []map[string]string
}

func newSyncBackend(t *testing.T, tariff string) (*syncBackend, *httptest.Server) {
	b := &syncBackend{tariff: tariff}
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, code int, v any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /billing-vbai/my_plan", func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		reply(w, 200, map[string]any{"tariff_code": b.tariff, "state": "active",
			"valid_until": "2099-01-01T00:00:00Z", "is_accessible": true})
	})
	mux.HandleFunc("GET /agents-vbai/sync/identity", func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.identity == nil {
			reply(w, 404, map[string]any{"error": "none"})
			return
		}
		reply(w, 200, map[string]any{"wrapped": b.identity, "version": 1})
	})
	mux.HandleFunc("PUT /agents-vbai/sync/identity", func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		var req struct {
			Wrapped []byte `json:"wrapped"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		b.identity = req.Wrapped
		reply(w, 200, map[string]any{"version": 1})
	})
	mux.HandleFunc("GET /agents-vbai/sync/blobs", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"blobs": []any{}})
	})
	mux.HandleFunc("PUT /agents-vbai/sync/blobs/{kind}/{name}", func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.blobs++
		b.put = append(b.put, r.PathValue("kind")+"/"+r.PathValue("name"))
		b.mu.Unlock()
		reply(w, 200, map[string]any{"version": 1})
	})
	mux.HandleFunc("GET /agents-vbai/workspaces/bindings", func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		out := []map[string]string{}
		for _, bd := range b.bindings {
			out = append(out, map[string]string{"agent_id": "agent-1", "workspace_id": bd["workspace_id"], "local_path": bd["local_path"]})
		}
		reply(w, 200, map[string]any{"bindings": out})
	})
	mux.HandleFunc("POST /agents-vbai/workspaces/bind", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		b.mu.Lock()
		b.bindings = append(b.bindings, req)
		b.mu.Unlock()
		reply(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /integrations-vbai/workspaces", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		b.mu.Lock()
		b.created = append(b.created, req.Name)
		b.mu.Unlock()
		reply(w, 200, map[string]any{"id": "ws-new", "name": req.Name, "tools": []any{}})
	})
	mux.HandleFunc("GET /integrations-vbai/workspaces", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, []any{map[string]any{"id": "ws-new", "name": "my-app"}})
	})
	mux.HandleFunc("POST /integrations-vbai/workspaces/{id}/tools", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"id": 1})
	})
	mux.HandleFunc("PUT /agents-vbai/sync/machines/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return b, srv
}

// syncTestModel — a chat in its own config directory, signed in, against
// the stub.
func syncTestModel(t *testing.T, base string) *tuiModel {
	m := newTestModel()
	m.cfg.APIBase = base
	m.textarea = textarea.New()
	return m
}

// drive runs a command chain the way bubbletea would and feeds the
// resulting messages back.
func drive(m *tuiModel, cmd tea.Cmd) {
	for cmd != nil {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				drive(m, c)
			}
			return
		}
		_, cmd = m.Update(msg)
	}
}

// enter types a line and presses Enter.
func enter(m *tuiModel, line string) {
	m.textarea.SetValue(line)
	m.lastKeyAt = time.Time{} // a test types faster than any human: no paste-burst
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	drive(m, cmd)
}

func isolateSync(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	if err := config.SaveCredentials(&config.Credentials{Token: "tok", Email: "u@example.com", AgentID: "agent-1"}); err != nil {
		t.Fatal(err)
	}
	prev := agentsync.ScryptWorkFactor
	agentsync.ScryptWorkFactor = 10
	t.Cleanup(func() { agentsync.ScryptWorkFactor = prev })
	i18n.SetLocale("ru")
	t.Cleanup(func() { i18n.SetLocale("en") })
}

// Without Work/Max: one refusal naming the plans, no conversation.
func TestSyncCommand_NoPlan(t *testing.T) {
	isolateSync(t)
	b, srv := newSyncBackend(t, "week_start")
	m := syncTestModel(t, srv.URL)
	drive(m, m.handleSyncCommand("/sync on"))
	if h := lastHint(m); !strings.Contains(h, "«Работа»") || !strings.Contains(h, "«Макс»") {
		t.Fatalf("refusal: %q", h)
	}
	if m.syncFlow != nil || b.identity != nil {
		t.Fatal("the setup went on without a plan")
	}
}

// The password is dots while typed, dots in the transcript, absent from the
// input history; the setup ends with what syncs and what does not.
func TestSyncCommand_OnWithMaskedPassword(t *testing.T) {
	isolateSync(t)
	b, srv := newSyncBackend(t, "week_work")
	m := syncTestModel(t, srv.URL)
	drive(m, m.handleSyncCommand("/sync on"))
	if m.syncFlow == nil || !m.syncStep.Secret {
		t.Fatalf("no password question: %q", lastHint(m))
	}
	const pw = "my sync password"
	m.textarea.SetValue(pw)
	if v := m.inputView(); strings.Contains(v, pw) || !strings.Contains(v, "•") {
		t.Fatalf("password visible while typed: %q", v)
	}
	enter(m, pw)
	enter(m, pw)
	if m.syncFlow != nil {
		t.Fatalf("setup not finished: %q", lastHint(m))
	}
	h := lastHint(m)
	if !strings.Contains(h, "Синхронизация включена") || !strings.Contains(h, "всё зашифровано на этом компьютере") {
		t.Fatalf("summary: %q", h)
	}
	for _, s := range m.uiSegments {
		if strings.Contains(s.body, pw) {
			t.Fatalf("password in the transcript: %+v", s)
		}
	}
	for _, l := range m.inputHistory {
		if strings.Contains(l, pw) {
			t.Fatal("password in the input history")
		}
	}
	if b.identity == nil {
		t.Fatal("the wrapped key did not reach the server")
	}
	// The status is local and names the command.
	drive(m, m.handleSyncCommand("/sync"))
	if !strings.Contains(lastHint(m), "/sync now") {
		t.Fatalf("status: %q", lastHint(m))
	}
}

// Esc leaves a conversation; nothing is deleted.
func TestSyncCommand_EscCancels(t *testing.T) {
	isolateSync(t)
	_, srv := newSyncBackend(t, "week_work")
	m := syncTestModel(t, srv.URL)
	drive(m, m.handleSyncCommand("/sync forget"))
	if m.syncFlow == nil || !strings.Contains(lastHint(m), "«удалить»") {
		t.Fatalf("forget question: %q", lastHint(m))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.syncFlow != nil || !strings.Contains(lastHint(m), "Отменено") {
		t.Fatalf("not cancelled: %q", lastHint(m))
	}
}

// A config pulled from another computer: the chat takes the sources (keys
// included) and follows the active one; this machine's api_base stays.
func TestSyncPulled_FollowsGlobalConfig(t *testing.T) {
	isolateSync(t)
	_, srv := newSyncBackend(t, "week_work")
	m := syncTestModel(t, srv.URL)
	dir, _ := config.Dir()
	writeTestFile(t, filepath.Join(dir, "subscriptions.json"),
		`{"subscriptions":{"zai":{"provider":"zai","api_key":"k-from-a","plan":"coding"}},"active":"zai"}`)
	writeTestFile(t, filepath.Join(dir, "config.json"), `{"api_base":"https://elsewhere.example","max_iterations":9}`)
	m.applyPulled([]string{agentsync.KindConfig})
	if m.subs.Active != "zai" || m.subs.Subscriptions["zai"].APIKey != "k-from-a" {
		t.Fatalf("sources not taken: %+v", m.subs)
	}
	if !strings.Contains(lastHint(m), "Z.ai") {
		t.Fatalf("switch not announced: %q", lastHint(m))
	}
	if m.cfg.MaxIterations != 9 || m.cfg.APIBase != srv.URL {
		t.Fatalf("config: %+v", m.cfg)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// /project publish: a web project from this directory (named after it),
// bound here, its .execai/ sent at once.
func TestProjectPublish_CreatesBindsAndSends(t *testing.T) {
	isolateSync(t)
	b, srv := newSyncBackend(t, "week_work")
	m := syncTestModel(t, srv.URL)
	m.creds = &config.Credentials{Token: "tok", AgentID: "agent-1"}
	drive(m, m.handleSyncCommand("/sync on"))
	enter(m, "my sync password")
	enter(m, "my sync password")
	if m.syncFlow != nil {
		t.Fatalf("sync not on: %q", lastHint(m))
	}

	dir := filepath.Join(t.TempDir(), "my-app")
	if err := os.MkdirAll(filepath.Join(dir, ".execai"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, ".execai", "EXECAI.md"), "project notes\n")
	out := m.projectPublish("", dir, false)

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.created) != 1 || b.created[0] != "my-app" {
		t.Fatalf("created: %v", b.created)
	}
	if len(b.bindings) != 1 || b.bindings[0]["workspace_id"] != "ws-new" || b.bindings[0]["local_path"] != dir {
		t.Fatalf("bindings: %v", b.bindings)
	}
	sent := false
	for _, k := range b.put {
		sent = sent || k == agentsync.KindProjectMemory+"/ws-new"
	}
	if !sent || !strings.Contains(out, "«my-app» создан") || !strings.Contains(out, "отправлены на сервер") {
		t.Fatalf("publish: sent=%v out=%q puts=%v", sent, out, b.put)
	}
}
