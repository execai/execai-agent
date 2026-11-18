package sync

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/writelock"
)

// The rest of the fourth round.

// N12 of the third review: a send that does not go through (the merged pack
// over the limit) records no base and writes nothing here: what the server
// never took is never a base (I1).
func TestRound4_RefusedSendRecordsNoBase(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	big := func() string {
		data := make([]byte, 600<<10)
		_, _ = rand.Read(data)
		return string(data)
	}
	writeProjectFile(t, pA, "big2.bin", big(), time.Time{})
	now(a)
	writeProjectFile(t, pB, "big1.bin", big(), time.Time{})
	b.use()
	before, _ := os.ReadFile(b.svc.Paths.BasesFile())
	v := serverVersion(stub)
	out := now(b)
	if serverVersion(stub) != v {
		t.Fatal("a pack over the limit went up")
	}
	if readProjectFile(pB, "big2.bin") != "<missing>" {
		t.Error("written here though the merge did not go up")
	}
	after, _ := os.ReadFile(b.svc.Paths.BasesFile())
	if !bytes.Equal(before, after) && strings.Contains(string(after), hashOf([]byte(readProjectFile(pB, "big1.bin")))) {
		t.Error("content the server never took became a base")
	}
	assertI1(t, stub, b)
	if !strings.Contains(out, "larger than the limit") {
		t.Errorf("not told:\n%s", out)
	}
	_ = os.Remove(filepath.Join(ProjectDir(pB), "big1.bin"))
	now(b)
	if readProjectFile(pB, "big2.bin") == "<missing>" {
		t.Error("the server's version did not come once it fits")
	}
	assertI1(t, stub, b)
}

// M21 of the third review: at the end of an exchange sync.json is written
// before sync-bases.json — stopped between the two, the bases are a step
// behind (and are rebuilt), never ahead of sync.json.
func TestRound4_StateIsSavedBeforeTheBases(t *testing.T) {
	stub, a, b, pA, _ := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	now(a)
	v := serverVersion(stub)
	old := crashPoint
	defer func() { crashPoint = old }()
	checked := false
	crashPoint = func(p string) {
		if p != "state" || checked {
			return
		}
		checked = true
		st, _ := LoadState(b.svc.Paths)
		bs := loadBases(b.svc.Paths)
		if st.Blobs[blobKey(KindProjectMemory, "ws-1")].Version != v {
			t.Error("sync.json is not saved yet at the point between the two")
		}
		if bs.Projects["ws-1"].Version == v {
			t.Error("sync-bases.json was saved first")
		}
	}
	now(b)
	if !checked {
		t.Fatal("the point between the two saves was never reached")
	}
}

// N11 of the third review: a send-only exchange takes nothing from the
// server for the memory — a note missing here (on a folder trusted by its
// path only, so not a deletion) is not written back then; the next full
// exchange does it.
func TestRound4_SendOnlyTakesNothing(t *testing.T) {
	noIdentity(t)
	stub, _, b := memPair(t)
	b.use()
	_ = os.Remove(filepath.Join(b.dir(), "memory", "role.md"))
	before := stub.snapshot()
	if _, err := b.svc.exchange(context.Background(), Options{PushOnly: true}, time.Second); err != nil {
		t.Fatal(err)
	}
	if b.readFile("memory/role.md") != "<missing>" {
		t.Error("a send-only exchange took the server's version")
	}
	for k, bl := range stub.snapshot() {
		if before[k].version != bl.version || before[k].deleted != bl.deleted {
			t.Errorf("%s changed on the server", k)
		}
	}
	now(b)
	if b.readFile("memory/role.md") != "Go engineer\n" {
		t.Error("the full exchange did not bring it back")
	}
}

