package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
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
	if st := b.svc.Status(); !strings.Contains(st, "Проекты, привязанные к этому компьютеру: 1 (Alpha)") || !strings.Contains(st, "проекты (1)") {
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

// Both sides have content: the server's version stays whatever the clocks
// say; this side's EXECAI.md stays next to it (.local-<time>) and is not
// uploaded.
func TestProjects_ConflictKeepsLosingCopy(t *testing.T) {
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	stub, a, b, pA, pB := projectPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "from A\n", time.Time{})
	now(a)

	// B's copy differs from the server's: B gets A's, keeps its own.
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

	// Both edit again, B later by the clock, without having seen A's
	// version: the server's (A's) stays, B's is kept next to it (row 4).
	writeProjectFile(t, pA, "EXECAI.md", "A again\n", past)
	now(a)
	stub.clock = stub.clock.Add(time.Second)
	writeProjectFile(t, pB, "EXECAI.md", "B newer\n", future)
	b.use()
	b.svc.SyncNow(context.Background())
	if readProjectFile(pB, "EXECAI.md") != "A again\n" || !hasCopyOf(ProjectDir(pB), "B newer\n") {
		t.Fatalf("the server's should stay: %q %v", readProjectFile(pB, "EXECAI.md"), copiesIn(ProjectDir(pB)))
	}
	now(a)
	if readProjectFile(pA, "EXECAI.md") != "A again\n" {
		t.Fatalf("A has %q", readProjectFile(pA, "EXECAI.md"))
	}
	// The copies stay on B and never reach A; A replaced nothing of its own.
	if got := copiesIn(ProjectDir(pA)); len(got) != 0 {
		t.Fatalf("copies on A: %v", got)
	}
}

// Only this machine's bindings are synced: another machine's project is not
// pulled into its path even when that path exists here, and a session
// without a machine id (a browser token) has no projects of its own.
func TestService_ProjectsOnlyOwnBindings(t *testing.T) {
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	mine, theirs := t.TempDir(), t.TempDir()
	stub.mu.Lock()
	stub.bindings = []Binding{
		{AgentID: "agent-other", WorkspaceID: "ws-theirs", LocalPath: theirs},
		{AgentID: "agent-a", WorkspaceID: "ws-mine", LocalPath: mine},
	}
	stub.mu.Unlock()
	got, err := a.svc.projects(context.Background())
	if err != nil || len(got) != 1 || got["ws-mine"] != mine {
		t.Fatalf("projects = %v, %v; want only ws-mine", got, err)
	}
	a.svc.Creds = func() *config.Credentials { return &config.Credentials{Token: "tok-a"} }
	if got, _ := a.svc.projects(context.Background()); len(got) != 0 {
		t.Fatalf("no machine id: projects = %v, want none", got)
	}
}

// An older agent deleted the whole project after it was edited here: the
// files here — with the edit — are not erased (the receiving side of
// keep.go), and the edit, not sent yet, goes to the server as a change.
func TestProjects_EditMeetsTombstoneFromAnOldAgent(t *testing.T) {
	i18n.SetLocale("en")
	stub, a, b, pA, pB := projectPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "v1\n", time.Time{})
	now(a)
	now(b)

	writeProjectFile(t, pB, "EXECAI.md", "edited here before the deletion\n", past)
	old := NewClient(stub.URL, func() string { return "tok-old" })
	if _, err := old.Delete(context.Background(), KindProjectMemory, "ws-1", stub.blob(KindProjectMemory, "ws-1").version); err != nil {
		t.Fatal(err)
	}
	now(b)
	if got := readProjectFile(pB, "EXECAI.md"); got != "edited here before the deletion\n" {
		t.Fatalf("the edit made here is gone: %q", got)
	}
	// The edit was made here after the last exchange and never sent: it is
	// a change, and it goes to the server (review R10) — the computer that
	// sent the deletion mark gets the project back.
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Error("an edit made here and not sent yet stayed behind the deletion mark")
	}
	now(a)
	if got := readProjectFile(pA, "EXECAI.md"); got != "edited here before the deletion\n" {
		t.Errorf("A has %q", got)
	}
}
