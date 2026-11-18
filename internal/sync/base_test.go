package sync

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// A project base belongs to the pair (project, folder), the folder known by
// its path and its identity (bases.go, rule П3 of the E105 loss review).

// TestDirIdentity_TellsFoldersApart — a folder removed and made again at the
// same path is another folder (ext4 gives it the same file number at
// once); a renamed one is the same; a link, a file, nothing — no identity.
func TestDirIdentity_TellsFoldersApart(t *testing.T) {
	base := t.TempDir()
	d := filepath.Join(base, "proj", ".execai")
	if err := os.MkdirAll(d, 0o700); err != nil {
		t.Fatal(err)
	}
	id1 := dirIdentity(d)
	if id1 == "" {
		t.Skip("this file system does not tell folders apart")
	}
	if dirIdentity(d) != id1 {
		t.Fatal("the same folder read twice differs")
	}
	moved := filepath.Join(base, "moved")
	if err := os.Rename(d, moved); err != nil {
		t.Fatal(err)
	}
	if dirIdentity(moved) != id1 {
		t.Error("a renamed folder is another folder")
	}
	for i := 0; i < 3; i++ {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if id := dirIdentity(d); id == "" || id == id1 {
			t.Fatalf("a folder made again at the same path: %q, before %q", id, id1)
		}
		_ = os.Remove(d)
	}
	if err := os.Symlink(moved, d); err == nil {
		if dirIdentity(d) != "" {
			t.Error("a link has an identity")
		}
	}
	f := filepath.Join(base, "file")
	_ = os.WriteFile(f, nil, 0o600)
	if dirIdentity(f) != "" || dirIdentity(filepath.Join(base, "none")) != "" {
		t.Error("a file or nothing has an identity")
	}
}

// R5 of the review: the binding moves without an exchange of that project
// (the editor panel binds without SyncProject), then a one-project exchange
// of ANOTHER project runs first. It must not tie ws-1's base to the new
// folder: the next full exchange sees the move and keeps the server's
// version.
func TestBase_OnlyRoundLeavesOtherProjectsAlone(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	p2 := t.TempDir()
	writeProjectFile(t, p2, "EXECAI.md", "beta\n", time.Time{})
	stub.mu.Lock()
	stub.workspaces = append(stub.workspaces, Workspace{ID: "ws-2", Name: "Beta"})
	stub.bindings = append(stub.bindings, Binding{AgentID: "agent-a", WorkspaceID: "ws-2", LocalPath: p2})
	stub.mu.Unlock()
	now(a)
	a.use()
	before := loadBases(a.svc.Paths).Projects["ws-1"]

	other := t.TempDir()
	writeProjectFile(t, other, "EXECAI.md", "older checkout\n", past)
	rebind(stub, 0, other) // bound from the editor panel: no exchange yet
	_ = os.RemoveAll(pA)   // the old clone is gone
	a.use()
	a.svc.SyncProject(context.Background(), "ws-2", p2, "Beta")
	if got := loadBases(a.svc.Paths).Projects["ws-1"]; !reflect.DeepEqual(got, before) {
		t.Errorf("a one-project exchange of ws-2 changed ws-1's record: %+v → %+v", before, got)
	}
	if st, _ := LoadState(a.svc.Paths); st.Projects["ws-1"] != pA {
		t.Errorf("a one-project exchange of ws-2 changed ws-1's folder: %v", st.Projects)
	}
	now(a)
	now(b)
	allA, allB := rvAll(ProjectDir(other)), rvAll(ProjectDir(pB))
	if !strings.Contains(allA+allB, "build: make\n") {
		t.Errorf("LOSS: the server's EXECAI.md is gone everywhere: A %q, B %q", allA, allB)
	}
	if readProjectFile(pB, "EXECAI.md") != "build: make\n" {
		t.Errorf("B's EXECAI.md = %q", readProjectFile(pB, "EXECAI.md"))
	}
}

