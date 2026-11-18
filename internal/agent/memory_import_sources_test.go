package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateHome points HOME and the config dir at fresh temp folders so the
// tests never see (or touch) the real ~/.claude, ~/.codex or execai memory.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", "")
	return home
}

func byPath(srcs []ImportSource) map[string]ImportSource {
	out := map[string]ImportSource{}
	for _, s := range srcs {
		out[s.Path] = s
	}
	return out
}

// Global instructions of Codex CLI, Claude Code, Gemini CLI and Windsurf are
// found; everything else in those folders (sessions, logs, auth, settings)
// is never touched.
func TestDiscoverGlobalMemorySources(t *testing.T) {
	home := isolateHome(t)
	want := map[string]string{
		filepath.Join(home, ".codex", "AGENTS.md"):                                 AgentCodex,
		filepath.Join(home, ".codex", "instructions.md"):                           AgentCodex,
		filepath.Join(home, ".claude", "CLAUDE.md"):                                AgentClaude,
		filepath.Join(home, ".gemini", "GEMINI.md"):                                AgentGemini,
		filepath.Join(home, ".codeium", "windsurf", "memories", "global_rules.md"): AgentWindsurf,
	}
	for p := range want {
		writeFile(t, p, "Answer in Russian. Go, not Python.")
	}
	noise := []string{
		filepath.Join(home, ".codex", "auth.json"),
		filepath.Join(home, ".codex", "config.toml"),
		filepath.Join(home, ".codex", "history.jsonl"),
		filepath.Join(home, ".codex", "sessions", "2026", "rollout.jsonl"),
		filepath.Join(home, ".codex", "log", "codex-tui.log"),
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(home, ".claude", ".credentials.json"),
		filepath.Join(home, ".claude", "projects", "-x", "session.jsonl"),
		filepath.Join(home, ".gemini", "settings.json"),
		filepath.Join(home, ".gemini", "oauth_creds.json"),
	}
	for _, p := range noise {
		writeFile(t, p, `{"api_key":"sk-zzzzzzzzzzzzzzzzzzzzzzzzzzzz"}`)
	}

	got := byPath(DiscoverGlobalMemorySources(t.TempDir()))
	for p, agentID := range want {
		s, ok := got[p]
		if !ok {
			t.Errorf("not found: %s", p)
			continue
		}
		if s.Agent != agentID {
			t.Errorf("%s: agent %q, want %q", p, s.Agent, agentID)
		}
		if !s.Global {
			t.Errorf("%s: must be marked global (applies to every project)", p)
		}
		if s.Label() == "" || s.Tool == "" {
			t.Errorf("%s: no label for the user", p)
		}
		if !strings.HasPrefix(s.Rel, "~") {
			t.Errorf("%s: shown as %q, want a ~/ path", p, s.Rel)
		}
	}
	for _, p := range noise {
		if _, ok := got[p]; ok {
			t.Errorf("service file picked up: %s", p)
		}
	}
	if len(got) != len(want) {
		t.Errorf("found %d sources, want %d: %v", len(got), len(want), got)
	}
}

// CODEX_HOME moves Codex CLI's folder; then ~/.codex is not Codex's anymore.
func TestDiscoverGlobalMemorySources_CodexHome(t *testing.T) {
	home := isolateHome(t)
	custom := filepath.Join(home, "codex-elsewhere")
	t.Setenv("CODEX_HOME", custom)
	writeFile(t, filepath.Join(custom, "AGENTS.md"), "global codex rules")
	writeFile(t, filepath.Join(home, ".codex", "AGENTS.md"), "stale")

	got := byPath(DiscoverGlobalMemorySources(t.TempDir()))
	if _, ok := got[filepath.Join(custom, "AGENTS.md")]; !ok {
		t.Error("AGENTS.md under CODEX_HOME not found")
	}
	if _, ok := got[filepath.Join(home, ".codex", "AGENTS.md")]; ok {
		t.Error("~/.codex used although CODEX_HOME points elsewhere")
	}
}

