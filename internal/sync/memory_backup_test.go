package sync

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// expandHome — a path from a message (~/…) as a path on disk.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		return home + p[1:]
	}
	return p
}

// conflictCopies — every permanent copy under <config>/sync-unsent: path →
// content.
func (s *side) conflictCopies() map[string]string {
	s.t.Helper()
	out := map[string]string{}
	root := filepath.Join(s.eng.Paths.ConfigDir, "sync-unsent")
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		out[p] = string(b)
		return nil
	})
	return out
}

// hasCopy — a copy with this content exists, outside the memory directory.
func (s *side) hasCopy(content string) (string, bool) {
	for p, c := range s.conflictCopies() {
		if c == content && !strings.HasPrefix(p, s.eng.Paths.MemoryDir()+string(filepath.Separator)) {
			return p, true
		}
	}
	return "", false
}

// A memory file changed on both computers: the server's version stays —
// whichever side changed later, whether the conflict shows at pull or at a
// refused push — and this side's is kept as a copy in
// <config>/sync-conflicts. The copy is not memory: it never travels.
func TestEngine_MemoryBothChangedServerStays(t *testing.T) {
	cases := []struct {
		name         string
		aTime, bTime time.Time
		pushOnly     bool
	}{
		{"pull, this side later", past, future, false},
		{"pull, the other side later", future, past, false},
		{"push refused, this side later", past, future, true},
		{"push refused, the other side later", future, past, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := newStub(t)
			key := mustKey(t)
			a, b := newSide(t, stub, key), newSide(t, stub, key)
			a.write(a.mem("notes/fact.md"), "v1\n", time.Time{})
			a.run(Options{})
			b.run(Options{})

			a.write(a.mem("notes/fact.md"), "from A\n", c.aTime)
			a.run(Options{PushOnly: true})
			b.write(b.mem("notes/fact.md"), "from B\n", c.bTime)
			// A refused send-only round takes nothing; the full one decides
			// (threeway.go, row 4) — the times decide nothing.
			rep := b.run(Options{PushOnly: c.pushOnly})
			if c.pushOnly {
				if rep.Pushed != 0 || b.read(b.mem("notes/fact.md")) != "from B\n" {
					t.Fatalf("a refused send-only round took or sent something: %+v", rep)
				}
				rep = b.run(Options{})
			}
			if rep.Conflicts == 0 {
				t.Fatalf("no conflict seen: %+v", rep)
			}
			if got := b.read(b.mem("notes/fact.md")); got != "from A\n" {
				t.Fatalf("memory file = %q, want the server's", got)
			}
			p, ok := b.hasCopy("from B\n")
			if !ok {
				t.Fatalf("this side's version is gone; copies: %v", b.conflictCopies())
			}
			if got := copiesOf(rep, KindMemory, reasonBoth); len(got) != 1 || got[0] != p {
				t.Errorf("report names %v, copy at %s", got, p)
			}
			if base := filepath.Base(p); !strings.HasPrefix(base, "fact.local-") || !strings.HasSuffix(base, ".md") ||
				filepath.Base(filepath.Dir(p)) != "notes" {
				t.Errorf("copy %s does not tell which file it is", p)
			}

			// The copy stays on this computer: nothing new goes up.
			stub.mu.Lock()
			n := len(stub.blobs)
			stub.mu.Unlock()
			b.run(Options{})
			stub.mu.Lock()
			defer stub.mu.Unlock()
			if len(stub.blobs) != n {
				t.Errorf("blobs on the server: %d → %d — the copy travelled", n, len(stub.blobs))
			}
		})
	}
}

// Deleted on another computer while changed here and not sent (row 4a):
// the content the server never saw stays here and goes up again; the
// person is told.
func TestEngine_MemoryEditMeetsADeletion(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.write(a.mem("fact.md"), "v1\n", time.Time{})
	a.write(a.mem("other.md"), "stays\n", time.Time{})
	a.run(Options{})
	b.run(Options{})

	b.write(b.mem("fact.md"), "edited here, not sent yet\n", past)
	_ = os.Remove(a.mem("fact.md"))
	a.run(Options{})
	rep := b.run(Options{})
	if got := b.read(b.mem("fact.md")); got != "edited here, not sent yet\n" {
		t.Fatalf("the edit made here is gone: %q", got)
	}
	if len(rep.Resent) != 1 {
		t.Errorf("not told: %+v", rep.Resent)
	}
	a.run(Options{})
	if got := a.read(a.mem("fact.md")); got != "edited here, not sent yet\n" {
		t.Errorf("A did not get the edit back: %q", got)
	}
	_ = stub
}

