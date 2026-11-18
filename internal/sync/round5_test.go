package sync

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Round 5 of the E105 loss review: the index keeps every line of text, a
// left-out name keeps its base, the agent's own model lists are not a
// settings change, an older snapshot of a project keeps what it lacks,
// copies are private.

// filesIn — every file under dir: path → content.
func filesIn(dir string) map[string]string {
	out := map[string]string{}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(p)
			out[p] = string(data)
		}
		return nil
	})
	return out
}

func anyHolds(files map[string]string, sub string) bool {
	for _, c := range files {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func historyOf(m *machine) map[string]string {
	m.use()
	return filesIn(m.svc.Paths.HistoryDir())
}

// ===== a name left out keeps its base (x11) =====

func lockOut(t *testing.T, file string) (restore func()) {
	t.Helper()
	if err := os.Chmod(file, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(file, 0o600) })
	return func() { _ = os.Chmod(file, 0o600) }
}

func TestRound5_LeftOutNameKeepsItsBase(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("needs a file that cannot be read")
	}
	t.Run("an edit made before goes up once it can be read (3a)", func(t *testing.T) {
		stub, a, b, pA, pB := lossPair(t)
		writeProjectFile(t, pA, "notes.md", "deploy from R7\nA's edit\n", time.Time{})
		restore := lockOut(t, filepath.Join(ProjectDir(pA), "notes.md"))
		now(a)
		restore()
		out := now(a)
		key, _ := crypto.LoadIdentity(a.dir())
		if got := serverPack(t, stub, key, "ws-1")["notes.md"]; got != "deploy from R7\nA's edit\n" {
			t.Errorf("the edit did not go up: %q\n%s", got, out)
		}
		if c := copiesIn(ProjectDir(pA)); len(c) != 0 || strings.Contains(out, "no agreed earlier version") {
			t.Errorf("a copy or a line for an ordinary edit: %v\n%s", c, out)
		}
		now(b)
		if got := readProjectFile(pB, "notes.md"); got != "deploy from R7\nA's edit\n" {
			t.Errorf("B: %q", got)
		}
		assertI1(t, stub, a)
		assertI1(t, stub, b)
	})
	t.Run("a file not touched here takes the newer version (2a)", func(t *testing.T) {
		stub, a, b, pA, pB := lossPair(t)
		restore := lockOut(t, filepath.Join(ProjectDir(pA), "notes.md"))
		now(a)
		writeProjectFile(t, pB, "notes.md", "deploy from R8\n", time.Time{})
		now(b)
		now(a)
		restore()
		out := now(a)
		if got := readProjectFile(pA, "notes.md"); got != "deploy from R8\n" {
			t.Errorf("A: %q", got)
		}
		if c := copiesIn(ProjectDir(pA)); len(c) != 0 || strings.Contains(out, "kept") {
			t.Errorf("an untouched file was taken for unsent content: %v\n%s", c, out)
		}
		if !anyHolds(historyOf(a), "deploy from R7\n") {
			t.Error("the replaced version is not in the history")
		}
		assertI1(t, stub, a)
	})
	t.Run("a deletion elsewhere removes it here, said (2c)", func(t *testing.T) {
		stub, a, b, pA, pB := lossPair(t)
		restore := lockOut(t, filepath.Join(ProjectDir(pA), "notes.md"))
		now(a)
		if err := os.Remove(filepath.Join(ProjectDir(pB), "notes.md")); err != nil {
			t.Fatal(err)
		}
		now(b)
		now(a)
		assertI1(t, stub, a) // still left out: its base is a version the server held
		restore()
		out := now(a)
		key, _ := crypto.LoadIdentity(a.dir())
		if _, ok := serverPack(t, stub, key, "ws-1")["notes.md"]; ok {
			t.Errorf("the deletion on B was undone: the file went up again\n%s", out)
		}
		if _, err := os.Stat(filepath.Join(ProjectDir(pA), "notes.md")); err == nil {
			t.Errorf("A still has the file deleted on B\n%s", out)
		}
		if !strings.Contains(out, "not in the version that came from the server") || !anyHolds(historyOf(a), "deploy from R7\n") {
			t.Errorf("not said, or no history:\n%s", out)
		}
		assertI1(t, stub, a)
	})
	t.Run("changed on both sides meanwhile: row 4, said as such", func(t *testing.T) {
		_, a, b, pA, pB := lossPair(t)
		writeProjectFile(t, pA, "notes.md", "deploy from R7\nA's edit\n", time.Time{})
		restore := lockOut(t, filepath.Join(ProjectDir(pA), "notes.md"))
		now(a)
		writeProjectFile(t, pB, "notes.md", "deploy from R8\n", time.Time{})
		now(b)
		restore()
		out := now(a)
		if got := readProjectFile(pA, "notes.md"); got != "deploy from R8\n" {
			t.Errorf("A: %q", got)
		}
		if !hasCopyOf(ProjectDir(pA), "deploy from R7\nA's edit\n") || !strings.Contains(out, "changed here and on the server") {
			t.Errorf("A's edit not kept, or said as something else:\n%s", out)
		}
	})
}