// The speed of taking a big project: 300 files, four versions in a row,
// each in a few seconds at most; each history folder read once per
// exchange, not once per file.
func TestRound4_BigProjectIsTakenFast(t *testing.T) {
	_, a, b, pA, _ := lossPair(t)
	for i := 0; i < 300; i++ {
		writeProjectFile(t, pA, fmt.Sprintf("f%03d.md", i), "v0\n", time.Time{})
	}
	now(a)
	now(b)
	reads := 0
	var mu gosync.Mutex
	old := readDir
	readDir = func(dir string) ([]os.DirEntry, error) {
		mu.Lock()
		reads++
		mu.Unlock()
		return old(dir)
	}
	defer func() { readDir = old }()
	for v := 1; v <= 4; v++ {
		for i := 0; i < 300; i++ {
			writeProjectFile(t, pA, fmt.Sprintf("f%03d.md", i), fmt.Sprintf("v%d\n", v), time.Time{})
		}
		now(a)
		reads = 0
		start := time.Now()
		now(b)
		if d := time.Since(start); d > 10*time.Second {
			t.Errorf("version %d taken in %v", v, d)
		}
		if reads > 2 {
			t.Errorf("version %d: the history read %d times", v, reads)
		}
	}
	b.use()
	if n := len(historyVersions(filepath.Join(b.svc.Paths.HistoryDir(), "projects", "ws-1"), "f000.md")); n != 4 {
		t.Errorf("history of f000.md: %d versions, want all 4", n)
	}
}

// Section 4: the agent's own writers take the writers' lock, and so does
// the keeper for its last comparison and rename: lines written all the time
// while the two computers exchange are never lost — each is in a file, a
// copy or on the server at the end.
func TestRound4_WritersAndExchangesLoseNothing(t *testing.T) {
	writersTest(t, true)
}

// A writer that is not the agent's (no lock): what it can lose is at most
// the one write that lands between the keeper's last comparison and the
// rename — and no copy is ever empty.
func TestRound4_ForeignWriterLosesAtMostTheWindow(t *testing.T) {
	writersTest(t, false)
}

func writersTest(t *testing.T, locked bool) {
	stub, a, b, pA, pB := lossPair(t)
	a.use()
	keyA, _ := crypto.LoadIdentity(a.dir())
	b.use()
	bMem := filepath.Join(b.dir(), "memory")
	_ = os.MkdirAll(bMem, 0o700)
	targets := []struct {
		file, cfg, tag string
	}{
		{filepath.Join(ProjectDir(pB), "EXECAI.md"), b.dir(), "bp"},
		{filepath.Join(ProjectDir(pA), "EXECAI.md"), a.dir(), "ap"},
		{filepath.Join(bMem, "fact.md"), b.dir(), "bm"},
	}
	var wg gosync.WaitGroup
	stop := make(chan struct{})
	var mu gosync.Mutex
	var written []string
	for _, tg := range targets {
		tg := tg
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; ; n++ {
				select {
				case <-stop:
					return
				default:
				}
				line := fmt.Sprintf("- %s-%d\n", tg.tag, n)
				release := func() {}
				if locked {
					release = writelock.Hold(tg.cfg)
				}
				old, _ := os.ReadFile(tg.file)
				_ = os.WriteFile(tg.file, append(old, line...), 0o600)
				release()
				mu.Lock()
				written = append(written, strings.TrimSpace(line))
				mu.Unlock()
				// Uneven pauses: sometimes inside the keeper's window,
				// sometimes leaving it room to replace the file.
				time.Sleep(time.Duration(3+(n*7)%37) * time.Millisecond)
			}
		}()
	}
	replaced := 0
	for i := 0; i < 30; i++ {
		now(a)
		b.use()
		rep, _ := b.svc.exchange(context.Background(), Options{}, 5*time.Second)
		replaced += rep.Pulled
	}
	close(stop)
	wg.Wait()
	for i := 0; i < 3; i++ {
		now(a)
		now(b)
	}
	var all strings.Builder
	for _, root := range []string{ProjectDir(pA), ProjectDir(pB), a.dir(), b.dir()} {
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				data, _ := os.ReadFile(p)
				if strings.Contains(p, ".local-") || strings.Contains(p, "sync-unsent") {
					if len(bytes.TrimSpace(data)) == 0 {
						t.Errorf("an empty copy: %s", p)
					}
				}
				all.Write(data)
			}
			return nil
		})
	}
	all.WriteString(serverPack(t, stub, keyA, "ws-1")["EXECAI.md"])
	lost := 0
	for _, w := range written {
		if !strings.Contains(all.String(), w+"\n") {
			lost++
			t.Logf("lost: %s", w)
		}
	}
	if replaced == 0 {
		// Under heavy load the writers may keep every file changing: the
		// keeper then leaves them (changed meanwhile), which is no loss
		// either, but nothing of the window was exercised.
		t.Logf("no replacement happened in this run")
	}
	switch {
	case locked && lost > 0:
		t.Errorf("%d of %d lines lost with the lock", lost, len(written))
	case !locked && lost > replaced:
		t.Errorf("%d of %d lines lost, more than one per replacement (%d)", lost, len(written), replaced)
	}
	t.Logf("%d lines written, %d lost, %d replacements", len(written), lost, replaced)
}

