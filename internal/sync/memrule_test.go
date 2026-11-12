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

// The table of rule П5 (keep.go, memMode): what a missing personal memory
// file means, by the memory folder and the index, in a full exchange and
// in a send-only one first.
func TestMemoryRule_Table(t *testing.T) {
	notes := []string{"role.md", "fb.md", "plan.md"}
	cases := []struct {
		name string
		// change — what happens on A between exchanges.
		change func(t *testing.T, a *machine)
		// gone — notes deleted everywhere afterwards; the rest is
		// everywhere.
		gone []string
		// indexBack — MEMORY.md is on A afterwards.
		indexBack bool
	}{
		{"one note deleted, index kept", func(t *testing.T, a *machine) {
			rm(t, a, "role.md")
		}, []string{"role.md"}, true},
		{"several notes deleted", func(t *testing.T, a *machine) {
			rm(t, a, "role.md", "fb.md")
		}, []string{"role.md", "fb.md"}, true},
		{"all notes deleted, index kept", func(t *testing.T, a *machine) {
			rm(t, a, notes...)
		}, notes, true},
		{"all notes and the index deleted in the same folder", func(t *testing.T, a *machine) {
			rm(t, a, append([]string{"MEMORY.md"}, notes...)...)
		}, nil, true},
		{"the index deleted alone", func(t *testing.T, a *machine) {
			rm(t, a, "MEMORY.md")
		}, nil, true},
		{"the memory folder removed", func(t *testing.T, a *machine) {
			_ = os.RemoveAll(filepath.Join(a.dir(), "memory"))
		}, nil, true},
		{"the memory folder made again with the chat's template", func(t *testing.T, a *machine) {
			_ = os.RemoveAll(filepath.Join(a.dir(), "memory"))
			a.writeFile("memory/MEMORY.md", "<!-- index template\n     - [Title](slug.md) — hook -->\n\n")
		}, nil, true},
		{"the memory folder replaced by an older copy of itself", func(t *testing.T, a *machine) {
			old := filepath.Join(t.TempDir(), "old")
			_ = os.MkdirAll(old, 0o700)
			_ = os.WriteFile(filepath.Join(old, "MEMORY.md"), []byte("- [Role](role.md)\n"), 0o600)
			_ = os.WriteFile(filepath.Join(old, "role.md"), []byte("role\n"), 0o600)
			_ = os.RemoveAll(filepath.Join(a.dir(), "memory"))
			if err := os.Rename(old, filepath.Join(a.dir(), "memory")); err != nil {
				t.Fatal(err)
			}
		}, nil, true},
		{"state of an older agent (no record), one note deleted", func(t *testing.T, a *machine) {
			_ = os.Remove(a.svc.Paths.BasesFile())
			rm(t, a, "role.md")
		}, nil, true},
	}
	for _, c := range cases {
		for _, sendOnlyFirst := range []bool{false, true} {
			name := c.name + map[bool]string{false: " / full first", true: " / send-only first"}[sendOnlyFirst]
			t.Run(name, func(t *testing.T) {
				i18n.SetLocale("en")
				stub := newStub(t)
				a := newMachine(t, stub, "a")
				a.converse(a.svc.OnFlow(), password, password)
				b := newMachine(t, stub, "b")
				b.converse(b.svc.OnFlow(), password)
				a.use()
				a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Fb](fb.md)\n- [Plan](plan.md)\n")
				for _, n := range notes {
					a.writeFile("memory/"+n, strings.TrimSuffix(n, ".md")+"\n")
				}
				now(a)
				now(b)
				a.use()
				c.change(t, a)
				if sendOnlyFirst {
					if _, err := a.svc.exchange(context.Background(), Options{PushOnly: true}, time.Second); err != nil {
						t.Fatal(err)
					}
				}
				for i := 0; i < 3; i++ {
					now(a)
					now(b)
				}
				key, _ := crypto.LoadIdentity(a.dir())
				gone := map[string]bool{}
				for _, n := range c.gone {
					gone[n] = true
				}
				for _, n := range notes {
					bl := stub.blob(KindMemory, memoryName(key, n))
					onServer := bl != nil && !bl.deleted
					a.use()
					onA := a.readFile("memory/"+n) != "<missing>"
					b.use()
					onB := b.readFile("memory/"+n) != "<missing>"
					if gone[n] && (onServer || onA || onB) {
						t.Errorf("%s deleted on A came back: server %v, A %v, B %v", n, onServer, onA, onB)
					}
					if !gone[n] && !(onServer && onA && onB) {
						t.Errorf("LOSS: %s gone: server %v, A %v, B %v", n, onServer, onA, onB)
					}
					if gone[n] && onB {
						continue
					}
					if gone[n] {
						// The server had B's version: it goes to the history
						// (row 2), silently.
						b.use()
						if len(historyVersions(filepath.Join(b.svc.Paths.HistoryDir(), "memory"), n)) == 0 {
							t.Errorf("B removed %s without a copy in the history", n)
						}
					}
				}
				a.use()
				idx := a.readFile("memory/MEMORY.md")
				if c.indexBack && idx == "<missing>" {
					t.Error("the index did not come back")
				}
				b.use()
				if bi := b.readFile("memory/MEMORY.md"); strings.Contains(bi, "template") || strings.Contains(bi, "<!--") || bi == "<missing>" {
					t.Errorf("B's index: %q", bi)
				}
			})
		}
	}
}

