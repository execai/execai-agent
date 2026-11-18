package sync

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
)

// The process dies in the middle of an exchange — after the k-th file it
// changed (k = 0…n), after the server took what was sent, before the state
// was saved — and the next exchange comes together on its own: nothing
// lost, nothing sent twice or sent half-written, no copy made twice.
//
// A death is played back as it would be: at the chosen point the disk of
// the computer (its config folder and its project folder) and the server
// are photographed; the exchange then runs to its end, and both are put
// back as photographed — what the process did after that moment never
// happened.

// tree — the files and folders under a root: relative path → content (nil
// for a folder). The exchange lock is not part of it.
type tree map[string][]byte

func snapTree(t *testing.T, root string) tree {
	t.Helper()
	out := tree{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if d.Name() == "sync.lock" {
			return nil
		}
		if d.IsDir() {
			out[rel] = nil
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[rel] = data
		return nil
	})
	return out
}

// restoreTree puts root back as snapshotted, in place: the root and the
// folders that were there keep their identity.
func restoreTree(t *testing.T, root string, img tree) {
	t.Helper()
	now := snapTree(t, root)
	for rel, data := range now {
		was, ok := img[rel]
		switch {
		case !ok:
			_ = os.RemoveAll(filepath.Join(root, rel))
		case data != nil && (was == nil || !bytes.Equal(was, data)):
			_ = os.Remove(filepath.Join(root, rel))
		}
	}
	for rel, data := range img {
		p := filepath.Join(root, rel)
		if data == nil {
			if err := os.MkdirAll(p, 0o700); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if cur, err := os.ReadFile(p); err == nil && bytes.Equal(cur, data) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func (s *stubServer) snapshot() map[string]stubBlob {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]stubBlob{}
	for k, b := range s.blobs {
		c := *b
		c.content = append([]byte(nil), b.content...)
		out[k] = c
	}
	return out
}

func (s *stubServer) restore(img map[string]stubBlob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blobs = map[string]*stubBlob{}
	for k, b := range img {
		c := b
		s.blobs[k] = &c
	}
}

// crashAt runs exchange on b and plays back its death at the n-th time it
// reaches point (1-based); false when it never got there.
func crashAt(t *testing.T, stub *stubServer, roots []string, point string, n int, exchange func()) bool {
	t.Helper()
	old := crashPoint
	defer func() { crashPoint = old }()
	seen := 0
	var disk []tree
	var server map[string]stubBlob
	crashPoint = func(p string) {
		if p != point || disk != nil {
			return
		}
		if seen++; seen == n {
			for _, r := range roots {
				disk = append(disk, snapTree(t, r))
			}
			server = stub.snapshot()
		}
	}
	exchange()
	if disk == nil {
		return false
	}
	for i, r := range roots {
		restoreTree(t, r, disk[i])
	}
	stub.restore(server)
	return true
}

// countPoints — how often one exchange reaches each point.
func countPoints(exchange func()) map[string]int {
	old := crashPoint
	defer func() { crashPoint = old }()
	n := map[string]int{}
	crashPoint = func(p string) { n[p]++ }
	exchange()
	return n
}

// crashCase — one scenario: setup makes the two computers and the change
// B's exchange applies; check looks at everything after the recovery.
type crashCase struct {
	name  string
	setup func(t *testing.T) (stub *stubServer, a, b *machine, roots []string, check func(t *testing.T))
}

func runCrashCases(t *testing.T, cases []crashCase) {
	for _, c := range cases {
		_, _, b, _, _ := c.setup(t)
		counts := countPoints(func() { now(b) })
		if counts["file"]+counts["sent"] == 0 {
			t.Fatalf("%s: setup: B's exchange changes no file and sends nothing", c.name)
		}
		type at struct {
			point string
			n     int
		}
		var points []at
		for k := 1; k <= counts["file"]; k++ {
			points = append(points, at{"file", k}) // after k-1 files
		}
		for k := 1; k <= counts["sent"]; k++ {
			points = append(points, at{"sent", k})
		}
		points = append(points, at{"save", 1})  // after every file, before the state
		points = append(points, at{"state", 1}) // sync.json saved, sync-bases.json not
		for _, p := range points {
			p := p
			t.Run(c.name+"/"+p.point+"#"+string(rune('0'+p.n)), func(t *testing.T) {
				stub, a, b, roots, check := c.setup(t)
				if !crashAt(t, stub, roots, p.point, p.n, func() { now(b) }) {
					t.Fatalf("never reached %s #%d", p.point, p.n)
				}
				// Right after the crash, before anything else: the bases
				// left on disk say only what the server holds.
				assertI1(t, stub, b)
				now(b)
				now(b)
				now(a)
				now(b)
				assertI1(t, stub, a)
				assertI1(t, stub, b)
				check(t)
			})
		}
	}
}

// historyVersionsOf — the history versions of one file of a project or of
// the memory on machine m.
func historyVersionsOf(m *machine, pl place) int {
	m.use()
	dir := filepath.Join(historyArea(m.svc.Paths, pl), filepath.Dir(filepath.FromSlash(pl.rel)))
	return len(historyVersions(dir, filepath.Base(pl.rel)))
}

func TestCrash_AfterEveryFileAndEveryStep(t *testing.T) {
	projectFiles := func(dir string) map[string]string {
		out := map[string]string{}
		for _, f := range rvList(ProjectDir(dir)) {
			out[strings.TrimPrefix(f, "/")] = readProjectFile(dir, strings.TrimPrefix(f, "/"))
		}
		return out
	}
	same := func(a, b map[string]string) bool {
		if len(a) != len(b) {
			return false
		}
		for k, v := range a {
			if b[k] != v {
				return false
			}
		}
		return true
	}
	runCrashCases(t, []crashCase{
		{
			// A's new version over B's unchanged folder: three files
			// replaced or added, one removed; nothing is sent by B.
			name: "project taken",
			setup: func(t *testing.T) (*stubServer, *machine, *machine, []string, func(t *testing.T)) {
				stub, a, b, pA, pB := lossPair(t)
				writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
				writeProjectFile(t, pA, "extra.md", "v1\n", time.Time{})
				now(a)
				now(b)
				writeProjectFile(t, pA, "extra.md", "v2\n", time.Time{})
				writeProjectFile(t, pA, "EXECAI.md", "build: make all test\n", time.Time{})
				writeProjectFile(t, pA, "plan.md", "the plan\n", time.Time{})
				_ = os.Remove(filepath.Join(ProjectDir(pA), "notes.md"))
				now(a)
				v := serverVersion(stub)
				hist := map[string]int{}
				for _, f := range []string{"EXECAI.md", "extra.md", "notes.md"} {
					hist[f] = historyVersionsOf(b, place{kind: KindProjectMemory, rel: f, scope: "ws-1"})
				}
				b.use()
				roots := []string{b.dir(), pB}
				return stub, a, b, roots, func(t *testing.T) {
					if !same(projectFiles(pA), projectFiles(pB)) || readProjectFile(pB, "plan.md") != "the plan\n" ||
						readProjectFile(pB, "notes.md") != "<missing>" {
						t.Errorf("not together: A %v, B %v", projectFiles(pA), projectFiles(pB))
					}
					if serverVersion(stub) != v {
						t.Errorf("B sent something: version %d → %d", v, serverVersion(stub))
					}
					if c := copiesIn(ProjectDir(pB)); len(c) != 0 {
						t.Errorf("copies next to the files: %v", c)
					}
					for _, f := range []string{"EXECAI.md", "extra.md", "notes.md"} {
						if n := historyVersionsOf(b, place{kind: KindProjectMemory, rel: f, scope: "ws-1"}); n != hist[f]+1 {
							t.Errorf("history of %s: %d versions, want %d", f, n, hist[f]+1)
						}
					}
				}
			},
		},
		{
			// The same, with an edit of B's own not sent yet: the merge goes
			// up once, with both sides' changes.
			name: "project merged and sent",
			setup: func(t *testing.T) (*stubServer, *machine, *machine, []string, func(t *testing.T)) {
				stub, a, b, pA, pB := lossPair(t)
				writeProjectFile(t, pA, "b.md", "b v1\n", time.Time{})
				now(a)
				now(b)
				writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
				writeProjectFile(t, pA, "plan.md", "the plan\n", time.Time{})
				_ = os.Remove(filepath.Join(ProjectDir(pA), "notes.md"))
				now(a)
				writeProjectFile(t, pB, "b.md", "b v2, B's own\n", time.Time{})
				v := serverVersion(stub)
				b.use()
				roots := []string{b.dir(), pB}
				return stub, a, b, roots, func(t *testing.T) {
					a.use()
					key, _ := crypto.LoadIdentity(a.dir())
					srv := serverPack(t, stub, key, "ws-1")
					if srv["b.md"] != "b v2, B's own\n" || srv["EXECAI.md"] != "build: make all\n" || srv["plan.md"] != "the plan\n" {
						t.Errorf("the server holds %v", srv)
					}
					if _, ok := srv["notes.md"]; ok {
						t.Error("notes.md, deleted on A, came back")
					}
					if !same(projectFiles(pA), projectFiles(pB)) || !same(projectFiles(pB), srv) {
						t.Errorf("not together: A %v, B %v", projectFiles(pA), projectFiles(pB))
					}
					if serverVersion(stub) != v+1 {
						t.Errorf("version %d → %d: want one sending", v, serverVersion(stub))
					}
					if c := copiesIn(ProjectDir(pB)); len(c) != 0 {
						t.Errorf("copies next to the files: %v", c)
					}
				}
			},
		},
		{
			// Personal memory: A edits two notes, deletes one, adds one and
			// changes the index; B unchanged.
			name: "memory taken",
			setup: func(t *testing.T) (*stubServer, *machine, *machine, []string, func(t *testing.T)) {
				stub, a, b := memPair(t)
				a.use()
				a.writeFile("memory/old.md", "old\n")
				a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Fb](fb.md)\n- [Old](old.md)\n")
				now(a)
				now(b)
				a.use()
				a.writeFile("memory/role.md", "Go and Rust\n")
				a.writeFile("memory/fb.md", "short answers, in Russian\n")
				a.writeFile("memory/new.md", "new\n")
				a.writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Fb](fb.md)\n- [New](new.md)\n")
				_ = os.Remove(filepath.Join(a.dir(), "memory", "old.md"))
				now(a)
				before := stub.snapshot()
				b.use()
				roots := []string{b.dir()}
				return stub, a, b, roots, func(t *testing.T) {
					for _, f := range []string{"MEMORY.md", "role.md", "fb.md", "new.md", "old.md"} {
						a.use()
						wa := a.readFile("memory/" + f)
						b.use()
						if wb := b.readFile("memory/" + f); wa != wb {
							t.Errorf("%s: A %q, B %q", f, wa, wb)
						}
					}
					after := stub.snapshot()
					for k, bl := range after {
						if before[k].version != bl.version || before[k].deleted != bl.deleted {
							t.Errorf("B sent %s", k)
						}
					}
					if len(b.conflictFiles()) != 0 {
						t.Errorf("unsent copies: %v", b.conflictFiles())
					}
					// The index is merged entry by entry and loses nothing:
					// no copy of it.
					for _, f := range []string{"role.md", "fb.md", "old.md"} {
						if n := historyVersionsOf(b, place{kind: KindMemory, rel: f}); n != 1 {
							t.Errorf("history of %s: %d versions, want 1", f, n)
						}
					}
				}
			},
		},
		{
			// B's whole memory folder emptied (the index too): nothing of it
			// is a deletion, everything comes back — also when the process
			// dies after the index came back and before the notes did.
			name: "memory restored",
			setup: func(t *testing.T) (*stubServer, *machine, *machine, []string, func(t *testing.T)) {
				stub, a, b := memPair(t)
				b.use()
				for _, n := range []string{"MEMORY.md", "role.md", "fb.md"} {
					_ = os.Remove(filepath.Join(b.dir(), "memory", n))
				}
				before := stub.snapshot()
				roots := []string{b.dir()}
				return stub, a, b, roots, func(t *testing.T) {
					for k, bl := range stub.snapshot() {
						if before[k].version != bl.version || bl.deleted {
							t.Errorf("%s changed on the server (deleted %v)", k, bl.deleted)
						}
					}
					for _, m := range []*machine{a, b} {
						m.use()
						if m.readFile("memory/role.md") != "Go engineer\n" || m.readFile("memory/fb.md") != "short answers\n" ||
							!strings.Contains(m.readFile("memory/MEMORY.md"), "[Role](role.md)") {
							t.Errorf("%s lost memory", m.name)
						}
					}
				}
			},
		},
		{
			// The project is marked deleted on the server and B wrote a new
			// note in it: the project comes back, sent once.
			name: "project back over a mark",
			setup: func(t *testing.T) (*stubServer, *machine, *machine, []string, func(t *testing.T)) {
				stub, a, b, _, pB := lossPair(t)
				r3Tombstone(t, stub)
				now(b)
				writeProjectFile(t, pB, "new.md", "written after the mark\n", time.Time{})
				v := serverVersion(stub)
				b.use()
				roots := []string{b.dir(), pB}
				return stub, a, b, roots, func(t *testing.T) {
					a.use()
					key, _ := crypto.LoadIdentity(a.dir())
					if serverPack(t, stub, key, "ws-1")["new.md"] != "written after the mark\n" {
						t.Error("the new note is not on the server")
					}
					if serverVersion(stub) != v+1 {
						t.Errorf("version %d → %d: want one sending", v, serverVersion(stub))
					}
				}
			},
		},
		{
			// Another computer went back to an earlier EXECAI.md: B keeps
			// the newer one next to the file, once.
			name: "project returned elsewhere",
			setup: func(t *testing.T) (*stubServer, *machine, *machine, []string, func(t *testing.T)) {
				stub, a, b, pA, pB := lossPair(t)
				writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
				now(a)
				now(b)
				writeProjectFile(t, pA, "EXECAI.md", "build: make\n", time.Time{})
				now(a)
				b.use()
				roots := []string{b.dir(), pB}
				return stub, a, b, roots, func(t *testing.T) {
					if readProjectFile(pB, "EXECAI.md") != "build: make\n" {
						t.Errorf("B: %q", readProjectFile(pB, "EXECAI.md"))
					}
					n := 0
					for _, c := range copiesIn(ProjectDir(pB)) {
						if c == "build: make all\n" {
							n++
						}
					}
					if n != 1 {
						t.Errorf("copies of the newer version: %d, want 1", n)
					}
				}
			},
		},
		{
			// Memory with a note of B's own not sent yet and a deletion of
			// B's: both go up once, A's edits come down.
			name: "memory merged and sent",
			setup: func(t *testing.T) (*stubServer, *machine, *machine, []string, func(t *testing.T)) {
				stub, a, b := memPair(t)
				a.use()
				a.writeFile("memory/role.md", "Go and Rust\n")
				now(a)
				b.use()
				b.writeFile("memory/fb.md", "short answers, B's own\n")
				_ = os.Remove(filepath.Join(b.dir(), "memory", "role.md")) // deleted here, changed there: 4b
				a.use()
				key, _ := crypto.LoadIdentity(a.dir())
				fbVer := stub.blob(KindMemory, memoryName(key, "fb.md")).version
				b.use()
				roots := []string{b.dir()}
				return stub, a, b, roots, func(t *testing.T) {
					b.use()
					if b.readFile("memory/role.md") != "Go and Rust\n" || b.readFile("memory/fb.md") != "short answers, B's own\n" {
						t.Errorf("B: role %q, fb %q", b.readFile("memory/role.md"), b.readFile("memory/fb.md"))
					}
					a.use()
					if a.readFile("memory/fb.md") != "short answers, B's own\n" {
						t.Errorf("A: fb %q", a.readFile("memory/fb.md"))
					}
					if v := stub.blob(KindMemory, memoryName(key, "fb.md")).version; v != fbVer+1 {
						t.Errorf("fb.md sent %d times", v-fbVer)
					}
					if len(b.conflictFiles()) != 0 {
						t.Errorf("unsent copies: %v", b.conflictFiles())
					}
				}
			},
		},
	})
}