// Every write of a blob names the key it is encrypted with; a write the
// server refuses because its key is another one now (replaced on another
// computer meanwhile) pauses sync with the line that says so.
func TestRound4_KeyMismatchPauses(t *testing.T) {
	i18n.SetLocale("en")
	stub, a, _ := memPair(t)
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	stub.mu.Lock()
	fps := append([]string(nil), stub.keyFPs...)
	stub.mu.Unlock()
	for _, fp := range fps {
		if fp != KeyFingerprint(key) {
			t.Fatalf("a write named key %q, not %q", fp, KeyFingerprint(key))
		}
	}
	other, _ := crypto.Generate()
	wrapped, _ := WrapIdentity(other, password)
	stub.mu.Lock()
	stub.identity.wrapped = wrapped
	stub.mu.Unlock()
	a.writeFile("memory/role.md", "Go engineer, edited\n")
	var notes []string
	a.svc.background(context.Background(), Hooks{Notify: func(s string) { notes = append(notes, s) }}, true)
	st, _ := LoadState(a.svc.Paths)
	if st.Paused != PausedKey {
		t.Errorf("not paused: %q", st.Paused)
	}
	if len(notes) == 0 || !strings.Contains(strings.Join(notes, "\n"), "sync on") {
		t.Errorf("not told: %q", notes)
	}
}

// Probe d: after «keep this computer's key» some projects stay on the
// server under the old key. The computer that has them bound is told which
// (once per process, with the way out), and /sync on asks to delete them —
// then its own versions go up again.
func TestRound4_ProjectsUnderAnotherKeyAreNamedAndAsked(t *testing.T) {
	i18n.SetLocale("en")
	stub, a, _, _, _ := lossPair(t)
	beta := t.TempDir()
	writeProjectFile(t, beta, "EXECAI.md", "beta here\n", time.Time{})
	other, _ := crypto.Generate()
	ct, _ := seal(other, KindProjectMemory, "ws-2", mustPack(map[string][]byte{"EXECAI.md": []byte("beta under the old key\n")}))
	stub.putDirect(KindProjectMemory, "ws-2", ct)
	stub.mu.Lock()
	stub.workspaces = append(stub.workspaces, Workspace{ID: "ws-2", Name: "Beta"})
	stub.bindings = append(stub.bindings, Binding{AgentID: "agent-a", WorkspaceID: "ws-2", LocalPath: beta})
	stub.mu.Unlock()
	a.use()
	var notes []string
	a.svc.background(context.Background(), Hooks{Notify: func(s string) { notes = append(notes, s) }}, false)
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "encrypted with an earlier sync key, not the one this computer has: \"Beta\"") || strings.Contains(joined, "Enter the sync password") {
		t.Errorf("told: %q", notes)
	}
	steps := a.converse(a.svc.OnFlow(), "1")
	if !strings.Contains(steps[0].Text, "\"Beta\"") || !strings.Contains(steps[1].Text, "Deleted from the server: 1.") {
		t.Fatalf("not asked or not deleted: %+v", steps)
	}
	if !strings.Contains(steps[1].Text, "sent to the server again, now encrypted with the current sync key") {
		t.Errorf("this computer's Beta going up is not said as what it is: %q", steps[1].Text)
	}
	now(a)
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	if serverPack(t, stub, key, "ws-2")["EXECAI.md"] != "beta here\n" {
		t.Error("this computer's version did not go up")
	}
}