func rm(t *testing.T, m *machine, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.Remove(filepath.Join(m.dir(), "memory", n)); err != nil {
			t.Fatal(err)
		}
	}
}

// R13 of the review: the only note deleted on purpose while no agent ran
// stays deleted — the next start's full exchange does not bring it back.
func TestMemoryRule_OnlyNoteDeletedStaysDeleted(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	a.use()
	a.writeFile("memory/MEMORY.md", "- [Secret](secret.md)\n")
	a.writeFile("memory/secret.md", "my bank pin hint\n")
	now(a)
	a.use()
	_ = os.Remove(filepath.Join(a.dir(), "memory", "secret.md"))
	out := now(a)
	if a.readFile("memory/secret.md") != "<missing>" {
		t.Errorf("RESURRECTED: the only note deleted on purpose came back; said:\n%s", out)
	}
	key, _ := crypto.LoadIdentity(a.dir())
	if bl := stub.blob(KindMemory, memoryName(key, "secret.md")); bl == nil || !bl.deleted {
		t.Error("the deletion did not reach the server")
	}
}

// R11 of the review: the template the chat writes into a new memory folder
// is not merged into the index of every computer.
func TestMemoryRule_TemplateIndexDoesNotTravel(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	a.use()
	a.writeFile("memory/MEMORY.md", "- [Role](user_role.md) — who I am\n")
	a.writeFile("memory/user_role.md", "Go engineer\n")
	now(a)
	now(b)
	a.use()
	_ = os.RemoveAll(filepath.Join(a.dir(), "memory"))
	a.writeFile("memory/MEMORY.md", "<!-- Индекс твоей памяти. Одна строка на файл. Формат:\n     - [Заголовок](slug.md) — короткий хук\n     Тип и содержимое живут внутри каждого файла (frontmatter). -->\n\n")
	now(a)
	now(b)
	for name, m := range map[string]*machine{"A": a, "B": b} {
		m.use()
		if got := m.readFile("memory/MEMORY.md"); got != "- [Role](user_role.md) — who I am\n" {
			t.Errorf("%s's index: %q", name, got)
		}
	}
}

// A memory file here with no base that differs from the server's (a
// computer joining with notes of its own): the server's version is used,
// this one kept, and the person told so — without claiming another
// computer changed it.
func TestMemoryRule_NoBaseTakesTheServersVersion(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	a.use()
	a.writeFile("memory/fact.md", "from A\n")
	now(a)
	b := newMachine(t, stub, "b")
	b.writeFile("memory/fact.md", "from B\n")
	text := allText(b.converse(b.svc.OnFlow(), password))
	b.use()
	if b.readFile("memory/fact.md") != "from A\n" {
		t.Fatalf("B's fact.md = %q", b.readFile("memory/fact.md"))
	}
	if !strings.Contains(text, "there was no agreed earlier version to compare with: the server's version is used here") ||
		!strings.Contains(text, "fact.local-") {
		t.Errorf("not told: %s", text)
	}
	found := false
	_ = filepath.WalkDir(filepath.Join(b.dir(), "sync-unsent"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(p)
			found = found || string(data) == "from B\n"
		}
		return nil
	})
	if !found {
		t.Error("B's own version is not kept")
	}
}
