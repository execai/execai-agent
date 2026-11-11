package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// The rest of the third round: an update from the earlier candidate's
// sync-bases.json, the time of files deciding nothing, names that differ
// only in case, the host told of a full exchange.

// v1Bases — sync-bases.json as the earlier candidate (e105-loss2) wrote it
// for machine m: format 1, identities with the device number first, no
// manifests, and its applying marks.
func v1Bases(t *testing.T, m *machine, applying map[string]BlobState) {
	t.Helper()
	m.use()
	st, err := LoadState(m.svc.Paths)
	if err != nil {
		t.Fatal(err)
	}
	cur := loadBases(m.svc.Paths)
	projects := map[string]any{}
	for ws, rec := range cur.Projects {
		b := st.Blobs[blobKey(KindProjectMemory, ws)]
		id := ""
		if rec.ID != "" {
			id = "2049:" + rec.ID
		}
		projects[ws] = map[string]any{"dir": rec.Dir, "id": id, "version": b.Version, "hash": b.Hash, "deleted": b.Deleted}
	}
	mem := ""
	if cur.Memory != "" {
		mem = "2049:" + cur.Memory
	}
	raw, _ := json.MarshalIndent(map[string]any{"v": 1, "projects": projects, "memory": mem, "applying": applying}, "", "  ")
	if err := os.WriteFile(m.svc.Paths.BasesFile(), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// An agent updated from the earlier candidate: its sync-bases.json is read
// as it is — identities with the device number match the same folders —
// and with everything the same here and on the server nothing is sent,
// copied, brought back or said; the file is then written in the new format.
func TestUpgrade_FromTheEarlierCandidatesBases(t *testing.T) {
	stub, a, b, _, pB := lossPair(t)
	b.use()
	b.writeFile("memory/MEMORY.md", "- [Role](role.md)\n")
	b.writeFile("memory/role.md", "Go engineer\n")
	now(b)
	now(a)
	v1Bases(t, b, nil)
	versions := stub.snapshot()
	hist := historyCount(b.svc.Paths)
	for i := 0; i < 2; i++ {
		out := now(b)
		for _, w := range []string{"local-", "kept", "taken from the server", "older versions", "error"} {
			if strings.Contains(out, w) {
				t.Errorf("round %d: %s", i, out)
			}
		}
	}
	for k, bl := range stub.snapshot() {
		if versions[k].version != bl.version {
			t.Errorf("%s sent", k)
		}
	}
	if historyCount(b.svc.Paths) != hist || len(copiesIn(ProjectDir(pB))) != 0 || len(b.conflictFiles()) != 0 {
		t.Error("copies made")
	}
	bs := loadBases(b.svc.Paths)
	if bs.V != basesVersion || bs.Projects["ws-1"].Files == nil || strings.Count(bs.Projects["ws-1"].ID, ":") > 1 {
		t.Errorf("not rewritten in the new format: %+v", bs.Projects["ws-1"])
	}
	// The same folder by its identity: a deletion here is a deletion.
	_ = os.Remove(filepath.Join(ProjectDir(pB), "notes.md"))
	now(b)
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	if _, ok := serverPack(t, stub, key, "ws-1")["notes.md"]; ok {
		t.Error("the folder was not recognised as the same one")
	}
}

// The earlier candidate died while writing a new version into a folder
// (its applying mark is in its sync-bases.json, sync.json still has the
// old base). The new agent reads that file without stumbling and loses
// nothing: the version on the server stays whole, and the folder gets it.
func TestUpgrade_EarlierCandidateDiedWhileWriting(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	writeProjectFile(t, pA, "plan.md", "the plan\n", time.Time{})
	_ = os.Remove(filepath.Join(ProjectDir(pA), "notes.md"))
	now(a)
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	bl := stub.blob(KindProjectMemory, "ws-1")
	v1Bases(t, b, map[string]BlobState{"ws-1": {Version: bl.version}})
	writeProjectFile(t, pB, "EXECAI.md", "build: make all\n", time.Time{}) // written before it died
	out := now(b)
	out += now(b)
	now(a)
	if strings.Contains(out, "rror") {
		t.Errorf("stumbled:\n%s", out)
	}
	srv := serverPack(t, stub, key, "ws-1")
	if srv["EXECAI.md"] != "build: make all\n" || srv["plan.md"] != "the plan\n" {
		t.Errorf("LOSS on the server: %v", srv)
	}
	for f, c := range srv {
		if readProjectFile(pB, f) != c || readProjectFile(pA, f) != c {
			t.Errorf("%s: server %q, B %q, A %q", f, c, readProjectFile(pB, f), readProjectFile(pA, f))
		}
	}
}

// The same exchanges with the files' times in the past, in the future and
// mixed come to the same result: the time of a file decides nothing.
func TestTime_TheOutcomeDoesNotDependOnTimes(t *testing.T) {
	old, late := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC)
	schemes := map[string]func(i int) time.Time{
		"as written": func(int) time.Time { return time.Time{} },
		"past":       func(int) time.Time { return old },
		"future":     func(int) time.Time { return late },
		"mixed": func(i int) time.Time {
			if i%2 == 0 {
				return old
			}
			return late
		},
		"mixed the other way": func(i int) time.Time {
			if i%2 == 1 {
				return old
			}
			return late
		},
	}
	results := map[string]string{}
	for name, mt := range schemes {
		results[name] = timeScenario(t, mt)
	}
	want := results["as written"]
	for name, got := range results {
		if got != want {
			t.Errorf("%s:\n%s\nas written:\n%s", name, got, want)
		}
	}
}

func timeScenario(t *testing.T, mt func(i int) time.Time) string {
	stub, a, b, pA, pB := lossPair(t)
	i := 0
	touch := func(path string) {
		if tm := mt(i); !tm.IsZero() {
			_ = os.Chtimes(path, tm, tm)
		}
		i++
	}
	memWrite := func(m *machine, rel, content string) {
		m.use()
		m.writeFile(rel, content)
		touch(filepath.Join(m.dir(), rel))
	}
	projWrite := func(dir, rel, content string) {
		writeProjectFile(t, dir, rel, content, time.Time{})
		touch(filepath.Join(ProjectDir(dir), rel))
	}
	memWrite(a, "memory/MEMORY.md", "- [Role](role.md)\n- [Fb](fb.md)\n- [Plan](plan.md)\n")
	memWrite(a, "memory/role.md", "Go engineer\n")
	memWrite(a, "memory/fb.md", "short answers\n")
	memWrite(a, "memory/plan.md", "the plan\n")
	memWrite(a, "permissions.json", `{"always_allowed_tools":["Read"],"always_allowed_exact":[]}`)
	now(a)
	now(b)
	// A: edits, deletions.
	memWrite(a, "memory/role.md", "Go engineer, from A\n")
	_ = os.Remove(filepath.Join(a.dir(), "memory", "fb.md"))
	memWrite(a, "permissions.json", `{"always_allowed_tools":["Read","Grep"],"always_allowed_exact":[]}`)
	projWrite(pA, "EXECAI.md", "build: make, from A\n")
	_ = os.Remove(filepath.Join(ProjectDir(pA), "notes.md"))
	now(a)
	// B: the same files changed too, others added and deleted.
	memWrite(b, "memory/role.md", "Go engineer, from B\n")
	memWrite(b, "memory/new.md", "B's new note\n")
	_ = os.Remove(filepath.Join(b.dir(), "memory", "plan.md"))
	memWrite(b, "permissions.json", `{"always_allowed_tools":["Read","Bash"],"always_allowed_exact":[]}`)
	projWrite(pB, "EXECAI.md", "build: make, from B\n")
	projWrite(pB, "b.md", "B's new file\n")
	now(b)
	now(a)
	now(b)

	var lines []string
	add := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	for _, m := range []*machine{a, b} {
		m.use()
		for _, f := range []string{"MEMORY.md", "role.md", "fb.md", "plan.md", "new.md"} {
			add("%s memory/%s %q", m.name, f, m.readFile("memory/"+f))
		}
		add("%s permissions %q", m.name, m.readFile("permissions.json"))
		var copies []string
		for _, c := range m.conflictFiles() {
			copies = append(copies, c)
		}
		sort.Strings(copies)
		add("%s unsent copies %q", m.name, copies)
		var hist []string
		_ = filepath.WalkDir(m.svc.Paths.HistoryDir(), func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				data, _ := os.ReadFile(p)
				hist = append(hist, historyNameRe.ReplaceAllString(filepath.Base(p), "$1")+"="+string(data))
			}
			return nil
		})
		sort.Strings(hist)
		add("%s history %q", m.name, hist)
	}
	for _, d := range []string{pA, pB} {
		files := rvList(ProjectDir(d))
		var contents []string
		for _, f := range files {
			name := strings.TrimPrefix(f, "/")
			if localOnly(filepath.Base(name)) {
				contents = append(contents, "copy="+readProjectFile(d, name))
				continue
			}
			contents = append(contents, name+"="+readProjectFile(d, name))
		}
		sort.Strings(contents)
		add("project %q", contents)
	}
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	for _, f := range []string{"MEMORY.md", "role.md", "fb.md", "plan.md", "new.md"} {
		add("server memory/%s %q", f, serverNote(t, stub, key, f))
	}
	add("server project %v", serverPack(t, stub, key, "ws-1"))
	return strings.Join(lines, "\n")
}

