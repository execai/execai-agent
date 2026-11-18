package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// The findings of the second review of the loss fix (Q1…Q13), each as a
// test of what must hold; red on the candidate the review read (79e637e),
// green now.

// chatTemplate — the memory index template the chat writes (agent.
// EnsureUserMemoryIndex).
const chatTemplate = "<!-- Индекс твоей памяти. Одна строка на файл. Формат:\n     - [Заголовок](slug.md) — короткий хук (что там и когда пригодится)\n     Тип и содержимое живут внутри каждого файла (frontmatter). -->\n\n"

// memPair — A and B with sync on and the same memory: an index with two
// entries and both notes.
func memPair(t *testing.T) (*stubServer, *machine, *machine) {
	t.Helper()
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	a.use()
	a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Fb](fb.md)\n")
	a.writeFile("memory/role.md", "Go engineer\n")
	a.writeFile("memory/fb.md", "short answers\n")
	now(a)
	now(b)
	return stub, a, b
}

// Q1: the index deleted alone, and the template written over its place
// before the exchange (an older chat's start): an index without entries
// where the base had them is «the index is gone» — taken back from the
// server, never sent.
func TestReview2_Q1_IndexDeletedTemplateWritten(t *testing.T) {
	stub, a, b := memPair(t)
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	ver := stub.blob(KindMemory, memoryName(key, "MEMORY.md")).version
	_ = os.Remove(filepath.Join(a.dir(), "memory", "MEMORY.md"))
	a.writeFile("memory/MEMORY.md", chatTemplate)
	now(a)
	now(b)
	a.use()
	idxA := a.readFile("memory/MEMORY.md")
	b.use()
	idxB := b.readFile("memory/MEMORY.md")
	if !strings.Contains(idxA, "[Role](role.md)") || !strings.Contains(idxB, "[Role](role.md)") {
		t.Errorf("LOSS: the index entries are gone: A %q, B %q", idxA, idxB)
	}
	if v := stub.blob(KindMemory, memoryName(key, "MEMORY.md")).version; v != ver {
		t.Errorf("the template went up (version %d → %d)", ver, v)
	}
}

// Q1b: every note and the index deleted, the template written: nothing is
// deleted anywhere, everything comes back.
func TestReview2_Q1b_AllDeletedTemplateWritten(t *testing.T) {
	stub, a, b := memPair(t)
	a.use()
	for _, n := range []string{"MEMORY.md", "role.md", "fb.md"} {
		_ = os.Remove(filepath.Join(a.dir(), "memory", n))
	}
	a.writeFile("memory/MEMORY.md", chatTemplate)
	now(a)
	now(b)
	key, _ := crypto.LoadIdentity(a.dir())
	for _, n := range []string{"role.md", "fb.md"} {
		if bl := stub.blob(KindMemory, memoryName(key, n)); bl == nil || bl.deleted {
			t.Errorf("LOSS: %s deleted on the server", n)
		}
		b.use()
		if b.readFile("memory/"+n) == "<missing>" {
			t.Errorf("LOSS: %s removed on B", n)
		}
		a.use()
		if a.readFile("memory/"+n) == "<missing>" {
			t.Errorf("%s did not come back on A", n)
		}
	}
}

// Q2: a project file changed between the scan and the write (the model
// saves EXECAI.md while the exchange runs). The keeper reads it again
// before replacing it: changed — not touched, the base not moved; the next
// exchange sees the edit against the server's version and keeps it.
func TestReview2_Q2_EditDuringExchangeIsNotLost(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	now(a)
	const edit = "build: make\nB: edit saved during the exchange\n"
	stub.mu.Lock()
	stub.onList = func() { writeProjectFile(t, pB, "EXECAI.md", edit, time.Time{}) }
	stub.mu.Unlock()
	if out := now(b); !strings.Contains(out, "changed while sync was working on it, so it is left for the next exchange") {
		t.Errorf("not told the file was left:\n%s", out)
	}
	stub.mu.Lock()
	stub.onList = nil
	stub.mu.Unlock()
	if got := readProjectFile(pB, "EXECAI.md"); got != edit {
		t.Fatalf("REVERT: the edit made during the exchange was replaced: %q", got)
	}
	out := now(b)
	now(a)
	// Both changed since the base: the server's stays, B's edit is kept
	// next to the file and the person told (row 4) — not replaced silently.
	if !hasCopyOf(ProjectDir(pB), edit) || !strings.Contains(out, "changed here and on the server") {
		t.Errorf("B's edit: folder %v; told:\n%s", rvList(ProjectDir(pB)), out)
	}
}