// An agent updated from the third candidate: its sync-bases.json (format 2,
// five earlier hashes, a manifest of files kept under a deletion mark) is
// read; the kept manifest is not taken for a base; with everything the same
// nothing is sent, copied or said.
func TestRound4_UpgradeFromTheThirdCandidate(t *testing.T) {
	stub, a, b, _, pB := lossPair(t)
	b.use()
	bs := loadBases(b.svc.Paths)
	raw, _ := json.Marshal(map[string]any{"v": 2, "projects": bs.Projects, "memory": bs.Memory})
	_ = os.WriteFile(b.svc.Paths.BasesFile(), raw, 0o600)
	versions := stub.snapshot()
	for i := 0; i < 2; i++ {
		if out := now(b); strings.Contains(out, "kept") || strings.Contains(out, "local-") {
			t.Errorf("round %d: %s", i, out)
		}
	}
	for k, bl := range stub.snapshot() {
		if versions[k].version != bl.version {
			t.Errorf("%s sent", k)
		}
	}
	if len(copiesIn(ProjectDir(pB))) != 0 {
		t.Error("copies made")
	}
	if loadBases(b.svc.Paths).V != basesVersion {
		t.Error("not written in the new format")
	}
	// A kept mark of the third candidate with its manifest (content the
	// server may never have seen): not a base.
	r3Tombstone(t, stub)
	now(b)
	bs = loadBases(b.svc.Paths)
	rec := bs.Projects["ws-1"]
	rec.Files = map[string]string{"EXECAI.md": hashOf([]byte("build: make\n"))}
	bs.Projects["ws-1"] = rec
	raw, _ = json.Marshal(map[string]any{"v": 2, "projects": bs.Projects})
	_ = os.WriteFile(b.svc.Paths.BasesFile(), raw, 0o600)
	if loadBases(b.svc.Paths).Projects["ws-1"].Files != nil {
		t.Error("a kept manifest of the third candidate is read as a base")
	}
	now(a)
	assertI1(t, stub, b)
}

// A folder moved with mv keeps its number and creation time: it is the
// same folder at a new path — an edit made there goes up, no copy.
func TestRound4_MovedFolderIsTheSameFolder(t *testing.T) {
	if dirIdentity(t.TempDir()) == "" {
		t.Skip("this file system cannot tell folders apart")
	}
	stub, a, b, _, pB := lossPair(t)
	moved := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(pB, moved); err != nil {
		t.Fatal(err)
	}
	rebind(stub, 1, moved)
	writeProjectFile(t, moved, "EXECAI.md", "build: make\nedited after the move\n", time.Time{})
	out := now(b)
	now(a)
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	if serverPack(t, stub, key, "ws-1")["EXECAI.md"] != "build: make\nedited after the move\n" || len(copiesIn(ProjectDir(moved))) != 0 {
		t.Errorf("the moved folder was taken for another one:\n%s", out)
	}
	b.use()
	if rec := loadBases(b.svc.Paths).Projects["ws-1"]; !samePath(rec.Dir, moved) {
		t.Errorf("the record kept the old path: %s", rec.Dir)
	}
}

// Case: asked of the folder itself, once per folder; a folder that cannot
// be asked is guessed from the system.
func TestRound4_CaseIsAskedOfTheFolder(t *testing.T) {
	dir := t.TempDir()
	ans, ok := probeCase(dir)
	if !ok {
		t.Fatal("a writable folder could not be probed")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("the probe file stayed: %v", entries)
	}
	if got := probeCaseInsensitive(dir); got != ans {
		t.Errorf("cached %v, probed %v", got, ans)
	}
	if _, ok := probeCase(filepath.Join(dir, "missing")); ok {
		t.Error("a folder that is not there was probed")
	}
}