// Н-9: names that differ only in case on a file system that does not tell
// them apart (stood in by the test on any system): the server's two names
// are neither written here nor deleted or changed there; the person is
// told; everything else syncs.
func TestCase_NamesDifferingOnlyInCase(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	a.use()
	a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Role, again](Role.md)\n")
	a.writeFile("memory/role.md", "lower\n")
	a.writeFile("memory/Role.md", "Upper\n")
	writeProjectFile(t, pA, "Notes.md", "Upper notes\n", time.Time{})
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	now(a)
	old := caseInsensitive
	caseInsensitive = func(dir string) bool { return strings.HasPrefix(dir, b.base) || strings.HasPrefix(dir, pB) }
	t.Cleanup(func() { caseInsensitive = old })
	before := stub.snapshot()
	out := now(b)
	out += now(b)
	b.use()
	if _, err := os.Stat(filepath.Join(b.dir(), "memory", "Role.md")); err == nil {
		t.Error("memory: one of two names differing in case was written")
	}
	if _, err := os.Stat(filepath.Join(b.dir(), "memory", "role.md")); err == nil {
		t.Error("memory: one of two names differing in case was written")
	}
	if readProjectFile(pB, "Notes.md") != "<missing>" || readProjectFile(pB, "notes.md") != "deploy from R7\n" {
		t.Errorf("project: Notes.md %q, notes.md %q", readProjectFile(pB, "Notes.md"), readProjectFile(pB, "notes.md"))
	}
	if readProjectFile(pB, "EXECAI.md") != "build: make all\n" {
		t.Error("the rest of the project did not sync")
	}
	for k, bl := range stub.snapshot() {
		if before[k].version != bl.version || before[k].deleted != bl.deleted {
			t.Errorf("%s changed on the server", k)
		}
	}
	if !strings.Contains(out, "differing only in case") || !strings.Contains(out, "Notes.md") || !strings.Contains(out, "Role.md") {
		t.Errorf("not told:\n%s", out)
	}
	now(a)
	a.use()
	if a.readFile("memory/Role.md") != "Upper\n" || a.readFile("memory/role.md") != "lower\n" || readProjectFile(pA, "Notes.md") != "Upper notes\n" {
		t.Error("the other computer lost a name")
	}
}

