package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
)

// promptProjectMemory — the project memory path the system prompt gives the
// model: the line after "**Project memory**".
func promptProjectMemory(t *testing.T, prompt string) string {
	t.Helper()
	lines := strings.Split(prompt, "\n")
	for i, l := range lines {
		if strings.Contains(l, "**Project memory**") && i+1 < len(lines) {
			return strings.TrimSpace(lines[i+1])
		}
	}
	t.Fatalf("no project memory path in the system prompt:\n%s", prompt)
	return ""
}

// The model writes the project memory where the system prompt tells it
// to; that file must be in what sync uploads for a bound project — the
// .execai/ pack — or the memory never reaches the other computers.
func TestProjectMemory_PromptPathIsInTheProjectPack(t *testing.T) {
	dir := t.TempDir()
	path := promptProjectMemory(t, agent.SystemPrompt(dir, nil, ""))
	if !filepath.IsAbs(path) {
		t.Fatalf("prompt path %q is not absolute", path)
	}
	const fact = "Tests: make test. Deploy only from the R7 branch.\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fact), 0o600); err != nil {
		t.Fatal(err)
	}

	e := &Engine{Paths: Paths{ConfigDir: t.TempDir()}}
	it, ok, err := e.projectItem("ws-1", dir)
	if err != nil {
		t.Fatal(err)
	}
	var packed map[string][]byte
	if ok {
		if packed, err = unpackFiles(it.Data); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range packed {
		if string(body) == fact {
			return
		}
	}
	t.Fatalf("the model writes project memory to %s, but the project pack sync sends holds %d file(s) without it",
		path, len(packed))
}

// The agent and sync name the same file.
func TestProjectMemory_SamePathInAgentAndSync(t *testing.T) {
	if agent.ProjectMemoryRel != ProjectMemoryRel {
		t.Fatalf("agent writes %s, sync describes %s", agent.ProjectMemoryRel, ProjectMemoryRel)
	}
	dir := t.TempDir()
	if agent.ProjectMemoryPath(dir) != ProjectMemoryFile(dir) {
		t.Fatalf("%s != %s", agent.ProjectMemoryPath(dir), ProjectMemoryFile(dir))
	}
}