// ===== the agent's own model lists are not a settings change (P7) =====

const (
	subsBase    = `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"k-zai-test"}}}`
	subsRefresh = `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"k-zai-test","base_url":"https://api.z.ai/api/anthropic",` +
		`"available_models":["glm-5"],"models":[{"id":"glm-5"}],"models_fetched_at":"2026-11-11T10:00:00Z"}}}`
)

func settingsPair(t *testing.T, subs string) (*stubServer, *machine, *machine) {
	t.Helper()
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.writeFile("config.json", `{"api_base":"https://api.a","model":"glm"}`)
	a.writeFile("subscriptions.json", subs)
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	b.use()
	if !strings.Contains(b.readFile("subscriptions.json"), "k-zai-test") {
		t.Fatalf("setup: B has %q", b.readFile("subscriptions.json"))
	}
	return stub, a, b
}

func TestRound5_ModelListRefreshIsNotASettingsChange(t *testing.T) {
	t.Run("a refresh alone does not go up", func(t *testing.T) {
		stub, _, b := settingsPair(t, subsBase)
		before := stub.blob(KindConfig, NameConfig).version
		b.use()
		b.writeFile("subscriptions.json", subsRefresh)
		now(b)
		if after := stub.blob(KindConfig, NameConfig).version; after != before {
			t.Errorf("the agent's own model list went up as a change (v%d → v%d)", before, after)
		}
	})
	t.Run("a key added here stays and goes up", func(t *testing.T) {
		_, a, b := settingsPair(t, subsBase)
		b.use()
		b.writeFile("subscriptions.json", subsRefresh)
		a.use()
		a.writeFile("subscriptions.json", `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"k-zai-test"},"kimi":{"provider":"kimi","api_key":"k-kimi-new-on-A"}}}`)
		now(b)
		out := now(a)
		a.use()
		if !strings.Contains(a.readFile("subscriptions.json"), "k-kimi-new-on-A") || len(a.conflictFiles()) != 0 {
			t.Errorf("A's new key left the settings: %q, aside %v\n%s", a.readFile("subscriptions.json"), a.conflictFiles(), out)
		}
		now(b)
		b.use()
		if !strings.Contains(b.readFile("subscriptions.json"), "k-kimi-new-on-A") {
			t.Errorf("B did not get the key: %q", b.readFile("subscriptions.json"))
		}
	})
	t.Run("a key removed here does not come back", func(t *testing.T) {
		both := `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"k-zai-test"},"kimi":{"provider":"kimi","api_key":"k-kimi"}}}`
		_, a, b := settingsPair(t, both)
		b.use()
		b.writeFile("subscriptions.json", strings.Replace(both, `"api_key":"k-kimi"}`, `"api_key":"k-kimi","models_fetched_at":"2026-11-11T10:00:00Z"}`, 1))
		now(b)
		a.use()
		a.writeFile("subscriptions.json", subsBase)
		now(a)
		now(b)
		for _, m := range []*machine{a, b} {
			m.use()
			if strings.Contains(m.readFile("subscriptions.json"), "k-kimi") {
				t.Errorf("%s: the removed key is back: %q", m.name, m.readFile("subscriptions.json"))
			}
		}
	})
}

// ===== the memory index keeps every line of text (Б-Т1, D, Н-4) =====

const (
	rulesSection = "## Rules I wrote by hand\nNever deploy on Fridays.\n"
	idxLines     = "- [Role](role.md)\n- [Fb](fb.md)\n"
)