// Claude Code keeps per-project memory outside the project, under
// ~/.claude/projects/<encoded path>/memory/. Its facts belong to this
// project; its own MEMORY.md index is not a fact and is skipped.
func TestDiscoverClaudeProjectMemory(t *testing.T) {
	home := isolateHome(t)
	proj := filepath.Join(home, "work", "my.app")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, mem := range claudeProjectMemoryDirs(proj) {
		_ = os.MkdirAll(mem, 0o755)
	}
	mem := ClaudeProjectMemoryDir(proj)
	writeFile(t, filepath.Join(mem, "MEMORY.md"), "- [role](user_role.md) — Go dev")
	writeFile(t, filepath.Join(mem, "user_role.md"), "---\nname: role\ndescription: senior Go dev\n---\n\nGo, bubbletea")
	writeFile(t, filepath.Join(filepath.Dir(mem), "abc.jsonl"), `{"type":"session"}`)

	got := DiscoverAllMemorySources(proj)
	var role *ImportSource
	for i, s := range got {
		if strings.HasSuffix(s.Path, "MEMORY.md") {
			t.Error("Claude's MEMORY.md index picked up as a fact")
		}
		if strings.HasSuffix(s.Path, ".jsonl") {
			t.Error("Claude session picked up")
		}
		if strings.HasSuffix(s.Path, "user_role.md") {
			role = &got[i]
		}
	}
	if role == nil {
		t.Fatalf("Claude project memory not found: %+v", got)
	}
	if role.Global {
		t.Error("project memory of Claude Code marked global")
	}
	if role.Agent != AgentClaude {
		t.Errorf("agent %q, want claude", role.Agent)
	}
}

// New project-level files: personal CLAUDE.local.md, Windsurf rules folder,
// Copilot path-specific instructions.
func TestDiscoverMemorySources_MoreProjectFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.local.md"), "my sandbox urls")
	writeFile(t, filepath.Join(dir, ".windsurf", "rules", "style.md"), "tabs")
	writeFile(t, filepath.Join(dir, ".github", "instructions", "go.instructions.md"), "gofmt")
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "run make test")
	writeFile(t, filepath.Join(dir, "GEMINI.md"), "be brief")

	want := map[string]string{
		"CLAUDE.local.md":                         AgentClaude,
		".windsurf/rules/style.md":                AgentWindsurf,
		".github/instructions/go.instructions.md": AgentCopilot,
		"AGENTS.md":                               AgentCodex,
		"GEMINI.md":                               AgentGemini,
	}
	got := map[string]ImportSource{}
	for _, s := range DiscoverMemorySources(dir) {
		got[filepath.ToSlash(s.Rel)] = s
	}
	for rel, agentID := range want {
		s, ok := got[rel]
		if !ok {
			t.Errorf("not found: %s", rel)
			continue
		}
		if s.Agent != agentID {
			t.Errorf("%s: agent %q, want %q", rel, s.Agent, agentID)
		}
		if s.Global {
			t.Errorf("%s: project file marked global", rel)
		}
	}
}

// A file that holds something like a key is never imported: memory is synced,
// and a key that leaked into a CLAUDE.md must not travel further.
func TestImport_SkipsFilesWithSecrets(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "# Project\nprod on R5\nANTHROPIC_API_KEY=sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789\n")
	writeFile(t, filepath.Join(dir, ".cursorrules"), "Answer briefly.")

	srcs := DiscoverMemorySources(dir)
	var claude ImportSource
	for _, s := range srcs {
		if s.Rel == "CLAUDE.md" {
			claude = s
		}
	}
	if claude.SecretLine != 3 {
		t.Fatalf("secret line = %d, want 3", claude.SecretLine)
	}
	if strings.Contains(claude.Preview, "sk-ant") {
		t.Error("the key leaks into the preview")
	}

	res, err := ImportSources(srcs)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 1 || len(res.Skipped) != 1 {
		t.Fatalf("imported %v, skipped %v; want 1 and 1", res.Imported, res.Skipped)
	}
	memDir, _ := UserMemoryDir()
	entries, _ := os.ReadDir(memDir)
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(memDir, e.Name()))
		if strings.Contains(string(data), "sk-ant-api03") {
			t.Fatalf("the key got into memory: %s", e.Name())
		}
	}
}