// The host is told of each full exchange that went through (the chat
// writes the memory index template only then), not of a send-only one.
func TestRunner_ToldOfAFullExchange(t *testing.T) {
	_, a, _, _, _ := lossPair(t)
	a.use()
	n := 0
	h := Hooks{Exchanged: func() { n++ }}
	a.svc.background(context.Background(), h, true)
	if n != 0 {
		t.Errorf("told after a send-only exchange")
	}
	a.svc.background(context.Background(), h, false)
	if n != 1 {
		t.Errorf("told %d times after a full exchange", n)
	}
}

// In the same folder (known by its identity) going back to an older text
// is an ordinary edit: it goes up, nothing is said of a copy put back. The
// same older text in a folder trusted by its path only is taken for an old
// copy: the server's version stays, the older one goes to the history.
func TestRollback_OnlyWhereTheFolderIsNotKnownForSure(t *testing.T) {
	for _, path := range []bool{false, true} {
		t.Run(fmt.Sprintf("path only %v", path), func(t *testing.T) {
			if path {
				noIdentity(t)
			}
			stub, a, b, pA, pB := lossPair(t)
			writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
			now(a)
			now(b)
			writeProjectFile(t, pB, "EXECAI.md", "build: make\n", time.Time{}) // the earlier text, back
			out := now(b)
			a.use()
			key, _ := crypto.LoadIdentity(a.dir())
			srv := serverPack(t, stub, key, "ws-1")["EXECAI.md"]
			switch {
			case !path && (srv != "build: make\n" || strings.Contains(out, "older versions")):
				t.Errorf("the edit back to the older text did not go up: server %q\n%s", srv, out)
			case path && (srv != "build: make all\n" || readProjectFile(pB, "EXECAI.md") != "build: make all\n" ||
				!strings.Contains(out, "an older version of what the server has")):
				t.Errorf("an older copy on a path-only folder: server %q, here %q\n%s", srv, readProjectFile(pB, "EXECAI.md"), out)
			}
		})
	}
}