func indexPair(t *testing.T, idx string) (*stubServer, *machine, *machine) {
	t.Helper()
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	a.use()
	a.writeFile("memory/role.md", "Go engineer\n")
	a.writeFile("memory/fb.md", "short answers\n")
	a.writeFile("memory/MEMORY.md", idx)
	now(a)
	now(b)
	b.use()
	if got := b.readFile("memory/MEMORY.md"); got != idx {
		t.Fatalf("setup: B's index %q", got)
	}
	return stub, a, b
}

func TestRound5_IndexTextOutsideEntries(t *testing.T) {
	where := map[string]string{
		"at the beginning": rulesSection + "\n" + idxLines,
		"in the middle":    "- [Role](role.md)\n" + rulesSection + "- [Fb](fb.md)\n",
		"at the end":       idxLines + "\n" + rulesSection,
	}
	for at, idx := range where {
		t.Run(at+": the model rewrote the index on another computer", func(t *testing.T) {
			stub, a, b := indexPair(t, idx)
			a.use()
			a.writeFile("memory/MEMORY.md", idxLines)
			now(a)
			out := now(b)
			b.use()
			got := b.readFile("memory/MEMORY.md")
			if !strings.Contains(got, "](role.md)") || !strings.Contains(got, "](fb.md)") {
				t.Errorf("B's index lost a note line: %q", got)
			}
			if !strings.Contains(got, "Never deploy on Fridays.") && !anyHolds(historyOf(b), "Never deploy on Fridays.") {
				t.Errorf("the person's text is gone on B with no copy: %q\n%s", got, out)
			}
			assertI1(t, stub, a)
			assertI1(t, stub, b)
		})
		t.Run(at+": the model rewrote the index here", func(t *testing.T) {
			stub, a, _ := indexPair(t, idx)
			a.use()
			a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n")
			now(a)
			a.use()
			got := a.readFile("memory/MEMORY.md")
			if !strings.Contains(got, "](fb.md)") {
				t.Errorf("the line of an existing note did not come back: %q", got)
			}
			if len(a.conflictFiles()) != 0 {
				t.Errorf("a copy of what the model wrote, though all of it stays: %v", a.conflictFiles())
			}
			assertI1(t, stub, a)
		})
		t.Run(at+": changed on both sides", func(t *testing.T) {
			_, a, b := indexPair(t, idx)
			b.use()
			b.writeFile("memory/MEMORY.md", strings.Replace(idx, "Never deploy on Fridays.\n", "Never deploy on Fridays.\nAlso not on Mondays.\n", 1))
			a.use()
			a.writeFile("memory/MEMORY.md", idxLines)
			now(a)
			out := now(b)
			kept := false
			for _, c := range b.conflictFiles() {
				kept = kept || strings.Contains(c, "Also not on Mondays.")
			}
			b.use()
			if !strings.Contains(b.readFile("memory/MEMORY.md"), "Also not on Mondays.") &&
				(!kept || !strings.Contains(out, "the lines of all notes are kept")) {
				t.Errorf("B's line neither stays nor is kept and said: %q, %v\n%s", b.readFile("memory/MEMORY.md"), b.conflictFiles(), out)
			}
		})
	}
	t.Run("text changed here while a note was added there", func(t *testing.T) {
		_, a, b := indexPair(t, rulesSection+"\n"+idxLines)
		a.use()
		a.writeFile("memory/MEMORY.md", rulesSection+"Also not on Mondays.\n\n"+idxLines)
		b.use()
		b.writeFile("memory/plan.md", "plan\n")
		b.writeFile("memory/MEMORY.md", rulesSection+"\n"+idxLines+"- [Plan](plan.md)\n")
		now(b)
		out := now(a)
		now(b)
		for _, m := range []*machine{a, b} {
			m.use()
			got := m.readFile("memory/MEMORY.md")
			if !strings.Contains(got, "Also not on Mondays.") || !strings.Contains(got, "](plan.md)") {
				t.Errorf("%s: not both: %q", m.name, got)
			}
		}
		if len(a.conflictFiles()) != 0 || strings.Contains(out, "changed here and on the server") {
			t.Errorf("A's text went aside though only A changed it: %v\n%s", a.conflictFiles(), out)
		}
	})
}

