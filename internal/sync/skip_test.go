package sync

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// What sync cannot read, what is over the limit and what is a symbolic link
// is not «no file» (rule П1 of the E105 loss review): it is left out of the
// exchange in both directions — not sent, not written over from the server,
// not removed, no deletion sent — and the person is told why, once.

// rvAll — every file under dir with its name, for «is it still anywhere».
func rvAll(dir string) string {
	all := ""
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(p)
			all += "[" + filepath.Base(p) + "]" + string(data)
		}
		return nil
	})
	return all
}

// rvList — the files under dir, relative.
func rvList(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, strings.TrimPrefix(p, dir))
		}
		return nil
	})
	return out
}

// serverVersion — the project's blob version on the stub.
func serverVersion(stub *stubServer) int64 {
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl != nil {
		return bl.version
	}
	return 0
}

// R1 of the review: a file of 1.5 MB put into .execai/ (a PDF of the
// specification) takes the pack over the limit. The project is left out:
// the big file and an edit of EXECAI.md stay, nothing goes to the server.
func TestSkip_TooBigProjectIsLeftAlone(t *testing.T) {
	stub, a, _, pA, _ := lossPair(t)
	big := make([]byte, 1500<<10)
	_, _ = rand.Read(big)
	if err := os.WriteFile(filepath.Join(ProjectDir(pA), "dataset.bin"), big, 0o600); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, pA, "EXECAI.md", "build: make\nlocal only edit\n", time.Time{})
	v := serverVersion(stub)
	out := now(a)
	if _, err := os.Stat(filepath.Join(ProjectDir(pA), "dataset.bin")); err != nil {
		t.Errorf("LOSS: dataset.bin (1.5 MB, only copy) deleted: %v", err)
	}
	if got := readProjectFile(pA, "EXECAI.md"); got != "build: make\nlocal only edit\n" {
		t.Errorf("LOSS: local edit of EXECAI.md reverted to %q; folder: %v", got, rvList(ProjectDir(pA)))
	}
	if serverVersion(stub) != v {
		t.Error("a project left out was sent")
	}
	if !strings.Contains(out, "Not synced (larger than the limit of 1024 KB): "+ProjectDir(pA)) {
		t.Errorf("not told: %s", out)
	}
}

// R2 of the review: one unreadable entry in .execai/ (a folder made under
// sudo; on Windows a file a virus scanner holds) is left out alone: it
// stays as it is here and on the server, the rest of the project syncs —
// the edit goes up — the exchange does not fail, and the next one does not
// take the edit back either (it was a loop before). Said once.
func TestSkip_UnreadableEntryLeavesTheProjectAlone(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads everything")
	}
	stub, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make\nlocal only edit\n", time.Time{})
	locked := filepath.Join(ProjectDir(pA), "private")
	if err := os.MkdirAll(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(locked, "x.md"), []byte("x"), 0o600)
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	for i := 0; i < 2; i++ {
		out := now(a)
		if strings.Contains(out, "Could not reach") || strings.Contains(out, "error") {
			t.Errorf("round %d failed: %s", i, out)
		}
		told := strings.Contains(out, "Not synced (no permission to read") && strings.Contains(out, ".execai/private") &&
			strings.Contains(out, "the rest syncs")
		if i == 0 && !told {
			t.Errorf("round %d: not told: %s", i, out)
		}
		if i > 0 && strings.Contains(out, "Not synced") {
			t.Errorf("round %d: told again: %s", i, out)
		}
		if got := readProjectFile(pA, "EXECAI.md"); got != "build: make\nlocal only edit\n" {
			t.Fatalf("LOSS (round %d): the edit was overwritten by the server's %q", i, got)
		}
	}
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	if srv := serverPack(t, stub, key, "ws-1"); srv["EXECAI.md"] != "build: make\nlocal only edit\n" || srv["notes.md"] != "deploy from R7\n" {
		t.Errorf("the rest of the project did not sync: %v", srv)
	}
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make\nlocal only edit\n" || readProjectFile(pB, "private/x.md") != "<missing>" {
		t.Errorf("B: %v", rvList(ProjectDir(pB)))
	}
}