// Q2m: the same for a memory note.
func TestReview2_Q2m_MemoryEditDuringExchangeIsNotLost(t *testing.T) {
	stub, a, b := memPair(t)
	a.use()
	a.writeFile("memory/role.md", "Go engineer, v2 from A\n")
	now(a)
	b.use()
	bfile := filepath.Join(b.dir(), "memory", "role.md")
	const edit = "Go engineer\nB: note saved during the exchange\n"
	stub.mu.Lock()
	stub.onList = func() { _ = os.WriteFile(bfile, []byte(edit), 0o600) }
	stub.mu.Unlock()
	if out := now(b); !strings.Contains(out, "changed while sync was working on it, so it is left for the next exchange") {
		t.Errorf("not told the file was left:\n%s", out)
	}
	stub.mu.Lock()
	stub.onList = nil
	stub.mu.Unlock()
	b.use()
	if got := b.readFile("memory/role.md"); got != edit {
		t.Fatalf("REVERT: the note edited during the exchange was replaced: %q", got)
	}
	out := now(b)
	kept := false
	for _, c := range b.conflictFiles() {
		kept = kept || c == edit
	}
	if !kept || !strings.Contains(out, "changed here and on the server") {
		t.Errorf("B's edit not kept or not told: %v\n%s", b.conflictFiles(), out)
	}
}

// Q3: the process dies in the middle of filling an emptied .execai/ from
// the server (the state not saved). The next exchange must not take the
// half-filled folder for files deleted here.
func TestReview2_Q3_CrashMidRestoreSendsNothing(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	_ = os.Remove(filepath.Join(ProjectDir(pA), "EXECAI.md"))
	_ = os.Remove(filepath.Join(ProjectDir(pA), "notes.md"))
	block := filepath.Join(ProjectDir(pA), "notes.md")
	_ = os.MkdirAll(block, 0o700) // the write stops after EXECAI.md
	a.use()
	saved, _ := os.ReadFile(a.svc.Paths.StateFile())
	_, _ = a.svc.exchange(context.Background(), Options{}, time.Second)
	_ = os.WriteFile(a.svc.Paths.StateFile(), saved, 0o600) // sync.json never saved
	_ = os.Remove(block)
	v := serverVersion(stub)
	now(a)
	now(b)
	if readProjectFile(pB, "notes.md") != "deploy from R7\n" || readProjectFile(pA, "notes.md") != "deploy from R7\n" {
		t.Errorf("LOSS: notes.md: A %q, B %q", readProjectFile(pA, "notes.md"), readProjectFile(pB, "notes.md"))
	}
	if serverVersion(stub) != v {
		t.Error("the half-filled folder was sent")
	}
}

// Q4 (Н-3): the device number changes across a reboot (btrfs, ZFS, disks
// found in another order) while the folder is the same. Work not sent
// before the reboot is not taken for another folder's.
func TestReview2_Q4_DeviceNumberChangeKeepsUnsentWork(t *testing.T) {
	_, a, _, pA, _ := lossPair(t)
	a.use()
	a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n")
	a.writeFile("memory/role.md", "Go engineer\n")
	now(a)
	writeProjectFile(t, pA, "EXECAI.md", "build: make\nunsent edit before reboot\n", time.Time{})
	_ = os.Remove(filepath.Join(ProjectDir(pA), "notes.md"))
	a.writeFile("memory/role.md", "Go engineer\nunsent memory edit\n")
	old := dirIdentityFunc
	dirIdentityFunc = func(p string) string {
		if id := old(p); id != "" {
			return "4242:" + id // the same number and creation time, another device
		}
		return ""
	}
	t.Cleanup(func() { dirIdentityFunc = old })
	now(a)
	a.use()
	if readProjectFile(pA, "EXECAI.md") != "build: make\nunsent edit before reboot\n" {
		t.Errorf("REVERT: the project edit: %q", readProjectFile(pA, "EXECAI.md"))
	}
	if readProjectFile(pA, "notes.md") != "<missing>" {
		t.Error("REVERT: the deleted notes.md came back")
	}
	if a.readFile("memory/role.md") != "Go engineer\nunsent memory edit\n" {
		t.Errorf("REVERT: the memory edit: %q", a.readFile("memory/role.md"))
	}
}