// A line of a note deleted on another computer, unknown here, goes (the
// mutation B02 of the fourth review).
func TestRound5_LineOfANoteDeletedElsewhereGoes(t *testing.T) {
	stub, a, b := memPair(t)
	a.use()
	a.writeFile("memory/old.md", "old\n")
	a.writeFile("memory/MEMORY.md", idxLines+"- [Old](old.md)\n")
	now(a)
	a.use()
	if err := os.Remove(filepath.Join(a.dir(), "memory", "old.md")); err != nil {
		t.Fatal(err)
	}
	a.writeFile("memory/MEMORY.md", idxLines)
	now(a)
	// An older agent elsewhere writes an index that still has the line.
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	ct, err := seal(key, KindMemory, "MEMORY.md", []byte(idxLines+"- [Old](old.md)\n"))
	if err != nil {
		t.Fatal(err)
	}
	stub.putDirect(KindMemory, memoryName(key, "MEMORY.md"), ct)
	now(b)
	b.use()
	if got := b.readFile("memory/MEMORY.md"); strings.Contains(got, "old.md") {
		t.Errorf("the line of a note deleted elsewhere stays: %q", got)
	}
}

// The objection to «no line without a note on the server»: a note another
// computer has but cannot send (a link, too big, unreadable) is on no
// server; dropping its line here would make the two computers take it away
// and give it back on every exchange. Its line stays, and nothing goes up
// twice.
func TestRound5_LineOfANoteNeverSentStays(t *testing.T) {
	stub, a, b := memPair(t)
	a.use()
	target := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(target, []byte("kept outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(a.dir(), "memory", "linked.md")); err != nil {
		t.Skip("no symbolic links here")
	}
	a.writeFile("memory/MEMORY.md", idxLines+"- [Linked](linked.md)\n")
	for i := 0; i < 2; i++ {
		now(a)
		now(b)
	}
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	idx := memoryName(key, "MEMORY.md")
	v := stub.blob(KindMemory, idx).version
	for i := 0; i < 2; i++ {
		now(a)
		now(b)
	}
	if got := stub.blob(KindMemory, idx).version; got != v {
		t.Errorf("the index goes back and forth: v%d → v%d", v, got)
	}
	b.use()
	if got := b.readFile("memory/MEMORY.md"); !strings.Contains(got, "](linked.md)") {
		t.Errorf("B dropped the line of a note another computer has: %q", got)
	}
}

// D: a note without a line gets one — from the description in its header,
// else from its name — on every computer, and settles.
func TestRound5_NoteWithoutALineGetsOne(t *testing.T) {
	stub, a, b := memPair(t)
	a.use()
	a.writeFile("memory/plan.md", "---\nname: plan\ndescription: the release plan for R8\nmetadata:\n  type: project\n---\n\nship it\n")
	a.writeFile("memory/bare.md", "no header here\n")
	now(a)
	a.use()
	if got := a.readFile("memory/MEMORY.md"); strings.Contains(got, "](plan.md)") {
		t.Errorf("a line made in the exchange that first sent the note (the model may be writing it): %q", got)
	}
	now(b)
	now(a)
	now(b)
	for _, m := range []*machine{a, b} {
		m.use()
		got := m.readFile("memory/MEMORY.md")
		if !strings.Contains(got, "](plan.md)") || !strings.Contains(got, "the release plan for R8") || !strings.Contains(got, "](bare.md)") {
			t.Errorf("%s: the notes have no lines: %q", m.name, got)
		}
		if strings.Count(got, "](plan.md)") != 1 {
			t.Errorf("%s: more than one line for a note: %q", m.name, got)
		}
	}
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	idx := memoryName(key, "MEMORY.md")
	v := stub.blob(KindMemory, idx).version
	now(a)
	now(b)
	if got := stub.blob(KindMemory, idx).version; got != v {
		t.Errorf("the index does not settle: v%d → v%d", v, got)
	}
	if len(a.conflictFiles())+len(b.conflictFiles()) != 0 {
		t.Errorf("copies for added lines: %v %v", a.conflictFiles(), b.conflictFiles())
	}
}

// The model writes a note and then its line: whether an exchange comes in
// between or the agent already gave the note a line, the index ends with
// one line for it, the model's, and no copy.
func TestRound5_TheModelsOwnLineWins(t *testing.T) {
	for _, made := range []bool{false, true} {
		name := "an exchange between the note and its line"
		if made {
			name = "after the agent gave the note a line"
		}
		t.Run(name, func(t *testing.T) {
			_, a, _ := memPair(t)
			a.use()
			a.writeFile("memory/plan.md", "---\ndescription: generated words\n---\nship it\n")
			now(a)
			if made {
				now(a)
				if got := a.readFile("memory/MEMORY.md"); !strings.Contains(got, "generated words") {
					t.Fatalf("setup: no line made: %q", got)
				}
			}
			a.use()
			a.writeFile("memory/MEMORY.md", a.readFile("memory/MEMORY.md")+"- [Plan](plan.md) — the model's own words\n")
			now(a)
			now(a)
			a.use()
			got := a.readFile("memory/MEMORY.md")
			if strings.Count(got, "](plan.md)") != 1 || !strings.Contains(got, "the model's own words") {
				t.Errorf("the index: %q", got)
			}
			if len(a.conflictFiles()) != 0 {
				t.Errorf("copies: %v", a.conflictFiles())
			}
		})
	}
}

// A base an older agent recorded hashed subscriptions.json byte by byte:
// with a model list in the file, the first exchange of this agent neither
// sends the same settings again nor takes a change made elsewhere for a
// conflict.
func TestRound5_SettingsBaseOfAnOlderAgent(t *testing.T) {
	stub, a, b := settingsPair(t, subsRefresh)
	b.use()
	raw, _ := os.ReadFile(b.svc.Paths.BasesFile())
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	settings, _ := m["settings"].(map[string]any)
	if settings == nil {
		t.Fatalf("setup: no settings base: %s", raw)
	}
	settings["subscriptions.json"] = hashOf([]byte(b.readFile("subscriptions.json")))
	out, _ := json.Marshal(m)
	if err := os.WriteFile(b.svc.Paths.BasesFile(), out, 0o600); err != nil {
		t.Fatal(err)
	}
	ver := stub.blob(KindConfig, NameConfig).version
	now(b)
	if got := stub.blob(KindConfig, NameConfig).version; got != ver {
		t.Errorf("the same settings went up again: v%d → v%d", ver, got)
	}
	a.use()
	a.writeFile("subscriptions.json", strings.Replace(subsRefresh, `"zai":{`, `"kimi":{"provider":"kimi","api_key":"k-kimi-from-A"},"zai":{`, 1))
	now(a)
	text := now(b)
	b.use()
	if !strings.Contains(b.readFile("subscriptions.json"), "k-kimi-from-A") || len(b.conflictFiles()) != 0 {
		t.Errorf("B: %q, copies %v\n%s", b.readFile("subscriptions.json"), b.conflictFiles(), text)
	}
}

// ===== an older snapshot of a project keeps what it lacks (A, x1) =====

func TestRound5_OlderSnapshotKeepsWhatItLacks(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make\nv2\n", time.Time{})
	writeProjectFile(t, pA, "a-new.md", "new on A\n", time.Time{})
	now(a)
	now(b)
	// An older agent bound a fresh clone with the .execai/ committed long
	// ago: it sends the old pack as an edit.
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	ct, err := seal(key, KindProjectMemory, "ws-1", mustPack(map[string][]byte{
		"EXECAI.md": []byte("build: make\n"), "notes.md": []byte("deploy from R7\n")}))
	if err != nil {
		t.Fatal(err)
	}
	stub.putDirect(KindProjectMemory, "ws-1", ct)
	for _, s := range []struct {
		m *machine
		p string
	}{{b, pB}, {a, pA}} {
		out := now(s.m)
		if got := readProjectFile(s.p, "EXECAI.md"); got != "build: make\n" {
			t.Errorf("%s: EXECAI.md %q", s.m.name, got)
		}
		if !hasCopyOf(ProjectDir(s.p), "build: make\nv2\n") {
			t.Errorf("%s: the newer EXECAI.md is not kept next to the file", s.m.name)
		}
		if !hasCopyOf(ProjectDir(s.p), "new on A\n") {
			t.Errorf("%s: a-new.md, missing from the older snapshot, is not kept for good (history only: %v)\n%s",
				s.m.name, anyHolds(historyOf(s.m), "new on A\n"), out)
		}
		if strings.Contains(out, "deleted on another computer") {
			t.Errorf("%s: a file nobody deleted is said deleted:\n%s", s.m.name, out)
		}
		if !strings.Contains(out, "older version of the project") || strings.Count(out, "older version") != 1 {
			t.Errorf("%s: not one line for the project:\n%s", s.m.name, out)
		}
	}
}

// A deletion with no return in the same version stays row 2c.
func TestRound5_ADeletionAloneStaysADeletion(t *testing.T) {
	_, a, b, pA, pB := lossPair(t)
	if err := os.Remove(filepath.Join(ProjectDir(pA), "notes.md")); err != nil {
		t.Fatal(err)
	}
	now(a)
	out := now(b)
	if _, err := os.Stat(filepath.Join(ProjectDir(pB), "notes.md")); err == nil {
		t.Error("not removed on B")
	}
	if hasCopyOf(ProjectDir(pB), "deploy from R7\n") || !anyHolds(historyOf(b), "deploy from R7\n") ||
		!strings.Contains(out, "not in the version that came from the server") || strings.Contains(out, "deleted on another computer") {
		t.Errorf("not row 2c, said without a cause:\n%s", out)
	}
}

// ===== a deletion here undone by a change elsewhere is said (i5b) =====

func TestRound5_DeletionUndoneElsewhereIsSaid(t *testing.T) {
	t.Run("a memory note", func(t *testing.T) {
		_, a, b := memPair(t)
		b.use()
		if err := os.Remove(filepath.Join(b.dir(), "memory", "fb.md")); err != nil {
			t.Fatal(err)
		}
		now(b)
		a.use()
		a.writeFile("memory/fb.md", "short answers, edited on A\n")
		now(a)
		out := now(b)
		b.use()
		if b.readFile("memory/fb.md") != "short answers, edited on A\n" {
			t.Fatalf("B: %q", b.readFile("memory/fb.md"))
		}
		if !strings.Contains(out, "deleted here") {
			t.Errorf("the note deleted here came back without a word:\n%s", out)
		}
	})
	t.Run("a project file", func(t *testing.T) {
		_, a, b, pA, pB := lossPair(t)
		if err := os.Remove(filepath.Join(ProjectDir(pB), "notes.md")); err != nil {
			t.Fatal(err)
		}
		now(b)
		writeProjectFile(t, pA, "notes.md", "deploy from R8\n", time.Time{})
		now(a)
		out := now(b)
		if readProjectFile(pB, "notes.md") != "deploy from R8\n" {
			t.Fatalf("B: %q", readProjectFile(pB, "notes.md"))
		}
		if !strings.Contains(out, "deleted here") {
			t.Errorf("the file deleted here came back without a word:\n%s", out)
		}
	})
}

// ===== a return older than the tail (Т-2) =====

func TestRound5_ReturnWithinTheLast64Versions(t *testing.T) {
	_, a, b, pA, pB := lossPair(t)
	for i := 1; i <= 40; i++ {
		writeProjectFile(t, pA, "EXECAI.md", "build: make\nv"+itoa(i)+"\n", time.Time{})
		now(a)
		now(b)
	}
	writeProjectFile(t, pA, "EXECAI.md", "build: make\n", time.Time{})
	now(a)
	now(b)
	if !hasCopyOf(ProjectDir(pB), "build: make\nv40\n") {
		t.Error("a return to the 41st version back went silently to the history")
	}
}

func TestRound5_TheTailHolds64(t *testing.T) {
	var list []string
	for i := 0; i < 100; i++ {
		list = pushPrev(list, hashOf([]byte(itoa(i))))
	}
	if len(list) != 64 {
		t.Errorf("the tail holds %d", len(list))
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for ; i > 0; i /= 10 {
		s = string(rune('0'+i%10)) + s
	}
	return s
}

// ===== copies are private (Н-5) =====

func TestRound5_CopiesArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("modes are Unix")
	}
	_, a, b := settingsPair(t, subsBase)
	for _, m := range []*machine{a, b} {
		m.use()
		for _, d := range []string{"sync-unsent", "sync-history"} {
			// As an older agent or a loose umask may have left them.
			_ = os.MkdirAll(filepath.Join(m.dir(), d), 0o755)
			_ = os.Chmod(filepath.Join(m.dir(), d), 0o755)
		}
	}
	a.use()
	a.writeFile("memory/MEMORY.md", idxLines)
	a.writeFile("memory/role.md", "Go engineer\n")
	a.writeFile("memory/fb.md", "short\n")
	now(a)
	now(b)
	// Both change the key (row 4: a copy in sync-unsent on A) and B the
	// model (row 2a on A: config.json to the history); a note too.
	b.use()
	b.writeFile("subscriptions.json", strings.Replace(subsBase, "k-zai-test", "k-zai-from-B", 1))
	b.writeFile("config.json", `{"api_base":"https://api.b","model":"kimi"}`)
	b.writeFile("memory/role.md", "Go engineer, from B\n")
	b.writeFile("memory/fb.md", "short, from B\n")
	now(b)
	a.use()
	a.writeFile("subscriptions.json", strings.Replace(subsBase, "k-zai-test", "k-zai-from-A", 1))
	a.writeFile("memory/fb.md", "short, from A\n")
	now(a)
	a.use()
	if len(a.conflictFiles()) == 0 || len(historyOf(a)) == 0 {
		t.Fatalf("setup: no copies (%v) or no history (%v)", a.conflictFiles(), historyOf(a))
	}
	for _, root := range []string{filepath.Join(a.dir(), "sync-unsent"), filepath.Join(a.dir(), "sync-history")} {
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			fi, _ := d.Info()
			want := os.FileMode(0o600)
			if d.IsDir() {
				want = 0o700
			}
			if fi.Mode().Perm() != want {
				t.Errorf("%s: %v, want %v", p, fi.Mode().Perm(), want)
			}
			return nil
		})
	}
}