// An older agent writes an old pack over the project (every file at an
// earlier version): the newer versions here are kept next to the files for
// good, and the person is told (row 2b) — nothing goes only into history.
func TestRound4_OldPackFromAnOlderAgent(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	writeProjectFile(t, pA, "notes.md", "deploy from R8\n", time.Time{})
	now(a)
	now(b)
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	ct, _ := seal(key, KindProjectMemory, "ws-1", mustPack(map[string][]byte{
		"EXECAI.md": []byte("build: make\n"), "notes.md": []byte("deploy from R7\n")}))
	stub.putDirect(KindProjectMemory, "ws-1", ct)
	out := now(b)
	if !hasCopyOf(ProjectDir(pB), "build: make all\n") || !hasCopyOf(ProjectDir(pB), "deploy from R8\n") ||
		!strings.Contains(out, "came from the server in earlier versions") || !strings.Contains(out, "(files: 2)") {
		t.Errorf("the newer versions not kept next to the files, or not told: %v\n%s", rvList(ProjectDir(pB)), out)
	}
	assertI1(t, stub, b)
}

// An update from an older agent's state, the server gone ahead: with
// nothing to compare file by file the server's version stays; the line
// does not tell to copy the kept one back without «if it is your change».
func TestRound4_OldStateServerAheadSaysIfItIsYourChange(t *testing.T) {
	i18n.SetLocale("en")
	_, a, b, pA, pB := lossPair(t)
	as715(t, b)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	now(a)
	writeProjectFile(t, pB, "notes.md", "deploy from R7\nB's edit\n", time.Time{})
	out := now(b)
	if !strings.Contains(out, "holds your change") {
		t.Errorf("no «if it is your change»:\n%s", out)
	}
	if !hasCopyOf(ProjectDir(pB), "deploy from R7\nB's edit\n") {
		t.Errorf("B's edit lost: %v", rvList(ProjectDir(pB)))
	}
}

// A version displaced again while the same copy of it is already kept: no
// second copy, but the person is told again, with where it is.
func TestRound4_SameCopyAgainIsToldAgain(t *testing.T) {
	_, a, b, pA, pB := lossPair(t)
	for i, theirs := range []string{"build: make, A1\n", "build: make, A2\n"} {
		writeProjectFile(t, pA, "EXECAI.md", theirs, time.Time{})
		now(a)
		writeProjectFile(t, pB, "EXECAI.md", "build: make, mine\n", time.Time{})
		out := now(b)
		if !strings.Contains(out, "EXECAI.md.local-") {
			t.Errorf("round %d: not told:\n%s", i, out)
		}
	}
	n := 0
	for _, c := range copiesIn(ProjectDir(pB)) {
		if c == "build: make, mine\n" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("copies of the same version: %d, want 1", n)
	}
}

// A kept mark written by the third candidate (sync.json: deleted with the
// hash of the files kept here) is no base either: a fresh version over the
// mark keeps B's files next to it for good, not in the history.
func TestRound4_KeptMarkOfTheThirdCandidateIsNoBase(t *testing.T) {
	stub, a, b, _, pB := lossPair(t)
	r3Tombstone(t, stub)
	now(b)
	b.use()
	st, _ := LoadState(b.svc.Paths)
	k := blobKey(KindProjectMemory, "ws-1")
	it, _, _ := (&Engine{Paths: b.svc.Paths}).projectScan("ws-1", pB)
	bl := st.Blobs[k]
	bl.Hash = it.Hash // the third candidate marked the kept files so
	st.Blobs[k] = bl
	_ = st.Save(b.svc.Paths)
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	ct, _ := seal(key, KindProjectMemory, "ws-1", mustPack(map[string][]byte{"EXECAI.md": []byte("fresh clone, nothing yet\n")}))
	stub.putDirect(KindProjectMemory, "ws-1", ct)
	out := now(b)
	if !hasCopyOf(ProjectDir(pB), "build: make\n") {
		t.Errorf("B's EXECAI.md not kept next to the file:\n%s", out)
	}
	assertI1(t, stub, b)
}