func TestSecretLine(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"plain notes\nnothing here", 0},
		{"use sk-xxx as a placeholder", 0},
		{"token: <your token>", 0},
		{"the secret lives in billing-vbai-secret", 0},
		{"x\n-----BEGIN OPENSSH PRIVATE KEY-----\nabc", 2},
		{"github: ghp_abcdefghijklmnopqrstuvwxyz0123456789", 1},
		{"aws AKIAABCDEFGHIJKLMNOP", 1},
		{"yandex y0_AgAAAAAbcdefghijklmnopqrstuvwxyz012345", 1},
		{"a\nb\npassword = hunter2hunter2", 3},
		{"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", 1},
	}
	for _, c := range cases {
		if got := secretLine(c.text); got != c.want {
			t.Errorf("secretLine(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

// Two projects each with their own CLAUDE.md: both land in memory under
// different names, instead of the second one being silently skipped.
func TestImportSources_SameFileInTwoProjects(t *testing.T) {
	isolateHome(t)
	a := filepath.Join(t.TempDir(), "alpha")
	b := filepath.Join(t.TempDir(), "beta")
	writeFile(t, filepath.Join(a, "CLAUDE.md"), "alpha rules")
	writeFile(t, filepath.Join(b, "CLAUDE.md"), "beta rules")

	ra, err := ImportSources(DiscoverMemorySources(a))
	if err != nil || len(ra.Imported) != 1 {
		t.Fatalf("alpha: %v %v", ra, err)
	}
	rb, err := ImportSources(DiscoverMemorySources(b))
	if err != nil || len(rb.Imported) != 1 {
		t.Fatalf("beta: %+v %v", rb, err)
	}
	if ra.Imported[0] == rb.Imported[0] {
		t.Errorf("both projects mapped to %s", ra.Imported[0])
	}
}

// Importing the same file again is a no-op with an explanation, even when
// the memory file was renamed by the user afterwards.
func TestImportSources_AlreadyImported(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "run make test")
	srcs := DiscoverMemorySources(dir)
	first, err := ImportSources(srcs)
	if err != nil || len(first.Imported) != 1 {
		t.Fatalf("first: %+v %v", first, err)
	}
	memDir, _ := UserMemoryDir()
	if err := os.Rename(filepath.Join(memDir, first.Imported[0]), filepath.Join(memDir, "renamed.md")); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanImport(srcs)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 || plan[0].Target != "" || plan[0].Skip == "" {
		t.Fatalf("re-import not skipped: %+v", plan)
	}
}

// The plan is what --dry-run shows: it must not create anything, not even
// the memory folder.
func TestPlanImport_WritesNothing(t *testing.T) {
	home := isolateHome(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "rules")

	plan, err := PlanImport(DiscoverAllMemorySources(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 || plan[0].Target == "" {
		t.Fatalf("plan: %+v", plan)
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
		t.Errorf("PlanImport created the config folder (err=%v)", err)
	}
}

func TestParseImportFrom(t *testing.T) {
	got, err := ParseImportFrom("claude,cursor")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	if all, err := ParseImportFrom("all"); err != nil || all != nil {
		t.Errorf("all → %v, %v; want nil (no filter)", all, err)
	}
	if all, err := ParseImportFrom(""); err != nil || all != nil {
		t.Errorf("empty → %v, %v; want nil (no filter)", all, err)
	}
	if _, err := ParseImportFrom("vim"); err == nil {
		t.Error("unknown agent accepted")
	}

	srcs := []ImportSource{{Agent: AgentClaude}, {Agent: AgentCodex}, {Agent: AgentExecai}}
	if f := FilterSourcesByAgent(srcs, []string{AgentCodex}); len(f) != 1 || f[0].Agent != AgentCodex {
		t.Errorf("filter codex: %+v", f)
	}
	if f := FilterSourcesByAgent(srcs, nil); len(f) != 3 {
		t.Errorf("no filter must keep everything: %+v", f)
	}
}

func TestPreviewSkipsFrontmatterAndNamesAreClean(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "style.mdc"), "---\ndescription: style\nglobs: *.go\n---\nComments in English.")
	writeFile(t, filepath.Join(dir, ".cursorrules"), "Answer briefly.")
	for _, s := range DiscoverMemorySources(dir) {
		if strings.Contains(s.Preview, "globs") {
			t.Errorf("%s: frontmatter in preview: %q", s.Rel, s.Preview)
		}
		if name := importedFileName(s); strings.Contains(name, "__") {
			t.Errorf("%s: ugly name %s", s.Rel, name)
		}
	}
}