// The same settings copy is not made twice; the person is told each time.
func TestRound5_SameSettingsCopyIsNotRepeated(t *testing.T) {
	_, a, b := settingsPair(t, subsBase)
	mine := strings.Replace(subsBase, "k-zai-test", "k-zai-from-A", 1)
	for i, theirs := range []string{"k-zai-from-B", "k-zai-from-B-again"} {
		b.use()
		b.writeFile("subscriptions.json", strings.Replace(subsBase, "k-zai-test", theirs, 1))
		now(b)
		a.use()
		a.writeFile("subscriptions.json", mine)
		out := now(a)
		if !strings.Contains(out, "subscriptions.local-") {
			t.Errorf("round %d: not told:\n%s", i, out)
		}
	}
	a.use()
	if n := len(a.conflictFiles()); n != 1 {
		t.Errorf("%d copies of the same settings file: %v", n, a.conflictFiles())
	}
}

// ===== what an earlier key left on the server: every kind (C, probe d) =====

// After «keep this computer's key» the settings and memory files written
// under the earlier key stayed there for good: unreadable everywhere, the
// settings of this computer never went up, and the line asked for a
// password that does not help. They are named now (the settings as such,
// memory files by number), /sync on asks about them as about projects, and
// once deleted this computer's versions go up.
func TestRound5_SettingsAndMemoryUnderAnEarlierKey(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.writeFile("permissions.json", `{"always_allowed_tools":["Read"]}`)
	a.writeFile("subscriptions.json", subsBase)
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	// What an earlier key left: the settings and a note.
	other, _ := crypto.Generate()
	cfg, _ := seal(other, KindConfig, NameConfig, mustPack(map[string][]byte{"permissions.json": []byte(`{"always_allowed_tools":["Bash"]}`)}))
	stub.putDirect(KindConfig, NameConfig, cfg)
	note, _ := seal(other, KindMemory, "old.md", []byte("under the old key\n"))
	stub.putDirect(KindMemory, memoryName(other, "old.md"), note)
	ver := stub.blob(KindConfig, NameConfig).version

	a.use()
	a.writeFile("permissions.json", `{"always_allowed_tools":["Read","Grep"]}`)
	var notes []string
	a.svc.background(context.Background(), Hooks{Notify: func(s string) { notes = append(notes, s) }}, false)
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "the settings and provider keys") || !strings.Contains(joined, "a memory file") ||
		strings.Contains(joined, "Enter the sync password") {
		t.Errorf("not named, or the password asked for: %q", notes)
	}
	if got := stub.blob(KindConfig, NameConfig).version; got != ver {
		t.Fatalf("the settings went over what another key holds: v%d → v%d", ver, got)
	}
	a.use()
	if st := a.svc.Status(); !strings.Contains(st, "the settings and provider keys") || strings.Contains(st, "Enter the sync password") {
		t.Errorf("/sync: %s", st)
	}
	steps := a.converse(a.svc.OnFlow(), "1")
	if !strings.Contains(steps[0].Text, "the settings and provider keys") || !strings.Contains(steps[1].Text, "Deleted from the server: 2.") {
		t.Fatalf("not asked, or not deleted: %+v", steps)
	}
	key, _ := crypto.LoadIdentity(a.dir())
	bl := stub.blob(KindConfig, NameConfig)
	if bl == nil || bl.deleted {
		t.Fatal("A's settings did not go up")
	}
	_, _, data, err := unseal(key, bl.content)
	if err != nil {
		t.Fatalf("the settings are not under A's key: %v", err)
	}
	files, _ := unpackFiles(data)
	if !strings.Contains(string(files["permissions.json"]), "Grep") || !strings.Contains(string(files["subscriptions.json"]), "k-zai-test") {
		t.Errorf("the server's settings, all of them: %q", files)
	}
	now(b)
	b.use()
	if !strings.Contains(b.readFile("permissions.json"), "Grep") {
		t.Errorf("B did not get A's permissions: %q", b.readFile("permissions.json"))
	}
	if last := a.converse(a.svc.OnFlow()); !last[0].Done() {
		t.Errorf("asked again after the answer: %+v", last[0])
	}
}

