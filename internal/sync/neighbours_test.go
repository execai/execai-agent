package sync

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Guards: what the live probe of the loss fix confirmed and must stay so.
// Green before this change as well (they protect against a regression, they
// do not prove a fix).

// trio — three computers with ws-1 bound and synced, two files.
func trio(t *testing.T) (stub *stubServer, ms []*machine, dirs []string) {
	t.Helper()
	i18n.SetLocale("en")
	stub = newStub(t)
	stub.workspaces = []Workspace{{ID: "ws-1", Name: "Alpha"}}
	for i, n := range []string{"a", "b", "c"} {
		m := newMachine(t, stub, n)
		if i == 0 {
			m.converse(m.svc.OnFlow(), password, password)
		} else {
			m.converse(m.svc.OnFlow(), password)
		}
		ms = append(ms, m)
		dirs = append(dirs, t.TempDir())
	}
	stub.mu.Lock()
	for i, n := range []string{"a", "b", "c"} {
		stub.bindings = append(stub.bindings, Binding{AgentID: "agent-" + n, WorkspaceID: "ws-1", LocalPath: dirs[i]})
	}
	stub.mu.Unlock()
	writeProjectFile(t, dirs[0], "EXECAI.md", "build: make\n", time.Time{})
	writeProjectFile(t, dirs[0], "notes.md", "deploy from R7\n", time.Time{})
	ms[0].use()
	ms[0].writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Old](old.md)\n")
	ms[0].writeFile("memory/role.md", "Go engineer\n")
	ms[0].writeFile("memory/old.md", "to be deleted\n")
	for i := 0; i < 2; i++ {
		for _, m := range ms {
			now(m)
		}
	}
	return stub, ms, dirs
}

// A deletion mark from a 7.15 computer erases nothing on the new ones; a
// person's edit then brings the project back to every computer; three
// rounds of three computers do not grow the version (no ping-pong).
func TestGuard_DeletionMarkFrom715NoPingPong(t *testing.T) {
	stub, ms, dirs := trio(t)
	old := NewClient(stub.URL, func() string { return "tok-old" })
	if _, err := old.Delete(context.Background(), KindProjectMemory, "ws-1", serverVersion(stub)); err != nil {
		t.Fatal(err)
	}
	for _, m := range ms[1:] {
		now(m)
	}
	for _, d := range dirs {
		if readProjectFile(d, "EXECAI.md") != "build: make\n" {
			t.Fatalf("files erased in %s", d)
		}
	}
	writeProjectFile(t, dirs[1], "EXECAI.md", "build: make\nedited by a person\n", time.Time{})
	now(ms[1])
	v := serverVersion(stub)
	for i := 0; i < 3; i++ {
		for _, m := range ms {
			now(m)
		}
	}
	if serverVersion(stub) != v {
		t.Errorf("the version grew %d → %d: ping-pong", v, serverVersion(stub))
	}
	for _, d := range dirs {
		if readProjectFile(d, "EXECAI.md") != "build: make\nedited by a person\n" {
			t.Errorf("%s: %q", d, readProjectFile(d, "EXECAI.md"))
		}
	}
}

// A personal memory file deleted on one computer does not come back over
// three rounds of three computers.
func TestGuard_DeletedMemoryFileStaysDeleted(t *testing.T) {
	stub, ms, _ := trio(t)
	ms[0].use()
	_ = os.Remove(filepath.Join(ms[0].dir(), "memory", "old.md"))
	for i := 0; i < 3; i++ {
		for _, m := range ms {
			now(m)
		}
	}
	key, _ := crypto.LoadIdentity(ms[0].dir())
	if bl := stub.blob(KindMemory, memoryName(key, "old.md")); bl == nil || !bl.deleted {
		t.Error("old.md came back on the server")
	}
	for _, m := range ms {
		m.use()
		if m.readFile("memory/old.md") != "<missing>" {
			t.Errorf("old.md came back on %s", m.name)
		}
	}
}

