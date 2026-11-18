package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// memoryHintSetup — a config directory with sync on (or off) and project
// ws-1 bound at a fresh folder (or not); the folder holds a root EXECAI.md.
func memoryHintSetup(t *testing.T, syncOn, bound bool) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)
	i18n.SetLocale("en")
	dir := t.TempDir()
	writeTestFile(t, agent.ProjectInstructionsPath(dir), "build: make\n")
	st := map[string]any{"enabled": syncOn}
	if bound {
		st["projects"] = map[string]string{"ws-1": dir}
	}
	data, _ := json.Marshal(st)
	if err := os.MkdirAll(filepath.Join(home, "execai"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(home, "execai", "sync.json"), string(data))
	return dir
}

func hints(m *tuiModel) string {
	var b strings.Builder
	for _, s := range m.uiSegments {
		if s.kind == "system_hint" {
			b.WriteString(s.body + "\n")
		}
	}
	return b.String()
}

// Bound project, sync on, memory only in the root EXECAI.md: the chat says
// so once — not on the next start.
func TestProjectMemoryHint_Once(t *testing.T) {
	dir := memoryHintSetup(t, true, true)
	m := newTestModel()
	m.maybeHintProjectMemory(dir)
	if !strings.Contains(hints(m), "/project memory") {
		t.Fatalf("no hint:\n%s", hints(m))
	}
	again := newTestModel()
	again.maybeHintProjectMemory(dir)
	if hints(again) != "" {
		t.Errorf("hint repeated:\n%s", hints(again))
	}
}

// No hint where nothing stays behind or nothing would travel anyway.
func TestProjectMemoryHint_NotWhenNotNeeded(t *testing.T) {
	for _, c := range []struct {
		name          string
		syncOn, bound bool
		execai        bool
	}{
		{"sync off", false, true, false},
		{"folder not bound", true, false, false},
		{".execai/EXECAI.md already there", true, true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := memoryHintSetup(t, c.syncOn, c.bound)
			if c.execai {
				_ = os.MkdirAll(filepath.Dir(agent.ProjectMemoryPath(dir)), 0o755)
				writeTestFile(t, agent.ProjectMemoryPath(dir), "synced\n")
			}
			m := newTestModel()
			m.maybeHintProjectMemory(dir)
			if h := hints(m); h != "" {
				t.Errorf("hint shown:\n%s", h)
			}
		})
	}
}

// /project memory copies the root file into .execai/ and leaves the root
// one in place.
func TestProjectMemoryCommand(t *testing.T) {
	dir := memoryHintSetup(t, true, true)
	t.Chdir(dir)
	out := newTestModel().handleProjectCommand("/project memory")
	if !strings.Contains(out, agent.ProjectMemoryPath(dir)) || !strings.HasPrefix(out, "Copied") {
		t.Fatalf("answer: %q", out)
	}
	for _, p := range []string{agent.ProjectMemoryPath(dir), agent.ProjectInstructionsPath(dir)} {
		if b, err := os.ReadFile(p); err != nil || string(b) != "build: make\n" {
			t.Errorf("%s: %q %v", p, b, err)
		}
	}
	if out := newTestModel().handleProjectCommand("/project memory"); !strings.Contains(out, "already has the same text") {
		t.Errorf("second run: %q", out)
	}
}

// /project memory says what will happen next, not always «now travels»:
// that only holds for a bound folder with sync on; otherwise it names what
// is missing — the binding, sync, or both.
func TestProjectMemoryCommand_TextFollowsState(t *testing.T) {
	for _, c := range []struct {
		name          string
		syncOn, bound bool
		want, not     []string
	}{
		{"bound, sync on", true, true, []string{"now travels"}, []string{"/project bind", "/sync on"}},
		{"not bound", true, false, []string{"/project bind"}, []string{"now travels", "/sync on"}},
		{"sync off", false, true, []string{"/sync on"}, []string{"now travels", "/project bind"}},
		{"neither", false, false, []string{"/project bind", "/sync on"}, []string{"now travels"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := memoryHintSetup(t, c.syncOn, c.bound)
			t.Chdir(dir)
			out := newTestModel().handleProjectCommand("/project memory")
			if !strings.HasPrefix(out, "Copied") {
				t.Fatalf("not copied: %q", out)
			}
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("lacks %q: %s", w, out)
				}
			}
			for _, n := range c.not {
				if strings.Contains(out, n) {
					t.Errorf("says %q: %s", n, out)
				}
			}
		})
	}
}

// Signed in as an agent, the binding comes from the server — this
// machine's own — even before sync.json has seen it.
func TestProjectMemoryCommand_BindingFromServer(t *testing.T) {
	dir := memoryHintSetup(t, true, false)
	t.Chdir(dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"bindings": []map[string]string{
			{"agent_id": testAgentID, "workspace_id": "ws-1", "local_path": dir},
		}})
	}))
	defer srv.Close()
	if out := modelWithAPI(srv.URL).handleProjectCommand("/project memory"); !strings.Contains(out, "now travels") {
		t.Errorf("bound on the server: %s", out)
	}
}