// R9 of the review: state of an older agent (no sync-bases.json) and the
// binding has moved to an older checkout meanwhile: no base, the server's
// version stays.
func TestBase_UpgradeWithMovedBinding(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	a.use()
	_ = os.Remove(a.svc.Paths.BasesFile())
	other := t.TempDir()
	writeProjectFile(t, other, "EXECAI.md", "older checkout\n", past)
	rebind(stub, 0, other)
	_ = os.RemoveAll(pA)
	now(a)
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make\n" {
		t.Errorf("LOSS: B's EXECAI.md = %q", readProjectFile(pB, "EXECAI.md"))
	}
	if !strings.Contains(rvAll(ProjectDir(other)), "older checkout\n") {
		t.Errorf("the older checkout is gone: %v", rvList(ProjectDir(other)))
	}
}

// state715 — sync.json exactly as the code of tag vR7.15 (and vR7.17)
// writes it: git show vR7.15:internal/sync/state.go.
type state715 struct {
	Enabled             bool                    `json:"enabled"`
	Blobs               map[string]blobState715 `json:"blobs,omitempty"`
	LastPull            time.Time               `json:"last_pull,omitempty"`
	LastPush            time.Time               `json:"last_push,omitempty"`
	LastError           string                  `json:"last_error,omitempty"`
	MachineManifestHash string                  `json:"machine_manifest_hash,omitempty"`
	ManifestAt          time.Time               `json:"manifest_at,omitempty"`
	Projects            map[string]string       `json:"projects,omitempty"`
	ProjectNames        map[string]string       `json:"project_names,omitempty"`
	Paused              string                  `json:"paused,omitempty"`
	Unreadable          int                     `json:"unreadable,omitempty"`
	TooBig              []string                `json:"too_big,omitempty"`
	QuotaFull           bool                    `json:"quota_full,omitempty"`
}