// Q4b: identities written with the device number (dev:ino:btime, the
// format of the earlier candidate) match the same folder read without it,
// and a change of the device part alone is the same folder; another inode
// or creation time is not.
func TestReview2_Q4b_IdentityWithoutTheDevice(t *testing.T) {
	if osName == "windows" {
		t.Skip("the Windows identity has no separate device part")
	}
	cases := []struct {
		a, b string
		same bool
	}{
		{"2049:1234:1700000000.000000001", "1234:1700000000.000000001", true},
		{"64769:1234:1700000000.000000001", "2049:1234:1700000000.000000001", true},
		{"259.3:1234:1700000000.000000001", "1234:1700000000.000000001", true},
		{"2049:1234:1700000000.000000001", "1235:1700000000.000000001", false},
		{"1234:1700000000.000000001", "1234:1700000001.000000001", false},
		{"", "1234:1700000000.000000001", false},
	}
	for _, c := range cases {
		if got := sameIdentity(c.a, c.b); got != c.same {
			t.Errorf("sameIdentity(%q, %q) = %v", c.a, c.b, got)
		}
	}
	if id := dirIdentity(t.TempDir()); id != "" && strings.Count(id, ":") != 1 {
		t.Errorf("an identity carries more than the number and the creation time: %q", id)
	}
}

// Q5: a computer joins with a note of its own whose name matches a note
// deleted long ago elsewhere (a deletion mark): no base here, the note is
// content the server never saw (row 5d) — it stays and goes up again.
func TestReview2_Q5_JoinKeepsOwnNoteOverAnOldDeletion(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	a.use()
	a.writeFile("memory/feedback.md", "A's feedback\n")
	now(a)
	_ = os.Remove(filepath.Join(a.dir(), "memory", "feedback.md"))
	now(a)
	c := newMachine(t, stub, "c")
	c.use()
	c.writeFile("memory/feedback.md", "C's own feedback, never synced\n")
	monthAgo := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	_ = os.Chtimes(filepath.Join(c.dir(), "memory", "feedback.md"), monthAgo, monthAgo)
	text := allText(c.converse(c.svc.OnFlow(), password))
	c.use()
	if c.readFile("memory/feedback.md") != "C's own feedback, never synced\n" {
		t.Fatalf("LOSS: C's own note removed by an old deletion mark: %q", c.readFile("memory/feedback.md"))
	}
	key, _ := crypto.LoadIdentity(a.dir())
	if bl := stub.blob(KindMemory, memoryName(key, "feedback.md")); bl == nil || bl.deleted {
		t.Error("C's note did not go up")
	}
	if !strings.Contains(text, "deleted on the server but changed here") {
		t.Errorf("not told:\n%s", text)
	}
}

// Q6: after another computer kept its own key (choice 2) and deleted what
// it did not have, this computer joins again with the new key: its notes
// are content the new key's server never saw — they stay and go up.
func TestReview2_Q6_OverwriteThenRejoinKeepsNotes(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	a.use()
	a.writeFile("memory/MEMORY.md", "- [A only](a_only.md)\n")
	a.writeFile("memory/a_only.md", "A's note\n")
	monthAgo := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for _, n := range []string{"MEMORY.md", "a_only.md"} {
		_ = os.Chtimes(filepath.Join(a.dir(), "memory", n), monthAgo, monthAgo)
	}
	now(a)
	c := newMachine(t, stub, "c")
	_, _, _ = crypto.LoadOrGenerate(c.dir())
	c.converse(c.svc.OnFlow(), password, "2")
	a.converse(a.svc.OnFlow(), password, "1")
	a.use()
	if a.readFile("memory/a_only.md") != "A's note\n" {
		t.Errorf("LOSS: A's note removed after C kept its key: %q", a.readFile("memory/a_only.md"))
	}
}

// Q7: sync-bases.json deleted, emptied, broken, or written by a newer
// agent: a memory edit not sent yet is not replaced by the server's
// version; a newer agent's file is read and not rewritten.
func TestReview2_Q7_BasesFileLostOrNewer(t *testing.T) {
	for _, how := range []string{"deleted", "empty", "garbage", "newer"} {
		t.Run(how, func(t *testing.T) {
			_, a, _, pA, _ := lossPair(t)
			a.use()
			a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n")
			a.writeFile("memory/role.md", "Go engineer\n")
			now(a)
			a.writeFile("memory/role.md", "Go engineer\nunsent edit\n")
			writeProjectFile(t, pA, "EXECAI.md", "build: make\nunsent project edit\n", time.Time{})
			var newer []byte
			switch how {
			case "deleted":
				_ = os.Remove(a.svc.Paths.BasesFile())
			case "empty":
				_ = os.WriteFile(a.svc.Paths.BasesFile(), nil, 0o600)
			case "garbage":
				_ = os.WriteFile(a.svc.Paths.BasesFile(), []byte(`{"v":2,"projects":`), 0o600)
			case "newer":
				var m map[string]any
				raw, _ := os.ReadFile(a.svc.Paths.BasesFile())
				_ = json.Unmarshal(raw, &m)
				m["v"] = basesVersion + 1
				m["future"] = map[string]any{"x": 1}
				newer, _ = json.Marshal(m)
				_ = os.WriteFile(a.svc.Paths.BasesFile(), newer, 0o600)
			}
			out := now(a)
			a.use()
			if got := a.readFile("memory/role.md"); got != "Go engineer\nunsent edit\n" {
				t.Errorf("REVERT: the memory edit: %q", got)
			}
			if got := readProjectFile(pA, "EXECAI.md"); got != "build: make\nunsent project edit\n" && !hasCopyOf(ProjectDir(pA), "build: make\nunsent project edit\n") {
				t.Errorf("LOSS: the project edit: %q", got)
			}
			if how == "newer" {
				raw, _ := os.ReadFile(a.svc.Paths.BasesFile())
				if string(raw) != string(newer) {
					t.Error("a newer agent's sync-bases.json was rewritten")
				}
				if !strings.Contains(out, "newer version of the agent") {
					t.Errorf("not told to update:\n%s", out)
				}
			}
		})
	}
}