// An agent updated in place from 7.15 or 7.17 (their sync.json, no
// sync-bases.json), the binding unchanged: three rounds delete nothing,
// bring nothing back, make no copy, send nothing.
func TestGuard_UpgradeInPlaceChangesNothing(t *testing.T) {
	stub, ms, dirs := trio(t)
	ms[0].use()
	_ = os.Remove(filepath.Join(ms[0].dir(), "memory", "old.md"))
	now(ms[0])
	for _, m := range ms {
		now(m)
	}
	versions := map[string]int64{}
	stub.mu.Lock()
	for k, b := range stub.blobs {
		versions[k] = b.version
	}
	stub.mu.Unlock()
	copies := map[string]int{}
	hist := map[string]int{}
	for _, m := range ms {
		copies[m.name] = len(filesUnder(filepath.Join(m.dir(), "sync-unsent")))
		hist[m.name] = historyCount(m.svc.Paths)
		as715(t, m)
	}
	for i := 0; i < 3; i++ {
		for _, m := range ms {
			if out := now(m); strings.Contains(out, "local-") || strings.Contains(out, "server-") || strings.Contains(out, "taken from the server") {
				t.Errorf("round %d on %s: %s", i, m.name, out)
			}
		}
	}
	stub.mu.Lock()
	for k, b := range stub.blobs {
		if versions[k] != b.version {
			t.Errorf("%s: version %d → %d", k, versions[k], b.version)
		}
	}
	stub.mu.Unlock()
	for i, m := range ms {
		m.use()
		if m.readFile("memory/old.md") != "<missing>" || m.readFile("memory/role.md") != "Go engineer\n" {
			t.Errorf("%s memory changed", m.name)
		}
		if readProjectFile(dirs[i], "EXECAI.md") != "build: make\n" || len(copiesIn(ProjectDir(dirs[i]))) != 0 {
			t.Errorf("%s project: %v", m.name, rvList(ProjectDir(dirs[i])))
		}
		if n := len(filesUnder(filepath.Join(m.dir(), "sync-unsent"))); n != copies[m.name] {
			t.Errorf("%s: copies in sync-unsent %d → %d", m.name, copies[m.name], n)
		}
		if n := historyCount(m.svc.Paths); n != hist[m.name] {
			t.Errorf("%s: history versions %d → %d", m.name, hist[m.name], n)
		}
	}
}

// Rule П8: the older agent built into ExecAI Studio shares the config
// folder and rewrites sync.json after its own exchange — without the fields
// it does not know, never touching sync-bases.json. Here it has taken a
// newer version into the same folder; the new agent then neither takes
// that for a change nor loses anything.
func TestNeighbours_OlderAgentRewritesTheState(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pB, "EXECAI.md", "build: make\nfrom B\n", time.Time{})
	now(b)
	// What the older agent on A does in its exchange: writes B's version
	// into pA and saves its sync.json with that base.
	a.use()
	writeProjectFile(t, pA, "EXECAI.md", "build: make\nfrom B\n", time.Time{})
	raw, _ := os.ReadFile(a.svc.Paths.StateFile())
	var st state715
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	it, ok, err := (&Engine{Paths: a.svc.Paths}).projectItem("ws-1", pA)
	if err != nil || !ok {
		t.Fatal(err)
	}
	st.Blobs["project-memory/ws-1"] = blobState715{Path: "ws-1", Version: serverVersion(stub), Hash: it.Hash}
	out, _ := json.Marshal(st)
	_ = os.WriteFile(a.svc.Paths.StateFile(), out, 0o600)
	v := serverVersion(stub)
	said := now(a)
	if serverVersion(stub) != v || strings.Contains(said, "local-") || strings.Contains(said, "server-") {
		t.Errorf("after the older agent's exchange: version %d → %d, said %s", v, serverVersion(stub), said)
	}
	// And a person's edit on A afterwards travels as an ordinary edit.
	writeProjectFile(t, pA, "EXECAI.md", "build: make\nfrom B\nand A\n", time.Time{})
	now(a)
	now(b)
	if readProjectFile(pB, "EXECAI.md") != "build: make\nfrom B\nand A\n" {
		t.Errorf("B: %q", readProjectFile(pB, "EXECAI.md"))
	}
}

// filesUnder — the files under dir.
func filesUnder(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}
