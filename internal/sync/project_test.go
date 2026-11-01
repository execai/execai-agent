package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

func writeProjectFile(t *testing.T, dir, rel, content string, mtime time.Time) {
	t.Helper()
	p := filepath.Join(ProjectDir(dir), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if !mtime.IsZero() {
		_ = os.Chtimes(p, mtime, mtime)
	}
}

func readProjectFile(dir, rel string) string {
	b, err := os.ReadFile(filepath.Join(ProjectDir(dir), filepath.FromSlash(rel)))
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

// projectPair — two machines with sync on, ws-1 bound on both in their own
// directories.
func projectPair(t *testing.T) (*stubServer, *machine, *machine, string, string) {
	stub := newStub(t)
	stub.workspaces = []Workspace{{ID: "ws-1", Name: "Alpha"}}
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	pA, pB := t.TempDir(), t.TempDir()
	stub.mu.Lock()
	stub.bindings = []Binding{
		{AgentID: "agent-a", WorkspaceID: "ws-1", LocalPath: pA},
		{AgentID: "agent-b", WorkspaceID: "ws-1", LocalPath: pB},
	}
	stub.mu.Unlock()
	return stub, a, b, pA, pB
}

func now(m *machine) string {
	m.use()
	return m.svc.SyncNow(context.Background())
}

// The whole .execai/ travels as one blob between machines that keep the
// project in different directories; a deletion travels too; the server
// sees ciphertext only; unbinding leaves the blob for the others.
func TestProjects_TravelBetweenMachines(t *testing.T) {
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	stub, a, b, pA, pB := projectPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "Проект на Go, тесты: make test\n", time.Time{})
	writeProjectFile(t, pA, "notes/plan.md", "план\n", time.Time{})
	now(a)

	bl := stub.blob(KindProjectMemory, "ws-1")
	if bl == nil || bl.deleted {
		t.Fatal("project blob not sent")
	}
	if strings.Contains(string(bl.content), "make test") || strings.Contains(string(bl.content), "plan.md") {
		t.Fatal("project content or file names readable on the server")
	}
	stub.mu.Lock()
	n := 0
	for k := range stub.blobs {
		if strings.HasPrefix(k, KindProjectMemory+"/") {
			n++
		}
	}
	stub.mu.Unlock()
	if n != 1 {
		t.Fatalf("project blobs: %d, want one per project", n)
	}

	now(b)
	if readProjectFile(pB, "EXECAI.md") != "Проект на Go, тесты: make test\n" || readProjectFile(pB, "notes/plan.md") != "план\n" {
		t.Fatal("project did not arrive in B's directory")
	}
	b.use()
	if st := b.svc.Status(); !strings.Contains(st, "Проекты: 1 (Alpha)") {
		t.Fatalf("status: %s", st)
	}

	_ = os.Remove(filepath.Join(ProjectDir(pB), "notes", "plan.md"))
	now(b)
	now(a)
	if readProjectFile(pA, "notes/plan.md") != "<missing>" || readProjectFile(pA, "EXECAI.md") == "<missing>" {
		t.Fatal("deletion inside .execai/ did not travel, or took too much")
	}

	// Unbound on A: the blob stays for B.
	stub.mu.Lock()
	stub.bindings = stub.bindings[1:]
	stub.mu.Unlock()
	_ = os.RemoveAll(ProjectDir(pA))
	now(a)
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatal("unbinding deleted the project for everyone")
	}
}

// /project bind on an empty directory takes the project from the server.
func TestProjects_BindEmptyFolderPulls(t *testing.T) {
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	stub, a, b, pA, _ := projectPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "память проекта\n", time.Time{})
	now(a)

	fresh := t.TempDir() // B binds another, empty directory
	stub.mu.Lock()
	stub.bindings[1].LocalPath = fresh
	stub.mu.Unlock()
	b.use()
	out := b.svc.SyncProject(context.Background(), "ws-1", fresh, "Alpha")
	if !strings.Contains(out, "получены с сервера") || readProjectFile(fresh, "EXECAI.md") != "память проекта\n" {
		t.Fatalf("bind did not pull: %q", out)
	}
}

// Both sides have content: the fresher wins; the losing EXECAI.md stays
// next to it (.local-<time> or .server-<time>) and is not uploaded.
func TestProjects_ConflictKeepsLosingCopy(t *testing.T) {
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	stub, a, b, pA, pB := projectPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "from A\n", time.Time{})
	now(a)

	// B's copy is older than the server's: B gets A's, keeps its own.
	writeProjectFile(t, pB, "EXECAI.md", "from B\n", past)
	b.use()
	out := b.svc.SyncProject(context.Background(), "ws-1", pB, "Alpha")
	if readProjectFile(pB, "EXECAI.md") != "from A\n" || !strings.Contains(out, "EXECAI.md.local-") {
		t.Fatalf("remote should win: %q / %q", readProjectFile(pB, "EXECAI.md"), out)
	}
	locals, _ := filepath.Glob(filepath.Join(ProjectDir(pB), "EXECAI.md.local-*"))
	if len(locals) != 1 {
		t.Fatalf("local copy: %v", locals)
	}
	if got, _ := os.ReadFile(locals[0]); string(got) != "from B\n" {
		t.Fatalf("local copy holds %q", got)
	}

	// Now a newer edit on B against an older one on A: B wins, A's text is
	// kept on B as .server-<time>.
	writeProjectFile(t, pA, "EXECAI.md", "A again\n", past)
	now(a)
	stub.clock = stub.clock.Add(time.Second)
	writeProjectFile(t, pB, "EXECAI.md", "B newer\n", future)
	// B has not seen A's last version: a conflict.
	b.use()
	b.svc.SyncNow(context.Background())
	servers, _ := filepath.Glob(filepath.Join(ProjectDir(pB), "EXECAI.md.server-*"))
	if readProjectFile(pB, "EXECAI.md") != "B newer\n" || len(servers) != 1 {
		t.Fatalf("local should win: %q %v", readProjectFile(pB, "EXECAI.md"), servers)
	}
	now(a)
	if readProjectFile(pA, "EXECAI.md") != "B newer\n" {
		t.Fatalf("A has %q", readProjectFile(pA, "EXECAI.md"))
	}
	// The copies stay on B and never reach A.
	if got, _ := filepath.Glob(filepath.Join(ProjectDir(pA), "EXECAI.md.*-*")); len(got) != 0 {
		t.Fatalf("conflict copies travelled: %v", got)
	}
}
