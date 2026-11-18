package sync

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// The findings of the third review (K1…K13), each as a test of what must
// hold now; red on the candidate it read (0e3f4da), green here.

func r3Tombstone(t *testing.T, stub *stubServer) {
	t.Helper()
	old := NewClient(stub.URL, func() string { return "tok-old" })
	if _, err := old.Delete(context.Background(), KindProjectMemory, "ws-1", serverVersion(stub)); err != nil {
		t.Fatal(err)
	}
}

// K1 — the scenario this work began with: a 7.15 computer clones again,
// marks the project deleted, then writes a fresh EXECAI.md over the mark.
// After the mark there is no base: B's project memory is not replaced
// silently — the server's fresh version is used, B's is kept next to it for
// good, and B is told.
func TestReview3_K1_KeptProjectMeetsAFreshVersion(t *testing.T) {
	stub, a, b, _, pB := lossPair(t)
	r3Tombstone(t, stub)
	now(b) // kept
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	ct, _ := seal(key, KindProjectMemory, "ws-1", mustPack(map[string][]byte{"EXECAI.md": []byte("fresh clone, nothing yet\n")}))
	stub.putDirect(KindProjectMemory, "ws-1", ct)
	out := now(b)
	if !hasCopyOf(ProjectDir(pB), "build: make\n") || !strings.Contains(out, "EXECAI.md.local-") {
		t.Errorf("B's project memory not kept next to the file, or not told: %v\n%s", rvList(ProjectDir(pB)), out)
	}
	if readProjectFile(pB, "notes.md") != "deploy from R7\n" || serverPack(t, stub, key, "ws-1")["notes.md"] != "deploy from R7\n" {
		t.Errorf("notes.md, which the fresh clone lacks, is not back in the project")
	}
	assertI1(t, stub, b)
}

// K2 — a computer joins with .execai/ content of its own while the project
// is marked deleted: content the server never saw brings the project back —
// it goes up at once, and the person is told; nothing lies unsent.
func TestReview3_K2_JoinerOwnContentBringsTheProjectBack(t *testing.T) {
	stub, a, _, _, _ := lossPair(t)
	r3Tombstone(t, stub)
	c := newMachine(t, stub, "c")
	c.converse(c.svc.OnFlow(), password)
	pC := t.TempDir()
	writeProjectFile(t, pC, "EXECAI.md", "C's own notes, never synced\n", time.Time{})
	stub.mu.Lock()
	stub.bindings = append(stub.bindings, Binding{AgentID: "agent-c", WorkspaceID: "ws-1", LocalPath: pC})
	stub.mu.Unlock()
	out := now(c)
	key, _ := crypto.LoadIdentity(c.dir())
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted || serverPack(t, stub, key, "ws-1")["EXECAI.md"] != "C's own notes, never synced\n" {
		t.Fatalf("C's own content did not go up over the mark:\n%s", out)
	}
	if !strings.Contains(out, "the whole project (one file) is sent back up for all your computers") {
		t.Errorf("not told:\n%s", out)
	}
	assertI1(t, stub, c)
	now(a)
	assertI1(t, stub, a)
}

// K3 — one computer: the project is marked deleted while the folder here is
// empty; the person then writes new notes there. They go up at once (the
// project comes back) instead of lying kept and unsent.
func TestReview3_K3_NewNotesInADeletedProjectGoUp(t *testing.T) {
	stub, a, b, _, pB := lossPair(t)
	_ = os.Remove(filepath.Join(ProjectDir(pB), "EXECAI.md"))
	_ = os.Remove(filepath.Join(ProjectDir(pB), "notes.md"))
	r3Tombstone(t, stub)
	now(b) // empty here, deletion mark on the server
	writeProjectFile(t, pB, "EXECAI.md", "B: new notes written after\n", time.Time{})
	out := now(b)
	key, _ := crypto.LoadIdentity(b.dir())
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted || serverPack(t, stub, key, "ws-1")["EXECAI.md"] != "B: new notes written after\n" {
		t.Fatalf("the new notes did not go up:\n%s", out)
	}
	assertI1(t, stub, b)
	now(a)
	assertI1(t, stub, a)
}