// Q8: no folder identity; the repository cloned again at the same path
// with an older .execai/: the older files are older base versions — the
// server's newer version stays everywhere.
func TestReview2_Q8_NoIdentityRecloneDoesNotSendOlder(t *testing.T) {
	noIdentity(t)
	stub, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pB, "EXECAI.md", "build: make\nnewest, from B\n", time.Time{})
	now(b)
	now(a)
	_ = os.RemoveAll(pA)
	writeProjectFile(t, pA, "EXECAI.md", "build: make\n", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	writeProjectFile(t, pA, "notes.md", "deploy from R7\n", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	v := serverVersion(stub)
	now(a)
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make\nnewest, from B\n" || readProjectFile(pA, "EXECAI.md") != "build: make\nnewest, from B\n" {
		t.Errorf("REVERT: B %q, A %q", readProjectFile(pB, "EXECAI.md"), readProjectFile(pA, "EXECAI.md"))
	}
	if serverVersion(stub) != v {
		t.Error("the older clone was sent")
	}
}

// Q9: no folder identity; the memory folder restored from a backup that
// holds the hidden marker of earlier candidates: no newer note is deleted
// anywhere, no edit taken back.
func TestReview2_Q9_NoIdentityMemoryRestoredFromBackup(t *testing.T) {
	noIdentity(t)
	_, a, b := memPair(t)
	a.use()
	_ = os.WriteFile(filepath.Join(a.dir(), "memory", ".sync-folder"), []byte("marker of an earlier candidate\n"), 0o600)
	backup := filepath.Join(t.TempDir(), "memory-backup")
	if out, err := exec.Command("cp", "-a", filepath.Join(a.dir(), "memory"), backup).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v %s", err, out)
	}
	a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Fb](fb.md)\n- [Plan](plan.md)\n")
	a.writeFile("memory/plan.md", "the plan, written after the backup\n")
	a.writeFile("memory/role.md", "Go engineer, newer\n")
	now(a)
	now(b)
	a.use()
	_ = os.RemoveAll(filepath.Join(a.dir(), "memory"))
	if out, err := exec.Command("cp", "-a", backup, filepath.Join(a.dir(), "memory")).CombinedOutput(); err != nil {
		t.Fatalf("cp back: %v %s", err, out)
	}
	now(a)
	now(b)
	for _, m := range []*machine{a, b} {
		m.use()
		if m.readFile("memory/plan.md") != "the plan, written after the backup\n" || m.readFile("memory/role.md") != "Go engineer, newer\n" ||
			!strings.Contains(m.readFile("memory/MEMORY.md"), "[Plan](plan.md)") {
			t.Errorf("LOSS/REVERT on %s: plan %q, role %q, index %q", m.name, m.readFile("memory/plan.md"),
				m.readFile("memory/role.md"), m.readFile("memory/MEMORY.md"))
		}
	}
}

