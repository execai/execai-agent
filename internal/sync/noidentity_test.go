package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// A file system that cannot tell folders apart (an NFS, sshfs or WSL /mnt
// mount): dirIdentity reads nothing. The folder is trusted by its path
// (threeway.go): an edit travels and is not replaced by the server's
// version every round, but a missing file is not a deletion — it comes back
// from the server, and the person is told; a memory folder made again is
// not taken for deletions either (a marker file gives no trust).
func noIdentity(t *testing.T) {
	t.Helper()
	old := dirIdentityFunc
	dirIdentityFunc = func(string) string { return "" }
	t.Cleanup(func() { dirIdentityFunc = old })
}

func TestNoIdentity_ProjectEditsTravel(t *testing.T) {
	noIdentity(t)
	stub, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	_ = os.Remove(filepath.Join(ProjectDir(pA), "notes.md"))
	outA := now(a)
	now(b)
	if readProjectFile(pA, "EXECAI.md") != "build: make all\n" || readProjectFile(pB, "EXECAI.md") != "build: make all\n" {
		t.Errorf("the edit did not travel: A %q, B %q; A said %s", readProjectFile(pA, "EXECAI.md"), readProjectFile(pB, "EXECAI.md"), outA)
	}
	if readProjectFile(pB, "notes.md") != "deploy from R7\n" || readProjectFile(pA, "notes.md") != "deploy from R7\n" {
		t.Errorf("a missing file was taken for a deletion: A %q, B %q", readProjectFile(pA, "notes.md"), readProjectFile(pB, "notes.md"))
	}
	if !strings.Contains(outA, "was missing here and was taken from the server") {
		t.Errorf("A not told: %s", outA)
	}
	// A binding moved to another folder still has no base.
	other := t.TempDir()
	writeProjectFile(t, other, "EXECAI.md", "older checkout\n", past)
	rebind(stub, 0, other)
	now(a)
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make all\n" {
		t.Errorf("LOSS: B has %q", readProjectFile(pB, "EXECAI.md"))
	}
}

func TestNoIdentity_MemoryFolderMadeAgainIsNotDeletions(t *testing.T) {
	noIdentity(t)
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	a.use()
	a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Old](old.md)\n")
	a.writeFile("memory/role.md", "Go engineer\n")
	a.writeFile("memory/old.md", "old\n")
	now(a)
	now(b)
	// An edit travels; a missing file comes back.
	a.use()
	a.writeFile("memory/role.md", "Go and Rust\n")
	_ = os.Remove(filepath.Join(a.dir(), "memory", "old.md"))
	now(a)
	now(b)
	b.use()
	if b.readFile("memory/role.md") != "Go and Rust\n" || b.readFile("memory/old.md") != "old\n" {
		t.Fatalf("B: role %q, old %q", b.readFile("memory/role.md"), b.readFile("memory/old.md"))
	}
	a.use()
	if a.readFile("memory/old.md") != "old\n" {
		t.Fatalf("A: old %q", a.readFile("memory/old.md"))
	}
	// The folder made again with the template: no deletions.
	a.use()
	_ = os.RemoveAll(filepath.Join(a.dir(), "memory"))
	a.writeFile("memory/MEMORY.md", "<!-- template -->\n")
	now(a)
	key, _ := crypto.LoadIdentity(a.dir())
	if bl := stub.blob(KindMemory, memoryName(key, "role.md")); bl == nil || bl.deleted {
		t.Fatal("LOSS: role.md deleted on the server")
	}
	if a.readFile("memory/role.md") != "Go and Rust\n" {
		t.Errorf("A did not get role.md back: %q", a.readFile("memory/role.md"))
	}
	if strings.Contains(a.readFile("memory/MEMORY.md"), "template") {
		t.Errorf("A's index: %q", a.readFile("memory/MEMORY.md"))
	}
}