// A file left out here (cannot be read) keeps the base it had, not the
// server's newer version: once it can be read again, its old content here
// is not taken for an edit and sent over what the server got meanwhile; it
// is the agreed version, so the newer one replaces it (row 2a).
func TestRound4_LeftOutFileKeepsItsOwnBase(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads everything")
	}
	stub, a, b, pA, pB := lossPair(t)
	notes := filepath.Join(ProjectDir(pB), "notes.md")
	if err := os.Chmod(notes, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(notes, 0o600) })
	writeProjectFile(t, pA, "notes.md", "deploy from R8\n", time.Time{})
	now(a)
	now(b)
	_ = os.Chmod(notes, 0o600)
	now(b)
	a.use()
	key, _ := crypto.LoadIdentity(a.dir())
	if got := serverPack(t, stub, key, "ws-1")["notes.md"]; got != "deploy from R8\n" {
		t.Errorf("the old content here went over the server's: %q", got)
	}
	if readProjectFile(pB, "notes.md") != "deploy from R8\n" || !anyHolds(historyOf(b), "deploy from R7\n") {
		t.Errorf("B: %q; the replaced version in the history: %v", readProjectFile(pB, "notes.md"), anyHolds(historyOf(b), "deploy from R7\n"))
	}
	assertI1(t, stub, b)
}

// A project the server lists but does not give (404 on reading it) is not
// decided in that exchange: nothing is written there at all.
func TestRound4_ListedProjectNotGivenIsLeftAlone(t *testing.T) {
	stub, a, b, pA, _ := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	now(a)
	inner := stub.Config.Handler
	puts := 0
	var mu gosync.Mutex
	stub.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/blobs/"+KindProjectMemory+"/") {
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			case http.MethodPut:
				mu.Lock()
				puts++
				mu.Unlock()
			}
		}
		inner.ServeHTTP(w, r)
	})
	now(b)
	stub.Config.Handler = inner
	if puts != 0 {
		t.Errorf("%d writes of a project the server did not give", puts)
	}
}

// The index, entry by entry: a line changed here stays over the server's
// unchanged one; a note's line both sides dropped comes back while the note
// exists; text outside note lines changed on both sides keeps this side's
// as a copy, and the person is told.
func TestRound4_IndexEntryRules(t *testing.T) {
	t.Run("a line changed here stays", func(t *testing.T) {
		_, a, b := memPair(t)
		a.use()
		a.writeFile("memory/MEMORY.md", "- [Role](role.md) — Go and Rust\n- [Fb](fb.md)\n")
		b.use()
		b.writeFile("memory/plan.md", "plan\n")
		b.writeFile("memory/MEMORY.md", "- [Role](role.md)\n- [Fb](fb.md)\n- [Plan](plan.md)\n")
		now(b)
		now(a)
		now(b)
		for _, m := range []*machine{a, b} {
			m.use()
			idx := m.readFile("memory/MEMORY.md")
			if !strings.Contains(idx, "[Role](role.md) — Go and Rust") || !strings.Contains(idx, "[Plan](plan.md)") {
				t.Errorf("%s: %q", m.name, idx)
			}
		}
	})
	t.Run("a dropped line of an existing note comes back", func(t *testing.T) {
		stub, a, b := memPair(t)
		b.use()
		b.writeFile("memory/MEMORY.md", "- [Fb](fb.md)\n")
		a.use()
		key, _ := crypto.LoadIdentity(a.dir())
		ct, _ := seal(key, KindMemory, "MEMORY.md", []byte("- [Fb](fb.md)\n- [Old agent](x.md)\n"))
		stub.putDirect(KindMemory, memoryName(key, "MEMORY.md"), ct)
		now(b)
		b.use()
		if idx := b.readFile("memory/MEMORY.md"); !strings.Contains(idx, "[Role](role.md)") {
			t.Errorf("the line of role.md, which exists, did not come back: %q", idx)
		}
	})
	t.Run("text outside note lines changed on both sides is kept", func(t *testing.T) {
		_, a, b := memPair(t)
		a.use()
		a.writeFile("memory/MEMORY.md", "# A's heading\n- [Role](role.md)\n- [Fb](fb.md)\n")
		now(a)
		b.use()
		b.writeFile("memory/MEMORY.md", "# B's heading\n- [Role](role.md)\n- [Fb](fb.md)\n")
		out := now(b)
		kept := false
		for _, c := range b.conflictFiles() {
			kept = kept || strings.Contains(c, "# B's heading")
		}
		if !kept || !strings.Contains(out, "the lines of all notes are kept") || strings.Contains(out, "copy it back over") {
			t.Errorf("B's heading not kept or not told: %v\n%s", b.conflictFiles(), out)
		}
	})
}