// A version of the project written on the server between this computer's
// reading it and sending the merge (the server answers 409): the exchange
// starts over from the comparison — both edits end up on the server, not
// the last one sent.
func TestProject_VersionWrittenMeanwhileIsComparedAgain(t *testing.T) {
	stub, a, b, _, pB := lossPair(t)
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	writeProjectFile(t, pB, "notes.md", "deploy from R7, B's edit\n", time.Time{})
	stub.mu.Lock()
	stub.onPut = func(kind, name string) {
		ct, err := seal(key, KindProjectMemory, "ws-1", mustPack(map[string][]byte{
			"EXECAI.md": []byte("build: make, A's edit meanwhile\n"), "notes.md": []byte("deploy from R7\n")}))
		if err != nil {
			t.Error(err)
			return
		}
		stub.putDirect(KindProjectMemory, "ws-1", ct)
	}
	stub.mu.Unlock()
	now(b)
	srv := serverPack(t, stub, key, "ws-1")
	if srv["EXECAI.md"] != "build: make, A's edit meanwhile\n" || srv["notes.md"] != "deploy from R7, B's edit\n" {
		t.Errorf("not both edits: %v", srv)
	}
	if readProjectFile(pB, "EXECAI.md") != "build: make, A's edit meanwhile\n" {
		t.Errorf("B: %q", readProjectFile(pB, "EXECAI.md"))
	}
}

// A folder whose identity was known and cannot be read any more (another
// file system mounted at the same path, a link put in its place) is not
// taken for the same folder: what differs there is not sent, it is kept next
// to the file and the person told.
func TestTrust_IdentityGoneIsAnotherFolder(t *testing.T) {
	stub, a, _, pA, _ := lossPair(t)
	old := dirIdentityFunc
	dirIdentityFunc = func(p string) string {
		if strings.HasPrefix(p, pA) {
			return ""
		}
		return old(p)
	}
	t.Cleanup(func() { dirIdentityFunc = old })
	writeProjectFile(t, pA, "EXECAI.md", "build: make, on the mount\n", time.Time{})
	v := serverVersion(stub)
	out := now(a)
	if serverVersion(stub) != v || readProjectFile(pA, "EXECAI.md") != "build: make\n" ||
		!hasCopyOf(ProjectDir(pA), "build: make, on the mount\n") || !strings.Contains(out, "this folder is not the one sync compared last time") {
		t.Errorf("version %d → %d, here %q, copies %v\n%s", v, serverVersion(stub), readProjectFile(pA, "EXECAI.md"), copiesIn(ProjectDir(pA)), out)
	}
}