// Forgotten on another computer: the server has no key and nothing on it;
// /sync does not ask for a password because of files that were unreadable
// before.
func TestRound5_NoLineAboutOldFilesAfterForget(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	other, _ := crypto.Generate()
	note, _ := seal(other, KindMemory, "old.md", []byte("under the old key\n"))
	stub.putDirect(KindMemory, memoryName(other, "old.md"), note)
	now(b)
	a.converse(a.svc.ForgetFlow(), "delete")
	now(b)
	b.use()
	if st := b.svc.Status(); strings.Contains(st, "Enter the sync password") || strings.Contains(st, "earlier sync key") {
		t.Errorf("/sync after forget: %s", st)
	}
}

// After another computer kept its own key and deleted a project it did not
// have, this computer takes the new key and sends the project again — said
// as what it is, not as a change made here.
func TestRound5_AfterAKeyChangeTheProjectIsSentAgainWithIt(t *testing.T) {
	stub, a, c, _, a2, _ := overwriteSetup(t)
	c.converse(c.svc.OnFlow(), password, "2")
	if bl := stub.blob(KindProjectMemory, "ws-2"); bl == nil || !bl.deleted {
		t.Fatal("setup: Beta was not deleted")
	}
	text := allText(a.converse(a.svc.OnFlow(), password, "1"))
	if !strings.Contains(text, "sent to the server again, now encrypted with the current sync key") ||
		strings.Contains(text, "changed here (or never sent)") || strings.Contains(text, "this computer has content in it the server never had") {
		t.Errorf("A:\n%s", text)
	}
	if bl := stub.blob(KindProjectMemory, "ws-2"); bl == nil || bl.deleted {
		t.Error("Beta did not go up with the new key")
	}
	if readProjectFile(a2, "EXECAI.md") != "beta from A\n" {
		t.Errorf("A's Beta: %q", readProjectFile(a2, "EXECAI.md"))
	}
}