// A memory note another computer went back to an earlier version of: the
// newer one is kept next to it here for good, and said (row 2b).
func TestRound4_MemoryReturnElsewhere(t *testing.T) {
	stub, a, b := memPair(t)
	a.use()
	a.writeFile("memory/role.md", "Go engineer, later\n")
	now(a)
	now(b)
	a.use()
	a.writeFile("memory/role.md", "Go engineer\n")
	now(a)
	out := now(b)
	b.use()
	kept := false
	for _, c := range b.conflictFiles() {
		kept = kept || c == "Go engineer, later\n"
	}
	if b.readFile("memory/role.md") != "Go engineer\n" || !kept || !strings.Contains(out, "came from the server in an earlier version") {
		t.Errorf("B: %q, kept %v\n%s", b.readFile("memory/role.md"), kept, out)
	}
	assertI1(t, stub, b)
}

// The window itself, made wide on purpose: an agent's writer that comes
// while the keeper is between its last comparison and the rename waits for
// the lock and writes after the replacement — its line is not lost.
func TestRound4_WriterInTheWindowWaitsForTheLock(t *testing.T) {
	_, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	now(a)
	b.use()
	file := filepath.Join(ProjectDir(pB), "EXECAI.md")
	done := make(chan struct{})
	old := crashPoint
	defer func() { crashPoint = old }()
	fired := false
	crashPoint = func(p string) {
		if p != "rename" || fired {
			return
		}
		fired = true
		go func() {
			defer close(done)
			release := writelock.Hold(b.dir())
			data, _ := os.ReadFile(file)
			_ = os.WriteFile(file, append(data, "- written in the window\n"...), 0o600)
			release()
		}()
		time.Sleep(100 * time.Millisecond) // the writer would be done by now if it did not wait
	}
	now(b)
	if !fired {
		t.Fatal("the keeper never reached its last moment")
	}
	<-done
	if got := readProjectFile(pB, "EXECAI.md"); !strings.Contains(got, "- written in the window") {
		t.Errorf("the line written in the window is lost: %q", got)
	}
}

// A file that changes after the keeper made its copy and before the rename:
// the replacement does not happen, and the copy made for it is taken back —
// no stray copy, no line about one; the change is said as left for later.
func TestRound4_CopyOfAReplacementThatDidNotHappenIsTakenBack(t *testing.T) {
	_, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	now(a)
	writeProjectFile(t, pB, "EXECAI.md", "build: make, B's own\n", time.Time{}) // row 4: a copy is made
	b.use()
	old := crashPoint
	defer func() { crashPoint = old }()
	fired := false
	crashPoint = func(p string) {
		if p == "prepared" && !fired {
			fired = true
			writeProjectFile(t, pB, "EXECAI.md", "build: make, B's own, edited again\n", time.Time{})
		}
	}
	out := now(b)
	crashPoint = old
	if !fired {
		t.Fatal("the keeper never prepared a replacement")
	}
	if c := copiesIn(ProjectDir(pB)); len(c) != 0 {
		t.Errorf("a copy stayed for a replacement that did not happen: %v", c)
	}
	if strings.Contains(out, ".local-") || !strings.Contains(out, "left for the next exchange") {
		t.Errorf("told:\n%s", out)
	}
	if got := readProjectFile(pB, "EXECAI.md"); got != "build: make, B's own, edited again\n" {
		t.Errorf("the file: %q", got)
	}
}
