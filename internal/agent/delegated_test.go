package agent

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/security"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// Claude's tool calls go through the same gate as execai's own: reading the
// project is quiet, an edit asks, a refusal reaches claude, and "always" for
// Edit given once covers both paths.
func TestApproveDelegatedUsesAgentGate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	work := filepath.Join(dir, "проект")
	mustDir(t, work)
	mustFile(t, filepath.Join(work, "a.txt"), "текст")
	security.Set(security.Deep)

	asked := 0
	ap := &recordingApprover{decision: ApproveDeny, count: &asked}
	ag := New(nil, tools.Default(work), "", ap, nil)

	in := func(v map[string]any) json.RawMessage { raw, _ := json.Marshal(v); return raw }

	if ok, _ := ag.approveDelegated("Read", in(map[string]any{"file_path": filepath.Join(work, "a.txt")})); !ok || asked != 0 {
		t.Fatalf("reading inside the project: ok=%v asked=%d, want quiet allow", ok, asked)
	}
	edit := in(map[string]any{"file_path": filepath.Join(work, "a.txt"), "old_string": "текст", "new_string": "новый"})
	if ok, reason := ag.approveDelegated("Edit", edit); ok || asked != 1 || reason == "" {
		t.Fatalf("denied edit: ok=%v asked=%d reason=%q", ok, asked, reason)
	}
	// An unknown tool (an MCP tool of claude) is asked about, not waved through.
	if ok, _ := ag.approveDelegated("mcp__github__create_issue", in(map[string]any{"title": "x"})); ok || asked != 2 {
		t.Fatalf("unknown tool: ok=%v asked=%d", ok, asked)
	}

	ap.decision = ApproveAlways
	if ok, _ := ag.approveDelegated("Edit", edit); !ok {
		t.Fatal("allowed edit refused")
	}
	if !ag.Permissions.HasTool("Edit") {
		t.Fatal(`"always" for claude's Edit must be saved as execai's Edit`)
	}
	ap.decision = ApproveDeny
	if ok, _ := ag.approveDelegated("Edit", edit); !ok || asked != 3 {
		t.Fatalf(`after "always" the edit must pass without a question: ok=%v asked=%d`, ok, asked)
	}
}

func TestDelegatedToNativeTranslatesArgs(t *testing.T) {
	name, args := delegatedToNative("Write", json.RawMessage(`{"file_path":"/w/a.txt","content":"x"}`))
	if name != "Write" || string(args) != `{"content":"x","path":"/w/a.txt"}` {
		t.Fatalf("Write → %s %s", name, args)
	}
	name, args = delegatedToNative("Bash", json.RawMessage(`{"command":"ls","timeout":120000}`))
	if name != "Bash" || string(args) != `{"command":"ls"}` {
		t.Fatalf("Bash → %s %s (timeout units differ and must not pass)", name, args)
	}
	name, _ = delegatedToNative("TodoWrite", json.RawMessage(`{}`))
	if name != "TodoWrite" {
		t.Fatalf("TodoWrite → %s", name)
	}
}
