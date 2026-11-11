package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// A project folder and the server's pack are merged file by file
// (threeway.go): a file only one side has stays on both, of two different
// versions the server's stays where this folder cannot be trusted or both
// changed, and this folder's is kept next to it. Times of files decide
// nothing (a clone has fresh times). The pack is never replaced as a whole.

// copies — the copies next to files in dir: name → content.
func copiesIn(dir string) map[string]string {
	out := map[string]string{}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && localOnly(d.Name()) {
			data, _ := os.ReadFile(p)
			out[d.Name()] = string(data)
		}
		return nil
	})
	return out
}

func hasCopyOf(dir, content string) bool {
	for _, c := range copiesIn(dir) {
		if c == content {
			return true
		}
	}
	return false
}

// Probe 9c: on the new agent the binding moves, with no exchange, to a
// fresh clone holding an older .execai/ — its files have the time of the
// clone, newer than the server's. They are earlier versions of what the
// server has (a rollback): the server's version stays (here, on the server,
// on the other computer), the clone's goes to the history, and the person
// is told once.
func TestMerge_Probe9c_FreshCloneWithOlderFiles(t *testing.T) {
	stub, a, b, _, pB := lossPair(t)
	writeProjectFile(t, pB, "EXECAI.md", "build: make\nnew from B\n", time.Time{})
	now(b)
	now(a)
	clone := t.TempDir()
	writeProjectFile(t, clone, "EXECAI.md", "build: make\n", time.Now()) // fresh time of the clone
	writeProjectFile(t, clone, "notes.md", "deploy from R7\n", time.Now())
	rebind(stub, 0, clone)
	v := serverVersion(stub)
	out := now(a)
	now(b)
	if got := readProjectFile(clone, "EXECAI.md"); got != "build: make\nnew from B\n" {
		t.Errorf("the clone has %q, want the server's version", got)
	}
	if got := readProjectFile(pB, "EXECAI.md"); got != "build: make\nnew from B\n" {
		t.Errorf("LOSS: B has %q", got)
	}
	if serverVersion(stub) != v {
		t.Error("the clone's older files were sent")
	}
	a.use()
	if !historyHas(a.svc.Paths, place{kind: KindProjectMemory, rel: "EXECAI.md", scope: "ws-1"}, []byte("build: make\n")) {
		t.Errorf("the clone's own version is not kept: %v", copiesIn(ProjectDir(clone)))
	}
	if len(copiesIn(ProjectDir(clone))) != 0 {
		t.Errorf("a version the server had is kept in the project: %v", copiesIn(ProjectDir(clone)))
	}
	if !strings.Contains(out, "was an older version of what the server has") || !strings.Contains(out, "sync-history") {
		t.Errorf("not told: %s", out)
	}
}

// R15 of the review: the binding moves to a clone where the agent already
// wrote a newer EXECAI.md. notes.md, which only the server has, stays in the
// project on every computer; plan.md, made only in the clone, goes up (no
// base for it: row 5a); the clone's EXECAI.md — the folder is not the one
// the base was agreed in — is kept next to the server's (row 3b).
func TestMerge_FilesOnlyOnTheServerStay(t *testing.T) {
	stub, a, b, _, pB := lossPair(t)
	fresh := t.TempDir()
	writeProjectFile(t, fresh, "EXECAI.md", "build: make\nnew note from the agent\n", time.Time{})
	writeProjectFile(t, fresh, "plan.md", "only in the clone\n", time.Time{})
	rebind(stub, 0, fresh)
	now(a)
	now(b)
	for name, dir := range map[string]string{"A": fresh, "B": pB} {
		if readProjectFile(dir, "notes.md") != "deploy from R7\n" {
			t.Errorf("%s: notes.md dropped from the project", name)
		}
		if readProjectFile(dir, "plan.md") != "only in the clone\n" {
			t.Errorf("%s: plan.md, only in the clone, is not in the project", name)
		}
	}
	if !hasCopyOf(ProjectDir(fresh), "build: make\nnew note from the agent\n") {
		t.Errorf("the clone's EXECAI.md is not kept: %v", copiesIn(ProjectDir(fresh)))
	}
}

// R14 of the review: a repository cloned again at the same path brings
// .execai/.gitkeep. It is another folder (bases.go) and holds nothing of
// the project: the project is taken from the server on A, B keeps it.
func TestMerge_RecloneAtTheSamePathWithAPlaceholder(t *testing.T) {
	_, a, b, pA, pB := lossPair(t)
	_ = os.RemoveAll(pA)
	writeProjectFile(t, pA, ".gitkeep", "", time.Time{})
	now(a)
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make\n" || readProjectFile(pA, "EXECAI.md") != "build: make\n" ||
		readProjectFile(pA, "notes.md") != "deploy from R7\n" || readProjectFile(pB, "notes.md") != "deploy from R7\n" {
		t.Errorf("PROJECT MEMORY GONE: A %v, B %v", rvList(ProjectDir(pA)), rvList(ProjectDir(pB)))
	}
}