// The index MEMORY.md merges, it does not lose: no copy for it.
func TestEngine_IndexMergeMakesNoCopy(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.write(a.mem("one.md"), "1\n", time.Time{})
	a.write(a.mem("MEMORY.md"), "- [One](one.md)\n", time.Time{})
	a.run(Options{})
	b.run(Options{})
	a.write(a.mem("a.md"), "A\n", time.Time{})
	a.write(a.mem("MEMORY.md"), "- [One](one.md)\n- [From A](a.md)\n", past)
	a.run(Options{PushOnly: true})
	b.write(b.mem("b.md"), "B\n", time.Time{})
	b.write(b.mem("MEMORY.md"), "- [One](one.md)\n- [From B](b.md)\n", future)
	rep := b.run(Options{})
	if len(copiesOf(rep, KindMemory)) != 0 || len(b.conflictCopies()) != 0 {
		t.Errorf("copies for a merged index: %v", b.conflictCopies())
	}
	if got := b.read(b.mem("MEMORY.md")); got != "- [One](one.md)\n- [From A](a.md)\n- [From B](b.md)\n" {
		t.Errorf("merged index %q", got)
	}
}

// Permanent copies stay in sync-unsent however old (sync never removes
// them, nor what older agents left in sync-conflicts). The same content of
// the same file is not copied again on any day — the copy there is named
// instead, so the person is still told; a different one in the same
// second gets its own name.
func TestMemoryBackup_KeptForGoodAndNotRepeated(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 10, 3, 0, time.Local)
	root := filepath.Join(t.TempDir(), "sync-unsent")
	day := func(d time.Time) string { return filepath.Join(root, d.Format("2006-01-02")) }
	old := day(now.AddDate(-1, 0, 0))
	for _, d := range []string{old, filepath.Join(root, "keep-me")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "x.local-000000.md"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(old, "fact.local-090000.md"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p, created, err := keepUnsentCopy(root, now, "fact.md", []byte("one\n")); err != nil || created || p != filepath.Join(old, "fact.local-090000.md") {
		t.Fatalf("the same content copied again (a year later), or the copy there not named: %q %v %v", p, created, err)
	}
	want := filepath.Join(day(now), "fact.local-121003.md")
	if p, created, err := keepUnsentCopy(root, now, "fact.md", []byte("two\n")); err != nil || !created || p != want {
		t.Fatalf("copy %q, want %s (%v)", p, want, err)
	}
	if fi, err := os.Stat(want); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("copy mode: %v %v", fi, err)
	}
	if p, _, _ := keepUnsentCopy(root, now, "fact.md", []byte("three\n")); p != filepath.Join(day(now), "fact.local-121003-2.md") {
		t.Errorf("second version in the same second: %s", p)
	}
	if p, _, _ := keepUnsentCopy(root, now, "README", []byte("no extension\n")); filepath.Base(p) != "README.local-121003" {
		t.Errorf("no extension: %s", p)
	}
	if !fileExists(old) || !fileExists(filepath.Join(root, "keep-me")) {
		t.Error("an old day or an unrelated entry was removed")
	}
	if n := len(conflictCopies(root)); n != 6 {
		t.Errorf("copies listed: %d, want 6", n)
	}
}

// What the person sees: the running agent says once which file lost and
// where its copy is; /sync status counts the copies and names the folder.
func TestMemoryBackup_ToldToThePerson(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)

	a.use()
	a.writeFile("memory/fact.md", "v1\n")
	now(a)
	now(b)
	a.use()
	a.writeFile("memory/fact.md", "from A\n")
	now(a)
	b.use()
	b.writeFile("memory/fact.md", "from B, not sent yet\n")

	var notes []string
	b.svc.background(context.Background(), Hooks{Notify: func(s string) { notes = append(notes, s) }}, false)
	if b.readFile("memory/fact.md") != "from A\n" {
		t.Fatalf("the server's version is not here: %q", b.readFile("memory/fact.md"))
	}
	var copyPath string
	for _, n := range notes {
		if strings.Contains(n, "A memory file was changed here and on the server") {
			copyPath = expandHome(n[strings.Index(n, "is kept: ")+len("is kept: ") : strings.LastIndex(n, ". To send")])
		}
	}
	if copyPath == "" || len(notes) != 1 {
		t.Fatalf("notes: %q", notes)
	}
	if got, err := os.ReadFile(copyPath); err != nil || string(got) != "from B, not sent yet\n" {
		t.Fatalf("copy %s: %q %v", copyPath, got, err)
	}
	if !strings.HasPrefix(copyPath, filepath.Join(b.dir(), "sync-unsent")+string(filepath.Separator)) {
		t.Errorf("copy outside <config>/sync-unsent: %s", copyPath)
	}
	st := b.svc.Status()
	if !strings.Contains(st, "Copies of memory and settings content that was not sent: 1") || !strings.Contains(st, homePath(filepath.Join(b.dir(), "sync-unsent"))) {
		t.Errorf("status:\n%s", st)
	}
}