// K3b — the very files the mark deleted stay where they are, unsent, and
// the person is told once — not on every exchange.
func TestReview3_K3b_KeptFilesToldOnce(t *testing.T) {
	stub, _, b, _, _ := lossPair(t)
	r3Tombstone(t, stub)
	v := serverVersion(stub)
	first := now(b)
	second := now(b)
	if !strings.Contains(first, "marked deleted") || strings.Contains(second, "marked deleted") {
		t.Errorf("told %v then %v", strings.Contains(first, "marked deleted"), strings.Contains(second, "marked deleted"))
	}
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || !bl.deleted || serverVersion(stub) != v {
		t.Error("the kept files were sent back by themselves")
	}
	b.use()
	if rec := loadBases(b.svc.Paths).Projects["ws-1"]; rec.Files != nil || rec.LastLive == nil {
		t.Errorf("after the mark: manifest %v (want none), last live %v", rec.Files, rec.LastLive)
	}
}

// K4 — the server lists a newer version of a note, but reading it answers
// 404 (purged meanwhile, or a gateway): not a deletion — the note here is
// not touched, and the next exchange takes the new version.
func TestReview3_K4_ListedNoteNotGivenIsNotADeletion(t *testing.T) {
	stub, a, b := memPair(t)
	a.use()
	a.writeFile("memory/role.md", "Go engineer, v2\n")
	now(a)
	key, _ := crypto.LoadIdentity(a.dir())
	target := "/blobs/" + KindMemory + "/" + memoryName(key, "role.md")
	inner := stub.Config.Handler
	stub.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, target) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
		inner.ServeHTTP(w, r)
	})
	now(b)
	stub.Config.Handler = inner
	b.use()
	if got := b.readFile("memory/role.md"); got != "Go engineer\n" {
		t.Fatalf("the note was touched on a 404: %q", got)
	}
	now(b)
	b.use()
	if got := b.readFile("memory/role.md"); got != "Go engineer, v2\n" {
		t.Errorf("the new version did not come: %q", got)
	}
	assertI1(t, stub, b)
}

// K6 — the index deleted while the chat runs and written again by the model
// with one new entry: the lines of the notes that exist come back; nothing
// is lost on any computer.
func TestReview3_K6_IndexWrittenAgainWithOneEntry(t *testing.T) {
	stub, a, b := memPair(t)
	a.use()
	_ = os.Remove(filepath.Join(a.dir(), "memory", "MEMORY.md"))
	a.writeFile("memory/MEMORY.md", "- [New](new.md)\n")
	a.writeFile("memory/new.md", "new fact\n")
	if _, err := a.svc.exchange(context.Background(), Options{PushOnly: true}, time.Second); err != nil {
		t.Fatal(err)
	}
	now(a)
	now(b)
	for _, m := range []*machine{a, b} {
		m.use()
		idx := m.readFile("memory/MEMORY.md")
		for _, line := range []string{"[Role](role.md)", "[Fb](fb.md)", "[New](new.md)"} {
			if !strings.Contains(idx, line) {
				t.Errorf("%s: %s lost from the index %q", m.name, line, idx)
			}
		}
	}
	assertI1(t, stub, a)
	assertI1(t, stub, b)
}

// K6b — the template on a fresh account: an index without a single entry
// never goes up, with no base as with one.
func TestReview3_K6b_TemplateOnAFreshAccountStaysHere(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	a.use()
	a.writeFile("memory/MEMORY.md", chatTemplate)
	now(a)
	key, _ := crypto.LoadIdentity(a.dir())
	if bl := stub.blob(KindMemory, memoryName(key, "MEMORY.md")); bl != nil {
		t.Fatal("the template went up")
	}
	now(b)
	b.use()
	if b.readFile("memory/MEMORY.md") != "<missing>" {
		t.Error("the template reached another computer")
	}
}