// The same with the old folder kept by inode: .execai/ emptied in place of
// everything but a placeholder is empty, not «everything deleted».
func TestMerge_OnlyAPlaceholderLeftIsEmpty(t *testing.T) {
	_, a, b, pA, pB := lossPair(t)
	for _, f := range []string{"EXECAI.md", "notes.md"} {
		_ = os.Remove(filepath.Join(ProjectDir(pA), f))
	}
	writeProjectFile(t, pA, ".DS_Store", "x", time.Time{})
	now(a)
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make\n" || readProjectFile(pA, "notes.md") != "deploy from R7\n" {
		t.Errorf("A %v, B %v", rvList(ProjectDir(pA)), rvList(ProjectDir(pB)))
	}
}

// R4 of the review (rule П4, K1 of the third): a project kept here after a
// deletion mark from 7.15, then that computer writes a fresh EXECAI.md into
// its new clone and sends it over the mark. After the mark there is no base:
// B's EXECAI.md differs from the fresh one (row 5c) — the server's is used
// here and B's is kept next to it for good, and B is told; notes.md, which
// the clone lacks, goes back up (5a) and stays in the project.
func TestMerge_KeptProjectMeetsANewVersion(t *testing.T) {
	stub, a, b, _, pB := lossPair(t)
	old := NewClient(stub.URL, func() string { return "tok-old" })
	ctx := context.Background()
	if _, err := old.Delete(ctx, KindProjectMemory, "ws-1", stub.blob(KindProjectMemory, "ws-1").version); err != nil {
		t.Fatal(err)
	}
	now(b) // kept
	a.use()
	key, err := crypto.LoadIdentity(a.dir())
	if err != nil {
		t.Fatal(err)
	}
	ct, err := seal(key, KindProjectMemory, "ws-1", mustPack(map[string][]byte{"EXECAI.md": []byte("fresh clone, nothing yet\n")}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Put(ctx, KindProjectMemory, "ws-1", ct, stub.blob(KindProjectMemory, "ws-1").version); err != nil {
		t.Fatal(err)
	}
	out := now(b)
	if !hasCopyOf(ProjectDir(pB), "build: make\n") || !strings.Contains(out, "there was no agreed earlier version") {
		t.Errorf("LOSS: B's kept EXECAI.md replaced without a copy next to it, or silently: %v\n%s", rvList(ProjectDir(pB)), out)
	}
	if readProjectFile(pB, "notes.md") != "deploy from R7\n" {
		t.Errorf("B's notes.md: %q", readProjectFile(pB, "notes.md"))
	}
}

// R10 of the review: an edit made here after the last exchange, not sent
// yet, when a deletion mark arrives, is a change: it goes to the server.
// Without such an edit the kept project is not sent back (no ping-pong).
func TestMerge_UnsentEditMeetsADeletionMark(t *testing.T) {
	stub, _, b, _, pB := lossPair(t)
	writeProjectFile(t, pB, "EXECAI.md", "build: make\nedit on B not sent yet\n", time.Time{})
	old := NewClient(stub.URL, func() string { return "tok-old" })
	if _, err := old.Delete(context.Background(), KindProjectMemory, "ws-1", stub.blob(KindProjectMemory, "ws-1").version); err != nil {
		t.Fatal(err)
	}
	now(b)
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatal("the edit made before the mark arrived was not sent")
	}
	v := serverVersion(stub)
	now(b)
	now(b)
	if serverVersion(stub) != v {
		t.Error("sent again with nothing changed")
	}
}

// With a base, both sides changed: file by file, a file added on either
// side stays everywhere; of two versions of one file the server's stays —
// whichever was later by the clock — and the other is kept as a copy on
// the computer that had it (row 4).
func TestMerge_BothChangedKeepsFilesOfBothSides(t *testing.T) {
	_, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "from-a.md", "A's new file\n", time.Time{})
	writeProjectFile(t, pA, "EXECAI.md", "build: make\nA's edit\n", past)
	now(a)
	writeProjectFile(t, pB, "from-b.md", "B's new file\n", time.Time{})
	writeProjectFile(t, pB, "EXECAI.md", "build: make\nB's edit\n", future)
	now(b)
	now(a)
	for name, dir := range map[string]string{"A": pA, "B": pB} {
		if readProjectFile(dir, "from-a.md") != "A's new file\n" || readProjectFile(dir, "from-b.md") != "B's new file\n" {
			t.Errorf("%s lost a file only one side had: %v", name, rvList(ProjectDir(dir)))
		}
		if readProjectFile(dir, "EXECAI.md") != "build: make\nA's edit\n" {
			t.Errorf("%s: EXECAI.md = %q, want the server's (A's)", name, readProjectFile(dir, "EXECAI.md"))
		}
	}
	if !hasCopyOf(ProjectDir(pB), "build: make\nB's edit\n") {
		t.Errorf("B's edit is not kept on B: %v", copiesIn(ProjectDir(pB)))
	}
	if len(copiesIn(ProjectDir(pA))) != 0 {
		t.Errorf("copies on A: %v", copiesIn(ProjectDir(pA)))
	}
}
