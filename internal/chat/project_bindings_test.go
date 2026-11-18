package chat

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// otherMachineAPI — an account with two machines: the other one bound
// project "site" at dir; this machine (testAgentID) bound "api" at its own
// path. It records the write requests.
func otherMachineAPI(t *testing.T, dir, ownPath string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var writes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/integrations-vbai/workspaces":
			_, _ = w.Write([]byte(`[{"id":"w1","name":"site","tools":[{"id":5,"tool_id":"agent","profile":"` + testAgentID + `","enabled":true}]},
				{"id":"w2","name":"api","tools":[]}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/agents-vbai/workspaces/bindings":
			_ = json.NewEncoder(w).Encode(map[string]any{"bindings": []map[string]string{
				{"agent_id": "other-machine", "workspace_id": "w1", "local_path": dir},
				{"agent_id": testAgentID, "workspace_id": "w2", "local_path": ownPath},
			}})
		default:
			mu.Lock()
			writes = append(writes, r.Method+" "+r.URL.Path+" "+string(raw))
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), writes...)
	}
}

// /project in a folder that another machine of the account bound at the
// same path: the folder is not shown as bound here, the other machine's
// path is not listed as this machine's, and this machine's own binding is.
func TestProjectList_OnlyOwnPath(t *testing.T) {
	i18n.SetLocale("en")
	cwd := t.TempDir()
	t.Chdir(cwd)
	ownPath := t.TempDir()
	srv, _ := otherMachineAPI(t, cwd, ownPath)
	out := modelWithAPI(srv.URL).projectList()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "site") && strings.HasPrefix(line, "● ") {
			t.Errorf("the folder shows as bound to another machine's project:\n%s", out)
		}
	}
	if strings.Contains(out, "↳ "+cwd) {
		t.Errorf("another machine's path listed as this machine's:\n%s", out)
	}
	if !strings.Contains(out, "↳ "+ownPath) {
		t.Errorf("this machine's own binding missing:\n%s", out)
	}
}

// /project off and /project unbind in that folder act on nothing: this
// machine is not bound to the other machine's project, so neither its
// switch in the project nor the project's membership may change.
func TestProjectToggleUnbind_OtherMachinesFolder(t *testing.T) {
	i18n.SetLocale("en")
	cwd := t.TempDir()
	t.Chdir(cwd)
	srv, writes := otherMachineAPI(t, cwd, t.TempDir())
	m := modelWithAPI(srv.URL)
	want := i18n.Tf("project.notBound", cwd)
	if out := m.projectToggle(false); out != want {
		t.Errorf("/project off: %q, want %q", out, want)
	}
	if out := m.projectUnbind(); out != want {
		t.Errorf("/project unbind: %q, want %q", out, want)
	}
	if w := writes(); len(w) != 0 {
		t.Errorf("requests that change the project:\n%s", strings.Join(w, "\n"))
	}
}

// The folder was bound through a symlink and the chat runs in the real
// path (or the other way round): it is the same folder — /project marks it
// as bound here, and /project off finds its project, as the editor panel
// does (inbox.SamePath).
func TestProjectList_SymlinkedFolderIsThisOne(t *testing.T) {
	i18n.SetLocale("en")
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "app-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	t.Chdir(real)
	var mu sync.Mutex
	var writes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/integrations-vbai/workspaces":
			_, _ = w.Write([]byte(`[{"id":"w1","name":"site","tools":[{"id":5,"tool_id":"agent","profile":"` + testAgentID + `","enabled":true}]}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/agents-vbai/workspaces/bindings":
			_ = json.NewEncoder(w).Encode(map[string]any{"bindings": []map[string]string{
				{"agent_id": testAgentID, "workspace_id": "w1", "local_path": link},
			}})
		default:
			mu.Lock()
			writes = append(writes, r.Method+" "+r.URL.Path)
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	m := modelWithAPI(srv.URL)
	out := m.projectList()
	if !strings.Contains(out, "● site") {
		t.Errorf("the folder bound through a link is not marked as this one:\n%s", out)
	}
	if got := m.projectToggle(false); strings.Contains(got, "No project is bound") {
		t.Errorf("/project off: %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(writes) != 1 || writes[0] != "PUT /integrations-vbai/workspaces/w1/tools/5" {
		t.Errorf("/project off requests: %v", writes)
	}
}

// After /project publish or bind the chat asks whether to start the
// background listener. The question is only that question: the result of
// publish/bind is an ordinary message, so the answered question («Question:
// … → answer») does not swallow «Project … bound to …».
func TestProjectBindPublish_QuestionIsOnlyTheQuestion(t *testing.T) {
	isolateSync(t)
	i18n.SetLocale("en")
	_, srv := newSyncBackend(t, "week_work")
	m := syncTestModel(t, srv.URL)
	m.creds = &config.Credentials{Token: "tok", AgentID: "agent-1"}

	dir := filepath.Join(t.TempDir(), "my-app")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	out := m.projectPublish("", dir, true)
	if !m.asking || m.askQuestion != i18n.T("project.serveQuestion") {
		t.Errorf("publish: question %q", m.askQuestion)
	}
	if !strings.Contains(out, "\"my-app\"") {
		t.Errorf("publish: the result is not a message of its own: %q", out)
	}

	m.asking, m.askQuestion, m.askPending = false, "", nil
	t.Chdir(dir)
	out = m.projectBind("my-app")
	if !m.asking || m.askQuestion != i18n.T("project.serveQuestion") {
		t.Errorf("bind: question %q", m.askQuestion)
	}
	if !strings.Contains(out, dir) {
		t.Errorf("bind: the result is not a message of its own: %q", out)
	}
}

// Н-4 of the second loss review: /project bind and /project publish in a
// folder this machine already bound to another project do not bind it —
// one folder holds one project's memory, and sync would mix the two on
// every computer; the person is told which project has the folder.
// Binding the same project again is fine.
func TestProjectBind_OneFolderOneProject(t *testing.T) {
	i18n.SetLocale("en")
	ownPath := t.TempDir()
	t.Chdir(ownPath)
	srv, writes := otherMachineAPI(t, t.TempDir(), ownPath)
	m := modelWithAPI(srv.URL)
	out := m.projectBind("site")
	if !strings.Contains(out, "already bound to the project \"api\"") || !strings.Contains(out, "\"site\" was not bound") {
		t.Errorf("bind told: %q", out)
	}
	out = m.projectPublish("fresh", ownPath, false)
	if !strings.Contains(out, "already bound to the project \"api\"") {
		t.Errorf("publish told: %q", out)
	}
	for _, w := range writes() {
		if strings.Contains(w, "/workspaces/bind") || strings.HasPrefix(w, "POST /integrations-vbai/workspaces ") {
			t.Errorf("written: %s", w)
		}
	}
	m.projectBind("api")
	found := false
	for _, w := range writes() {
		found = found || strings.Contains(w, `/agents-vbai/workspaces/bind {"local_path":"`+ownPath+`","workspace_id":"w2"}`)
	}
	if !found {
		t.Errorf("binding the same project again was refused: %v", writes())
	}
}

// The bind and unbind lines show the folder as sync's own lines do: under
// the home folder as ~/… (texts of the second loss probe).
func TestProjectBind_FolderUnderHomeWithTilde(t *testing.T) {
	i18n.SetLocale("en")
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "tilde-proj")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Chdir(dir)
	srv, _ := otherMachineAPI(t, t.TempDir(), dir)
	m := modelWithAPI(srv.URL)
	out := m.projectBind("api")
	if !strings.Contains(out, "bound to ~/tilde-proj") || strings.Contains(out, home) {
		t.Errorf("bind said: %q", out)
	}
	out = m.projectUnbind()
	if !strings.Contains(out, "~/tilde-proj") || strings.Contains(out, home) {
		t.Errorf("unbind said: %q", out)
	}
}