// R3 of the review: the only memory note besides the index grows over the
// limit. It is left out: not overwritten by the server's older version.
func TestSkip_TooBigMemoryNoteLeftAlone(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	a.use()
	a.writeFile("memory/MEMORY.md", "- [Log](log.md)\n")
	a.writeFile("memory/log.md", "v1\n")
	now(a)
	big := strings.Repeat("a line of my long running notes\n", 10000) // ~320 KB
	a.use()
	a.writeFile("memory/log.md", big)
	out := now(a)
	if got := a.readFile("memory/log.md"); got != big {
		t.Errorf("LOSS: big log.md (%d bytes, only copy) replaced by %q", len(big), got)
	}
	if !strings.Contains(out, "larger than the limit of 256 KB") {
		t.Errorf("not told: %s", out)
	}
}

// R12 of the review (rule П6): a memory note among several grows over the
// limit. It is never deleted on the server or on another computer — not in
// a full round, not in a send-only one.
func TestSkip_TooBigMemoryNoteNeverDeleted(t *testing.T) {
	for _, pushOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "full", true: "sendOnly"}[pushOnly], func(t *testing.T) {
			i18n.SetLocale("en")
			stub := newStub(t)
			a := newMachine(t, stub, "a")
			a.converse(a.svc.OnFlow(), password, password)
			b := newMachine(t, stub, "b")
			b.converse(b.svc.OnFlow(), password)
			a.use()
			a.writeFile("memory/MEMORY.md", "- [Log](log.md)\n- [Role](role.md)\n")
			a.writeFile("memory/log.md", "v1\n")
			a.writeFile("memory/role.md", "Go engineer\n")
			now(a)
			now(b)
			a.use()
			a.writeFile("memory/log.md", strings.Repeat("a line of my long running notes\n", 10000))
			if pushOnly {
				if _, err := a.svc.exchange(context.Background(), Options{PushOnly: true}, time.Second); err != nil {
					t.Fatal(err)
				}
			}
			now(a)
			key, _ := crypto.LoadIdentity(a.dir())
			if bl := stub.blob(KindMemory, memoryName(key, "log.md")); bl == nil || bl.deleted {
				t.Fatal("LOSS: a memory file that only grew too big was deleted on the server")
			}
			now(b)
			if got := b.readFile("memory/log.md"); got != "v1\n" {
				t.Errorf("B's log.md = %q", got)
			}
		})
	}
}

