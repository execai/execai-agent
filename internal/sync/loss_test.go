package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// The project memory of a whole project is never deleted from the server by
// the agent itself. A binding moved to an empty folder, a folder deleted and
// made again, a .execai/ removed by hand: the agent takes the server's
// version into the folder, as a fresh computer would, and every other
// computer keeps its files. (Live probe of 7.17: the agent sent a tombstone
// and the other computers erased their .execai/ without a copy.)

// lossPair — A and B share project ws-1; A's .execai/ has two files, both
// computers have them, the server holds them live.
func lossPair(t *testing.T) (*stubServer, *machine, *machine, string, string) {
	t.Helper()
	i18n.SetLocale("en")
	stub, a, b, pA, pB := projectPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make\n", time.Time{})
	writeProjectFile(t, pA, "notes.md", "deploy from R7\n", time.Time{})
	now(a)
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make\n" {
		t.Fatal("setup: B did not get the project")
	}
	return stub, a, b, pA, pB
}

// rebind moves machine index i's binding of ws-1 to dir.
func rebind(stub *stubServer, i int, dir string) {
	stub.mu.Lock()
	stub.bindings[i].LocalPath = dir
	stub.mu.Unlock()
}

// assertProjectAlive — the server still holds the project, and B still has
// both files.
func assertProjectAlive(t *testing.T, stub *stubServer, b *machine, pB string) {
	t.Helper()
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatal("the project memory was deleted on the server")
	}
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make\n" || readProjectFile(pB, "notes.md") != "deploy from R7\n" {
		t.Fatalf("B lost the project: EXECAI.md %q, notes.md %q", readProjectFile(pB, "EXECAI.md"), readProjectFile(pB, "notes.md"))
	}
}

// assertRestored — the folder got the server's version back.
func assertRestored(t *testing.T, dir string) {
	t.Helper()
	if readProjectFile(dir, "EXECAI.md") != "build: make\n" || readProjectFile(dir, "notes.md") != "deploy from R7\n" {
		t.Fatalf("%s did not get the project from the server", dir)
	}
}

// (a) /project unbind, then /project bind of the same project in an empty
// folder (a fresh clone of the repository).
func TestProjectLoss_RebindToEmptyFolder(t *testing.T) {
	stub, a, b, _, pB := lossPair(t)
	fresh := t.TempDir()
	rebind(stub, 0, fresh)
	a.use()
	out := a.svc.SyncProject(context.Background(), "ws-1", fresh, "Alpha")
	if strings.Contains(out, "<missing>") {
		t.Fatal(out)
	}
	now(a)
	assertRestored(t, fresh)
	assertProjectAlive(t, stub, b, pB)
}

// (b) The bound folder deleted and made again, empty.
func TestProjectLoss_FolderRecreatedEmpty(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	if err := os.RemoveAll(pA); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pA, 0o700); err != nil {
		t.Fatal(err)
	}
	now(a)
	assertRestored(t, pA)
	assertProjectAlive(t, stub, b, pB)
}

// (c) The same binding, .execai/ removed by hand.
func TestProjectLoss_ExecaiRemovedByHand(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	if err := os.RemoveAll(ProjectDir(pA)); err != nil {
		t.Fatal(err)
	}
	a.use()
	// The background round that notices the change sends only; it must not
	// delete either.
	if _, err := a.svc.exchange(context.Background(), Options{PushOnly: true}, time.Second); err != nil {
		t.Fatal(err)
	}
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatal("a push round deleted the project memory on the server")
	}
	out := now(a)
	assertRestored(t, pA)
	if !strings.Contains(out, "taken from the server") {
		t.Errorf("not told that the project was taken from the server: %s", out)
	}
	assertProjectAlive(t, stub, b, pB)
}

// (d) One file deleted inside a live .execai/: an ordinary edit that
// travels — and the computer that applies it keeps the removed file in its
// history (the server had it: row 2, silently).
func TestProjectLoss_OneFileDeletedTravelsWithACopy(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	if err := os.Remove(filepath.Join(ProjectDir(pA), "notes.md")); err != nil {
		t.Fatal(err)
	}
	now(a)
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatal("deleting one file deleted the whole project")
	}
	now(b)
	if readProjectFile(pB, "notes.md") != "<missing>" || readProjectFile(pB, "EXECAI.md") != "build: make\n" {
		t.Fatalf("the deletion did not travel as an edit: notes.md %q", readProjectFile(pB, "notes.md"))
	}
	b.use()
	if !historyHas(b.svc.Paths, place{kind: KindProjectMemory, rel: "notes.md", scope: "ws-1"}, []byte("deploy from R7\n")) {
		t.Fatal("B removed notes.md without a copy in its history")
	}
}