// Q10 (Н-4): two projects bound to one folder (a binding made elsewhere):
// the exchange keeps the folder to the project whose base was agreed in it
// and leaves the other out — their memories do not mix on any computer.
func TestReview2_Q10_TwoProjectsOneFolder(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	pB2 := t.TempDir()
	writeProjectFile(t, pB2, "EXECAI.md", "beta project memory\n", time.Time{})
	stub.mu.Lock()
	stub.workspaces = append(stub.workspaces, Workspace{ID: "ws-2", Name: "Beta"})
	stub.bindings = append(stub.bindings, Binding{AgentID: "agent-b", WorkspaceID: "ws-2", LocalPath: pB2})
	stub.mu.Unlock()
	now(b)
	stub.mu.Lock()
	stub.bindings = append(stub.bindings, Binding{AgentID: "agent-a", WorkspaceID: "ws-2", LocalPath: pA})
	stub.mu.Unlock()
	var outA string
	for i := 0; i < 2; i++ {
		outA += now(a)
		now(b)
	}
	if readProjectFile(pB, "EXECAI.md") != "build: make\n" || readProjectFile(pB2, "EXECAI.md") != "beta project memory\n" ||
		readProjectFile(pA, "EXECAI.md") != "build: make\n" {
		t.Errorf("MIX: Alpha on B %q, Beta on B %q, A's folder %q", readProjectFile(pB, "EXECAI.md"),
			readProjectFile(pB2, "EXECAI.md"), readProjectFile(pA, "EXECAI.md"))
	}
	if !strings.Contains(outA, "is already bound to project \"Alpha\"") {
		t.Errorf("not told:\n%s", outA)
	}
}

// Q12: the process dies after writing the server's version over an
// unchanged folder, before the state was saved; the person edits a file
// before the next exchange. Against the base still on disk the file
// changed here and on the server (row 4): the server's version stays and
// the edit is kept next to the file, and the person is told — never
// replaced silently (with the applying mark gone nothing else tells that
// the folder was written just before the crash; Возражения in the report).
func TestReview2_Q12_EditAfterACrashIsAnEdit(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	for i, both := range []bool{false, true} {
		next := fmt.Sprintf("build: make all, round %d\n", i)
		writeProjectFile(t, pA, "EXECAI.md", next, time.Time{})
		now(a)
		b.use()
		savedState, _ := os.ReadFile(b.svc.Paths.StateFile())
		savedBases, _ := os.ReadFile(b.svc.Paths.BasesFile())
		now(b)                                                       // writes A's version …
		_ = os.WriteFile(b.svc.Paths.StateFile(), savedState, 0o600) // … sync.json is lost
		if both {
			_ = os.WriteFile(b.svc.Paths.BasesFile(), savedBases, 0o600) // … and sync-bases.json
		} else {
			// … and the earlier candidate's applying mark stays: read and
			// not needed any more.
			var m map[string]any
			raw, _ := os.ReadFile(b.svc.Paths.BasesFile())
			_ = json.Unmarshal(raw, &m)
			m["applying"] = map[string]any{"ws-1": map[string]any{"version": serverVersion(stub)}}
			raw, _ = json.Marshal(m)
			_ = os.WriteFile(b.svc.Paths.BasesFile(), raw, 0o600)
		}
		edit := next + "B: edit after the restart\n"
		writeProjectFile(t, pB, "EXECAI.md", edit, time.Time{})
		out := now(b)
		now(a)
		inPlace := readProjectFile(pB, "EXECAI.md") == edit && readProjectFile(pA, "EXECAI.md") == edit
		kept := hasCopyOf(ProjectDir(pB), edit) && strings.Contains(out, "this computer's is kept: ")
		if !inPlace && !kept {
			t.Errorf("REVERT (bases lost too: %v): B %q, A %q, copies %v\n%s", both, readProjectFile(pB, "EXECAI.md"),
				readProjectFile(pA, "EXECAI.md"), copiesIn(ProjectDir(pB)), out)
		}
		for name := range copiesIn(ProjectDir(pB)) {
			_ = os.Remove(filepath.Join(ProjectDir(pB), name))
		}
	}
}

// Q13: copies an older agent left next to project files, and in
// sync-conflicts — the only copy of a losing side — are never removed,
// however old their names say they are.
func TestReview2_Q13_OlderAgentsCopiesStay(t *testing.T) {
	_, a, _, pA, _ := lossPair(t)
	old := filepath.Join(ProjectDir(pA), "EXECAI.md.local-20260901-120000")
	if err := os.WriteFile(old, []byte("the losing side of a 7.15 conflict\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := filepath.Join(ProjectDir(pA), "notes.md.server-20250101-000000")
	_ = os.WriteFile(server, []byte("server side of 2025\n"), 0o600)
	a.use()
	memCopy := filepath.Join(a.dir(), "sync-conflicts", "2025-01-01", "fact.local-120000.md")
	_ = os.MkdirAll(filepath.Dir(memCopy), 0o700)
	_ = os.WriteFile(memCopy, []byte("an old memory copy\n"), 0o600)
	for i := 0; i < 3; i++ {
		now(a)
	}
	for _, p := range []string{old, server, memCopy} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("LOSS: %s removed", p)
		}
	}
}