type blobState715 struct {
	Path    string `json:"path"`
	Version int64  `json:"version"`
	Hash    string `json:"hash,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

// as715 rewrites m's sync.json the way 7.15 saves it — what it does not
// know is gone — and removes what 7.15 never writes (sync-bases.json).
func as715(t *testing.T, m *machine) {
	t.Helper()
	m.use()
	raw, err := os.ReadFile(m.svc.Paths.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	var old state715
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	out, _ := json.MarshalIndent(old, "", "  ")
	if err := os.WriteFile(m.svc.Paths.StateFile(), out, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(m.svc.Paths.BasesFile())
}

// Probe 9b, step by step: O runs 7.15 with sync on and the project synced
// in o2; o1 next to it holds an older .execai/ (an earlier checkout). The
// binding moves to o1 with no exchange (the web, or /project bind while sync
// was paused); the agent is replaced by the new one and exchanges. The
// fresh edit of M1 must stay on the server, on M1 and on M2; o1's older
// files are kept next to the server's version.
func TestBase_Probe9b_UpgradeFrom715WithBindingMoved(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	stub.workspaces = []Workspace{{ID: "ws-1", Name: "Alpha"}}
	m1 := newMachine(t, stub, "a")
	m1.converse(m1.svc.OnFlow(), password, password)
	m2 := newMachine(t, stub, "b")
	m2.converse(m2.svc.OnFlow(), password)
	o := newMachine(t, stub, "o")
	o.converse(o.svc.OnFlow(), password)
	a, b, o1, o2 := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	stub.mu.Lock()
	stub.bindings = []Binding{
		{AgentID: "agent-a", WorkspaceID: "ws-1", LocalPath: a},
		{AgentID: "agent-b", WorkspaceID: "ws-1", LocalPath: b},
		{AgentID: "agent-o", WorkspaceID: "ws-1", LocalPath: o1},
	}
	stub.mu.Unlock()
	// o1 is synced first (the earlier checkout), then the binding goes to o2.
	writeProjectFile(t, a, "EXECAI.md", "MARK-s7-main\n", time.Time{})
	writeProjectFile(t, a, "notes.md", "MARK-s7-notes\n", time.Time{})
	now(m1)
	now(m2)
	now(o)
	if readProjectFile(o1, "EXECAI.md") != "MARK-s7-main\n" {
		t.Fatal("setup: o1 did not get the project")
	}
	for _, f := range []string{"EXECAI.md", "notes.md"} {
		_ = os.Chtimes(filepath.Join(ProjectDir(o1), f), past, past) // older than the server's edit
	}
	rebind(stub, 2, o2)
	now(o)
	writeProjectFile(t, a, "EXECAI.md", "MARK-s7-main,MARK-s7-edit-M1\n", time.Time{})
	now(m1)
	now(m2)
	now(o)
	if readProjectFile(o2, "EXECAI.md") != "MARK-s7-main,MARK-s7-edit-M1\n" || readProjectFile(o1, "EXECAI.md") != "MARK-s7-main\n" {
		t.Fatalf("setup: o2 %q, o1 %q", readProjectFile(o2, "EXECAI.md"), readProjectFile(o1, "EXECAI.md"))
	}
	as715(t, o) // O ran 7.15 all along: its sync.json has no new field
	rebind(stub, 2, o1)
	out := now(o) // the new agent's first exchange
	now(m1)
	now(m2)
	want := "MARK-s7-main,MARK-s7-edit-M1\n"
	for name, dir := range map[string]string{"M1": a, "M2": b, "O:o1": o1} {
		if got := readProjectFile(dir, "EXECAI.md"); got != want {
			t.Errorf("%s: EXECAI.md = %q, want the fresh edit; O said:\n%s", name, got, out)
		}
	}
	if !strings.Contains(rvAll(ProjectDir(o1)), "]MARK-s7-main\n") {
		t.Errorf("o1's older version is not kept: %v", rvList(ProjectDir(o1)))
	}
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatal("the project is gone on the server")
	}
}

// An exchange that fails after the bindings were refreshed must not lose
// the folder of the last exchange: the record of an older agent's base is
// written first (bases.go), so the next exchange still sees the move.
func TestBase_FailedExchangeKeepsTheFolderOfTheLastOne(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	as715(t, a)
	other := t.TempDir()
	writeProjectFile(t, other, "EXECAI.md", "older checkout\n", past)
	rebind(stub, 0, other)
	stub.mu.Lock()
	stub.failList = true
	stub.mu.Unlock()
	a.use()
	if _, err := a.svc.exchange(context.Background(), Options{}, time.Second); err == nil {
		t.Fatal("the exchange did not fail")
	}
	stub.mu.Lock()
	stub.failList = false
	stub.mu.Unlock()
	if st, _ := LoadState(a.svc.Paths); st.Projects["ws-1"] != other {
		t.Fatalf("setup: the failed exchange did not save the new folder: %v", st.Projects)
	}
	_ = os.RemoveAll(pA)
	now(a)
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make\n" {
		t.Errorf("LOSS: B's EXECAI.md = %q", readProjectFile(pB, "EXECAI.md"))
	}
}

// The repository cloned again at the same path, with an older .execai/ of
// real content (a clone gives its files the time of cloning): the same
// path, another folder — no base, so the older files are not sent as an
// edit; the server's version stays here and on B, the clone's is kept next
// to it.
func TestBase_RecloneAtTheSamePathWithOlderFiles(t *testing.T) {
	_, a, b, pA, pB := lossPair(t)
	if dirIdentity(ProjectDir(pA)) == "" {
		t.Skip("this file system does not tell folders apart")
	}
	_ = os.RemoveAll(ProjectDir(pA))
	writeProjectFile(t, pA, "EXECAI.md", "build: an older checkout\n", time.Time{}) // fresh time of the clone
	writeProjectFile(t, pA, "notes.md", "deploy from R7\n", time.Time{})
	now(a)
	now(b)
	if got := readProjectFile(pB, "EXECAI.md"); got != "build: make\n" {
		t.Errorf("LOSS: B has %q", got)
	}
	if got := readProjectFile(pA, "EXECAI.md"); got != "build: make\n" {
		t.Errorf("A has %q", got)
	}
	if !hasCopyOf(ProjectDir(pA), "build: an older checkout\n") {
		t.Errorf("the clone's file is not kept: %v", copiesIn(ProjectDir(pA)))
	}
}

// The same folder bound by another path through a symbolic link (review
// M12) is the same folder: same identity, an edit travels as an edit, no
// copies.
func TestBase_SameFolderThroughALink(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	link := filepath.Join(t.TempDir(), "via-link")
	if err := os.Symlink(pA, link); err != nil {
		t.Skip("no symlinks:", err)
	}
	rebind(stub, 0, link)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	out := now(a)
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make all\n" || len(copiesIn(ProjectDir(pA))) != 0 {
		t.Errorf("B %q, A's copies %v, A said %s", readProjectFile(pB, "EXECAI.md"), copiesIn(ProjectDir(pA)), out)
	}
}