// A failed /sync now says the error sync made in the person's language.
func TestRound5_FailedExchangeSaysTheErrorInTheLanguage(t *testing.T) {
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	if got := (&Service{}).failText(&StatusError{Code: 404}); strings.Contains(got, "sync: HTTP") || !strings.Contains(got, "404") {
		t.Errorf("%q", got)
	}
}

// The objection to «l ≠ b: always a permanent copy, said»: a line the merge
// leaves out that the server had (in the base) is no unsent content. Here A
// changed only a note line, B only the text around the lines: the table
// gives B's text (row 2a for that text), A's previous text goes to the
// history, and A is told nothing — telling «changed here and on the
// server» would be untrue.
func TestRound5_TextTheServerHadGoesToTheHistory(t *testing.T) {
	_, a, b := indexPair(t, rulesSection+"\n"+idxLines)
	a.use()
	a.writeFile("memory/MEMORY.md", rulesSection+"\n- [Role](role.md) — Go and Rust\n- [Fb](fb.md)\n")
	b.use()
	b.writeFile("memory/MEMORY.md", "## Rules\nDeploy on Tuesdays only.\n\n"+idxLines)
	now(b)
	out := now(a)
	a.use()
	got := a.readFile("memory/MEMORY.md")
	if !strings.Contains(got, "Deploy on Tuesdays only.") || !strings.Contains(got, "— Go and Rust") || strings.Contains(got, "Never deploy") {
		t.Errorf("A's index: %q", got)
	}
	if len(a.conflictFiles()) != 0 || strings.Contains(out, "changed here and on the server") {
		t.Errorf("a permanent copy or a line for text the server had: %v\n%s", a.conflictFiles(), out)
	}
	if !anyHolds(historyOf(a), "Never deploy on Fridays.") {
		t.Error("A's previous text is not in the history")
	}
}
