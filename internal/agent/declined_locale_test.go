package agent

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
	"github.com/velesbsdllc/agent-vbai/internal/security"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

var cyrillicRe = regexp.MustCompile(`[А-Яа-яЁё]`)

// What a declined call returns is the tool's result the person sees in the
// tool card (the claude CLI path shows the refusal reason the same way): in
// the interface language. The scope line of a permission question too.
func TestDeclinedTool_InTheInterfaceLanguage(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	work := filepath.Join(dir, "work")
	mustDir(t, work)
	mustFile(t, filepath.Join(work, "a.txt"), "text")
	security.Set(security.Deep)
	asked := 0
	ap := &recordingApprover{decision: ApproveDeny, count: &asked}
	ag := New(nil, tools.Default(work), "", ap, nil)

	edit, _ := json.Marshal(map[string]any{"file_path": filepath.Join(work, "a.txt"), "old_string": "text", "new_string": "new"})
	if ok, reason := ag.approveDelegated("Edit", edit); ok || reason == "" || cyrillicRe.MatchString(reason) {
		t.Errorf("claude path: ok=%v reason %q", ok, reason)
	}
	write, _ := json.Marshal(map[string]any{"path": filepath.Join(work, "b.txt"), "content": "x"})
	if out, err := ag.runTool(t.Context(), "Write", write); err != nil || out == "" || cyrillicRe.MatchString(out) {
		t.Errorf("execai path: %q %v", out, err)
	}

	i18n.SetLocale("ru")
	if _, reason := ag.approveDelegated("Edit", edit); !cyrillicRe.MatchString(reason) {
		t.Errorf("ru: %q", reason)
	}
}

// /memory export that cannot write: the reason is in the interface language.
func TestMemoryExportError_InTheInterfaceLanguage(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	blocker := filepath.Join(dir, "a-file")
	mustFile(t, blocker, "x")
	_, err := ExportMemory(filepath.Join(blocker, "export"))
	if err == nil || cyrillicRe.MatchString(err.Error()) {
		t.Errorf("export error %v", err)
	}
}