// K7 — an in-place return to older content in a trusted folder: it goes up
// as a return and the sender says so; the other computer keeps the newer
// text next to the file (2b). A version with a return in it is an older
// snapshot of the project (round 5): the file it lacks is removed there but
// kept for good next to its place, and the project is said in one line.
func TestReview3_K7_InPlaceReturnIsSaidOnBothSides(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make\nnewer\n", time.Time{})
	writeProjectFile(t, pA, "plan.md", "plan written later\n", time.Time{})
	now(a)
	now(b)
	writeProjectFile(t, pA, "EXECAI.md", "build: make\n", time.Time{}) // the older content, same folder
	_ = os.Remove(filepath.Join(ProjectDir(pA), "plan.md"))
	outA := now(a)
	outB := now(b)
	if !strings.Contains(outA, "the same as an earlier version") {
		t.Errorf("A did not say it sent a return:\n%s", outA)
	}
	if readProjectFile(pB, "EXECAI.md") != "build: make\n" || !hasCopyOf(ProjectDir(pB), "build: make\nnewer\n") ||
		!strings.Contains(outB, "older version of the project") {
		t.Errorf("B: the newer text not kept next to the file, or not told:\n%s", outB)
	}
	if readProjectFile(pB, "plan.md") != "<missing>" || !hasCopyOf(ProjectDir(pB), "plan written later\n") ||
		strings.Contains(outB, "deleted on another computer") {
		t.Errorf("B: plan.md not kept for good, or said deleted:\n%s", outB)
	}
	assertI1(t, stub, b)
}

// K8 — the memory folder here is another one (copied: a new identity) and
// holds a note and its index line not sent yet: the note goes up, and its
// line is in the index on every computer.
func TestReview3_K8_UntrustedMemoryFolderKeepsTheIndexLine(t *testing.T) {
	stub, a, b := memPair(t)
	a.use()
	a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Fb](fb.md)\n- [Plan](plan.md)\n")
	a.writeFile("memory/plan.md", "the plan\n")
	mem := filepath.Join(a.dir(), "memory")
	tmp := filepath.Join(a.dir(), "memory-copy")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(mem)
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(mem, e.Name()))
		_ = os.WriteFile(filepath.Join(tmp, e.Name()), data, 0o600)
	}
	_ = os.RemoveAll(mem)
	_ = os.Rename(tmp, mem)
	now(a)
	now(b)
	for _, m := range []*machine{a, b} {
		m.use()
		if m.readFile("memory/plan.md") != "the plan\n" || !strings.Contains(m.readFile("memory/MEMORY.md"), "[Plan](plan.md)") {
			t.Errorf("%s: plan.md %q, index %q", m.name, m.readFile("memory/plan.md"), m.readFile("memory/MEMORY.md"))
		}
	}
	assertI1(t, stub, a)
}

// K10 — the settings are decided file by file: a provider key added on A
// and a model changed on B both stay, on both computers.
func TestReview3_K10_SettingsFileByFile(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.writeFile("config.json", `{"api_base":"https://api.a","model":"glm"}`)
	a.writeFile("subscriptions.json", `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"k-zai-test"}}}`)
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	a.use()
	a.writeFile("subscriptions.json", `{"active":"zai","subscriptions":{"zai":{"provider":"zai","api_key":"k-zai-test"},"kimi":{"provider":"kimi","api_key":"k-kimi-new-on-A"}}}`)
	b.use()
	b.writeFile("config.json", `{"api_base":"https://api.a","model":"kimi-k3"}`)
	now(b)
	outA := now(a)
	now(b)
	for _, m := range []*machine{a, b} {
		m.use()
		if !strings.Contains(m.readFile("subscriptions.json"), "k-kimi-new-on-A") || !strings.Contains(m.readFile("config.json"), "kimi-k3") {
			t.Errorf("%s: subscriptions %q, config %q", m.name, m.readFile("subscriptions.json"), m.readFile("config.json"))
		}
	}
	if strings.Contains(outA, "kept") {
		t.Errorf("A was told of a copy:\n%s", outA)
	}
	assertI1(t, stub, a)
	assertI1(t, stub, b)
}

// K10b — a conflict in permissions.json is said as such: the permissions and
// prohibitions of this computer were not applied, and where they are.
func TestReview3_K10b_PermissionsConflictIsSaid(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.writeFile("permissions.json", `{"always_allowed_tools":["Read"],"always_allowed_exact":[]}`)
	a.converse(a.svc.OnFlow(), password, password)
	b := newMachine(t, stub, "b")
	b.converse(b.svc.OnFlow(), password)
	a.use()
	a.writeFile("permissions.json", `{"always_allowed_tools":["Read","Grep"],"always_allowed_exact":[]}`)
	now(a)
	b.use()
	b.writeFile("permissions.json", `{"always_allowed_tools":[],"always_allowed_exact":[]}`)
	out := now(b)
	if !strings.Contains(out, "Permissions and prohibitions set on this computer were not applied") {
		t.Errorf("not said:\n%s", out)
	}
}