// R6 of the review: .execai/ is a symbolic link (shared by git worktrees).
// Sync does not go through it: the files behind it stay, nothing is sent,
// the exchange does not fail.
func TestSkip_LinkedExecaiLeftAlone(t *testing.T) {
	stub, a, _, pA, _ := lossPair(t)
	shared := filepath.Join(t.TempDir(), "shared-execai")
	if err := os.Rename(ProjectDir(pA), shared); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, ProjectDir(pA)); err != nil {
		t.Skip("no symlinks:", err)
	}
	if err := os.WriteFile(filepath.Join(shared, "EXECAI.md"), []byte("build: make\nlocal only edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := serverVersion(stub)
	out := now(a)
	got, _ := os.ReadFile(filepath.Join(shared, "EXECAI.md"))
	if string(got) != "build: make\nlocal only edit\n" {
		t.Errorf("LOSS: EXECAI.md behind the link overwritten with %q; folder %v", got, rvList(shared))
	}
	if strings.Contains(out, "not found") || strings.Contains(out, "Could not reach") {
		t.Errorf("the exchange failed: %s", out)
	}
	if !strings.Contains(out, "a symbolic link") || serverVersion(stub) != v {
		t.Errorf("sent %v, told: %s", serverVersion(stub) != v, out)
	}
}

// R7 of the review: the personal memory folder is a symbolic link. Neither
// a full round nor a send-only one touches it or deletes anything on the
// server; the other computer keeps its files.
func TestSkip_LinkedMemoryFolderLeftAlone(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	a.use()
	a.writeFile("memory/MEMORY.md", "- [Role](user_role.md)\n")
	a.writeFile("memory/user_role.md", "Go engineer\n")
	now(a)
	now(b)
	a.use()
	real := filepath.Join(t.TempDir(), "memory-real")
	if err := os.Rename(filepath.Join(a.dir(), "memory"), real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(a.dir(), "memory")); err != nil {
		t.Skip("no symlinks:", err)
	}
	_ = os.WriteFile(filepath.Join(real, "user_role.md"), []byte("Go engineer\nlocal edit\n"), 0o600)
	out := now(a)
	got, _ := os.ReadFile(filepath.Join(real, "user_role.md"))
	if string(got) != "Go engineer\nlocal edit\n" {
		t.Errorf("LOSS(full round): user_role.md behind the link overwritten with %q", got)
	}
	if !strings.Contains(out, "a symbolic link") {
		t.Errorf("not told: %s", out)
	}
	a.use()
	_ = os.WriteFile(filepath.Join(real, "user_role.md"), []byte("Go engineer\nlocal edit 2\n"), 0o600)
	if _, err := a.svc.exchange(context.Background(), Options{PushOnly: true}, time.Second); err != nil {
		t.Fatal(err)
	}
	key, _ := crypto.LoadIdentity(a.dir())
	if bl := stub.blob(KindMemory, memoryName(key, "user_role.md")); bl == nil || bl.deleted {
		t.Errorf("LOSS(send-only round): user_role.md deleted on the server")
	}
	now(b)
	if b.readFile("memory/user_role.md") != "Go engineer\n" {
		t.Errorf("B's user_role.md = %q", b.readFile("memory/user_role.md"))
	}
}

// The folder bound to a project was removed (or its disk is not mounted):
// the exchange says so plainly, counts nothing as received, writes nothing
// (no folder made), sends nothing; once the folder is back, empty, the
// project is taken from the server.
func TestSkip_BoundFolderNotHere(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	if err := os.RemoveAll(pA); err != nil {
		t.Fatal(err)
	}
	v := serverVersion(stub)
	out := now(a)
	if !strings.Contains(out, "Done: 0 received, 0 sent.") {
		t.Errorf("the count lies: %s", out)
	}
	if !strings.Contains(out, "The folder "+pA+" bound to a project is not on this computer") {
		t.Errorf("not told: %s", out)
	}
	if _, err := os.Stat(pA); !os.IsNotExist(err) {
		t.Error("the bound folder was made")
	}
	if serverVersion(stub) != v {
		t.Error("something was sent")
	}
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make\n" {
		t.Fatal("B lost the project")
	}
	_ = os.MkdirAll(pA, 0o700)
	now(a)
	assertRestored(t, pA)
}

// What is left out is told once per process by the background runner,
// not on every round.
func TestSkip_ToldOncePerProcess(t *testing.T) {
	_, a, _, pA, _ := lossPair(t)
	if err := os.RemoveAll(pA); err != nil {
		t.Fatal(err)
	}
	a.use()
	var notes []string
	for i := 0; i < 3; i++ {
		a.svc.background(context.Background(), Hooks{Notify: func(s string) { notes = append(notes, s) }}, i == 1)
	}
	n := 0
	for _, s := range notes {
		if strings.Contains(s, "is not on this computer") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("told %d times: %q", n, notes)
	}
}

// What is left out here is not written over even when the server has a
// newer version of it: a project over the limit and a memory file over the
// limit stay as they are while another computer changes them.
func TestSkip_NewerServerVersionIsNotWrittenOver(t *testing.T) {
	_, a, b, pA, pB := lossPair(t)
	a.use()
	a.writeFile("memory/log.md", "v1\n")
	now(a)
	now(b)
	big := make([]byte, 1500<<10)
	_, _ = rand.Read(big)
	if err := os.WriteFile(filepath.Join(ProjectDir(pA), "dataset.bin"), big, 0o600); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, pA, "EXECAI.md", "build: make\nlocal only edit\n", time.Time{})
	bigNote := strings.Repeat("a line of my long running notes\n", 10000)
	a.use()
	a.writeFile("memory/log.md", bigNote)

	writeProjectFile(t, pB, "EXECAI.md", "build: make\nfrom B\n", time.Time{})
	b.use()
	b.writeFile("memory/log.md", "v2 from B\n")
	now(b)
	now(a)
	if got := readProjectFile(pA, "EXECAI.md"); got != "build: make\nlocal only edit\n" {
		t.Errorf("a project left out was written over: %q", got)
	}
	if _, err := os.Stat(filepath.Join(ProjectDir(pA), "dataset.bin")); err != nil {
		t.Errorf("dataset.bin: %v", err)
	}
	a.use()
	if got := a.readFile("memory/log.md"); got != bigNote {
		t.Errorf("a memory file left out was written over: %d bytes", len(got))
	}
}
