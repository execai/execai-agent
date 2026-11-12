package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// A fresh computer: the chat starts, picks the catalog's default model and
// writes it to config.json — marked as picked by the agent (model_auto), so
// that joining sync later does not take it for the person's own setting.
// A model the person then chooses with /model is theirs: it no longer
// matches the mark.
func TestBoot_DefaultModelIsMarkedAuto(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Chdir(t.TempDir())
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"glm-5.3-flash","provider":"zai","name":"GLM","is_primary":true,"has_tools":true},
			{"id":"claude-opus-5-5","provider":"anthropic","name":"Opus","has_tools":true}]`))
	}))
	defer api.Close()
	m := newTestModel()
	m.subs.Subscriptions = nil
	m.cfg = &config.Config{APIBase: api.URL}
	m.creds = &config.Credentials{Token: "tok", AgentID: "agent-1"}
	m.registry = tools.Default(t.TempDir())
	m.textarea = textarea.New()
	if err := m.bootAfterLogin(); err != nil {
		t.Fatal(err)
	}
	raw := readConfigJSON(t, dir)
	if raw["selected_model_id"] == nil || string(raw["selected_model_id"]) != string(raw["model_auto"]) {
		t.Fatalf("boot wrote %s; the default model is not marked as picked by the agent", mustJSON(raw))
	}

	m.handleCommand("/model claude-opus-5-5")
	raw = readConfigJSON(t, dir)
	if string(raw["selected_model_id"]) != `"claude-opus-5-5"` || string(raw["selected_model_id"]) == string(raw["model_auto"]) {
		t.Fatalf("after /model: %s", mustJSON(raw))
	}
}

func readConfigJSON(t *testing.T, dir string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "execai", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