// K11 — a file here and a folder of the same name on the server: left as
// it is both ways, said once in plain words; nothing claims a sending that
// did not happen, and the rest of the project syncs.
func TestReview3_K11_FileAndFolderOfOneName(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	_ = os.Remove(filepath.Join(ProjectDir(pB), "notes.md"))
	writeProjectFile(t, pB, "notes.md/x.md", "x in a folder\n", time.Time{})
	outB := now(b)
	if !strings.Contains(outB, "is a file on one side and a folder on the other") {
		t.Errorf("not said:\n%s", outB)
	}
	writeProjectFile(t, pA, "notes.md", "deploy from R7\nedited on A\n", time.Time{})
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	var outs []string
	for i := 0; i < 3; i++ {
		outs = append(outs, now(a), now(b))
	}
	for _, o := range outs {
		if strings.Contains(o, "sent again") {
			t.Errorf("a sending claimed:\n%s", o)
		}
	}
	if readProjectFile(pB, "notes.md/x.md") != "x in a folder\n" || readProjectFile(pB, "EXECAI.md") != "build: make all\n" {
		t.Errorf("B: %v", rvList(ProjectDir(pB)))
	}
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	if serverPack(t, stub, key, "ws-1")["EXECAI.md"] != "build: make all\n" {
		t.Error("the rest of the project did not sync")
	}
	if readProjectFile(pA, "notes.md") != "deploy from R7\nedited on A\n" {
		t.Error("the file here was touched")
	}
	assertI1(t, stub, a)
}

// K12 (probe f2, guard): S deletes x.md and edits a.md; N edits b.md; both
// edits stay everywhere, x.md stays deleted.
func TestReview3_K12_ProbeF2PerFileMerge(t *testing.T) {
	_, s, n, pS, pN := lossPair(t)
	writeProjectFile(t, pS, "x.md", "x\n", time.Time{})
	writeProjectFile(t, pS, "a.md", "a\n", time.Time{})
	writeProjectFile(t, pS, "b.md", "b\n", time.Time{})
	now(s)
	now(n)
	_ = os.Remove(filepath.Join(ProjectDir(pS), "x.md"))
	writeProjectFile(t, pS, "a.md", "a\nS edit\n", time.Time{})
	writeProjectFile(t, pN, "b.md", "b\nN edit\n", time.Time{})
	now(s)
	now(n)
	now(s)
	for name, dir := range map[string]string{"S": pS, "N": pN} {
		if readProjectFile(dir, "a.md") != "a\nS edit\n" || readProjectFile(dir, "b.md") != "b\nN edit\n" || readProjectFile(dir, "x.md") != "<missing>" {
			t.Errorf("f2 on %s: a %q b %q x %q", name, readProjectFile(dir, "a.md"), readProjectFile(dir, "b.md"), readProjectFile(dir, "x.md"))
		}
	}
}

// K13 (probe i5, guard): a note deleted on B; A's memory folder replaced by
// an old copy holding it (a new identity): it does not come back anywhere.
func TestReview3_K13_ProbeI5OldCopyDoesNotResurrect(t *testing.T) {
	_, a, b := memPair(t)
	a.use()
	a.writeFile("memory/n6.md", "n6\n")
	a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Fb](fb.md)\n- [N6](n6.md)\n")
	now(a)
	now(b)
	a.use()
	copyDir := filepath.Join(t.TempDir(), "old")
	_ = os.MkdirAll(copyDir, 0o700)
	mem := filepath.Join(a.dir(), "memory")
	entries, _ := os.ReadDir(mem)
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(mem, e.Name()))
		_ = os.WriteFile(filepath.Join(copyDir, e.Name()), data, 0o600)
	}
	b.use()
	_ = os.Remove(filepath.Join(b.dir(), "memory", "n6.md"))
	b.writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Fb](fb.md)\n")
	now(b)
	now(a)
	a.use()
	_ = os.RemoveAll(mem)
	_ = os.Rename(copyDir, mem)
	now(a)
	now(b)
	for _, m := range []*machine{a, b} {
		m.use()
		if m.readFile("memory/n6.md") != "<missing>" || strings.Contains(m.readFile("memory/MEMORY.md"), "n6.md") {
			t.Errorf("%s: n6 back: %q / %q", m.name, m.readFile("memory/n6.md"), m.readFile("memory/MEMORY.md"))
		}
	}
}