// (e) A fresh computer joins the project: no base, an empty folder — it
// takes the project from the server.
func TestProjectLoss_FreshComputerTakesTheProject(t *testing.T) {
	stub, _, _, _, _ := lossPair(t)
	c := newMachine(t, stub, "c")
	c.converse(c.svc.OnFlow(), password)
	pC := t.TempDir()
	stub.mu.Lock()
	stub.bindings = append(stub.bindings, Binding{AgentID: "agent-c", WorkspaceID: "ws-1", LocalPath: pC})
	stub.mu.Unlock()
	now(c)
	assertRestored(t, pC)
}

// (f) Two folders of one computer bound to the project in turn: nothing is
// deleted, and no version is lost — the first folder's older file, a version
// the server had, is replaced by the server's and kept in the history.
func TestProjectLoss_TwoFoldersInTurn(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	second := t.TempDir()
	rebind(stub, 0, second)
	now(a)
	assertRestored(t, second)
	writeProjectFile(t, second, "EXECAI.md", "build: make test\n", time.Time{})
	now(a)
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatal("the project memory was deleted on the server")
	}

	rebind(stub, 0, pA) // back to the first folder, which holds the old version
	now(a)
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatal("the project memory was deleted on the server")
	}
	a.use()
	if readProjectFile(pA, "EXECAI.md") != "build: make test\n" ||
		!historyHas(a.svc.Paths, place{kind: KindProjectMemory, rel: "EXECAI.md", scope: "ws-1"}, []byte("build: make\n")) {
		t.Errorf("a version was lost in the first folder: %q, history of the old one: %v", readProjectFile(pA, "EXECAI.md"),
			historyHas(a.svc.Paths, place{kind: KindProjectMemory, rel: "EXECAI.md", scope: "ws-1"}, []byte("build: make\n")))
	}
	now(b)
	if readProjectFile(pB, "EXECAI.md") == "<missing>" {
		t.Error("B lost the project")
	}
}

// A computer on an older agent (7.15 and below) still sends a tombstone of
// the whole project. A computer with live files keeps them, says so, and
// does not send them back by itself; an edit made afterwards sends the
// project again, as an ordinary change.
func TestProjectLoss_TombstoneFromAnOldAgentKeepsTheFiles(t *testing.T) {
	stub, _, b, _, pB := lossPair(t)
	old := NewClient(stub.URL, func() string { return "tok-old" })
	if _, err := old.Delete(context.Background(), KindProjectMemory, "ws-1", stub.blob(KindProjectMemory, "ws-1").version); err != nil {
		t.Fatal(err)
	}
	out := now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make\n" || readProjectFile(pB, "notes.md") != "deploy from R7\n" {
		t.Fatal("B erased the project after a tombstone from another computer")
	}
	if !strings.Contains(out, "marked deleted") {
		t.Errorf("not told: %s", out)
	}
	b.use()
	if st := b.svc.Status(); !strings.Contains(st, "marked deleted on the server, kept here: 1") {
		t.Errorf("status:\n%s", st)
	}
	now(b)
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || !bl.deleted {
		t.Fatal("B sent the project back by itself (ping-pong with the old agent)")
	}
	writeProjectFile(t, pB, "EXECAI.md", "build: make\nkeep it\n", time.Time{})
	now(b)
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatal("an edit after the tombstone did not send the project again")
	}
}

// Personal memory: the memory folder removed and made again by the chat
// (only the index template in it) is not a deletion of every memory file —
// the files are taken from the server; the other computer keeps them.
func TestMemoryLoss_FolderRecreatedIsNotADeletion(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	a.use()
	a.writeFile("memory/MEMORY.md", "- [Role](user_role.md) — who I am\n")
	a.writeFile("memory/user_role.md", "Go engineer\n")
	a.writeFile("memory/feedback.md", "short answers\n")
	now(a)
	now(b)

	a.use()
	if err := os.RemoveAll(filepath.Join(a.dir(), "memory")); err != nil {
		t.Fatal(err)
	}
	a.writeFile("memory/MEMORY.md", "<!-- index template -->\n")
	now(a)
	key, _ := crypto.LoadIdentity(a.dir())
	for _, rel := range []string{"user_role.md", "feedback.md"} {
		if bl := stub.blob(KindMemory, memoryName(key, rel)); bl == nil || bl.deleted {
			t.Errorf("%s deleted on the server", rel)
		}
	}
	if a.readFile("memory/user_role.md") != "Go engineer\n" {
		t.Error("A did not get its memory back from the server")
	}
	now(b)
	if b.readFile("memory/user_role.md") != "Go engineer\n" || b.readFile("memory/feedback.md") != "short answers\n" {
		t.Error("B lost memory files")
	}
}

// Personal memory: one file deleted on a computer travels, and the computer
// that applies the deletion keeps its file in the history.
func TestMemoryLoss_DeletionAppliedWithACopy(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	a.use()
	a.writeFile("memory/user_role.md", "Go engineer\n")
	a.writeFile("memory/feedback.md", "short answers\n")
	now(a)
	now(b)
	a.use()
	_ = os.Remove(filepath.Join(a.dir(), "memory", "feedback.md"))
	now(a)
	now(b)
	if b.readFile("memory/feedback.md") != "<missing>" {
		t.Fatal("the deletion did not travel")
	}
	b.use()
	if !historyHas(b.svc.Paths, place{kind: KindMemory, rel: "feedback.md"}, []byte("short answers\n")) {
		t.Fatal("B removed feedback.md without a copy in its history")
	}
}

// Settings are never deleted through sync: a computer with every settings
// file gone sends no tombstone.
func TestConfigLoss_NeverATombstone(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.writeFile("config.json", `{"api_base":"https://api.a","model":"glm"}`)
	a.converse(a.svc.OnFlow(), password, password)
	for _, f := range ConfigFiles {
		_ = os.Remove(filepath.Join(a.dir(), f))
	}
	now(a)
	if bl := stub.blob(KindConfig, NameConfig); bl == nil || bl.deleted {
		t.Fatal("the settings were deleted on the server")
	}
}

// State written by an older agent knows no folder for the project base.
// With files in the bound folder it is adopted as is: an edit travels as
// before, no copy, no «taken from the server».
func TestProjectLoss_StateFromAnOlderAgentIsAdopted(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	a.use()
	// An older agent keeps no sync-bases.json (bases.go).
	if err := os.Remove(a.svc.Paths.BasesFile()); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	out := now(a)
	if strings.Contains(out, "taken from the server") || strings.Contains(out, ".local-") || strings.Contains(out, ".server-") {
		t.Errorf("an upgrade looked like a conflict or a restore: %s", out)
	}
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make all\n" {
		t.Errorf("the edit did not travel: %q", readProjectFile(pB, "EXECAI.md"))
	}
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatal("deleted on the server")
	}
	a.use()
	if rec := loadBases(a.svc.Paths).Projects["ws-1"]; rec.Dir != pA || rec.ID == "" || rec.ID != dirIdentity(ProjectDir(pA)) {
		t.Errorf("folder of the base not recorded: %+v", rec)
	}
}

// The binding moved to a folder with other content and only a send round
// runs before a full one: nothing is deleted, and the server's version is
// not overwritten silently — the side that loses is kept as a copy.
func TestProjectLoss_MovedBindingSendOnlyRound(t *testing.T) {
	stub, a, _, _, _ := lossPair(t)
	other := t.TempDir()
	writeProjectFile(t, other, "EXECAI.md", "another checkout\n", time.Time{})
	rebind(stub, 0, other)
	a.use()
	st, _ := LoadState(a.svc.Paths)
	st.Projects["ws-1"] = other
	_ = st.Save(a.svc.Paths)
	if _, err := a.svc.exchange(context.Background(), Options{PushOnly: true}, time.Second); err != nil {
		t.Fatal(err)
	}
	bl := stub.blob(KindProjectMemory, "ws-1")
	if bl == nil || bl.deleted {
		t.Fatal("deleted on the server")
	}
	all := ""
	_ = filepath.WalkDir(ProjectDir(other), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(p)
			all += string(data)
		}
		return nil
	})
	if !strings.Contains(all, "build: make\n") || !strings.Contains(all, "another checkout\n") {
		t.Errorf("a version was lost (folder files and copies): %q", all)
	}
}

// A project kept after a tombstone and then removed here by hand stays
// deleted: it is not taken back from the server, and nothing is sent.
func TestProjectLoss_KeptProjectRemovedByHand(t *testing.T) {
	stub, _, b, _, pB := lossPair(t)
	old := NewClient(stub.URL, func() string { return "tok-old" })
	if _, err := old.Delete(context.Background(), KindProjectMemory, "ws-1", stub.blob(KindProjectMemory, "ws-1").version); err != nil {
		t.Fatal(err)
	}
	now(b)
	_ = os.RemoveAll(ProjectDir(pB))
	out := now(b)
	if readProjectFile(pB, "EXECAI.md") != "<missing>" || strings.Contains(out, "taken from the server") {
		t.Errorf("a project deleted on purpose came back: %s", out)
	}
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || !bl.deleted {
		t.Error("sent back")
	}
	b.use()
	if st := b.svc.Status(); strings.Contains(st, "kept here") {
		t.Errorf("still counted as kept:\n%s", st)
	}
}
