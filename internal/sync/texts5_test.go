package sync

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// The lines of the fifth round: what the agent prints says what happened,
// in the person's language, once.

// A second project bound to a folder that holds one is left out; the
// project that keeps the folder is still in sync, and /sync counts it as
// sync now does.
func TestTexts5_SecondProjectOfAFolderLeavesTheFirstCounted(t *testing.T) {
	stub, a, _, pA, _ := lossPair(t)
	stub.mu.Lock()
	stub.workspaces = append(stub.workspaces, Workspace{ID: "ws-2", Name: "Beta"})
	stub.bindings = append(stub.bindings, Binding{AgentID: "agent-a", WorkspaceID: "ws-2", LocalPath: pA})
	stub.mu.Unlock()
	out := now(a)
	a.use()
	st := a.svc.Status()
	if !strings.Contains(out, "projects (1)") || !strings.Contains(st, "projects (1)") {
		t.Errorf("sync now:\n%s\n/sync:\n%s", out, st)
	}
	if !strings.Contains(st, "is left out here") {
		t.Errorf("/sync does not say the second project is left out:\n%s", st)
	}
}

// Only the project left out is dropped from the count: the folder-taken
// skip names the folder of the project that stays.
func TestTexts5_ContentsDropsOnlyTheProjectLeftOut(t *testing.T) {
	live := func(ws string) BlobState { return BlobState{Path: ws, Version: 1, Hash: "h"} }
	st := &State{
		Projects: map[string]string{"ws-1": "/w/app", "ws-2": "/w/app", "ws-3": "/w/gone"},
		Blobs: map[string]BlobState{
			blobKey(KindProjectMemory, "ws-1"): live("ws-1"),
			blobKey(KindProjectMemory, "ws-2"): live("ws-2"),
			blobKey(KindProjectMemory, "ws-3"): live("ws-3"),
		},
		Skipped: []Skip{
			{Path: "/w/app", Reason: SkipFolderTaken, Detail: "Alpha\x00Beta", Project: "ws-2"},
			{Path: "/w/gone", Reason: SkipNoFolder},
		},
	}
	if n := st.Contents().Projects; n != 1 {
		t.Errorf("projects in sync: %d, want 1 (ws-1)", n)
	}
}

// A name that is a file on one side and a folder on the other is said once,
// as a thing left out is: not again on the next exchange while it lasts,
// once per process in the background; /sync lists it while it lasts.
func TestTexts5_FileAndFolderSaidOnce(t *testing.T) {
	_, _, b, _, pB := lossPair(t)
	const clash = "is a file on one side and a folder on the other"
	_ = os.Remove(filepath.Join(ProjectDir(pB), "notes.md"))
	writeProjectFile(t, pB, "notes.md/x.md", "x in a folder\n", time.Time{})
	first := now(b)
	second := now(b)
	if !strings.Contains(first, clash) || strings.Contains(second, clash) {
		t.Errorf("first:\n%s\nsecond:\n%s", first, second)
	}
	b.use()
	if st := b.svc.Status(); strings.Count(st, clash) != 1 {
		t.Errorf("/sync:\n%s", st)
	}
	var notes []string
	h := Hooks{Notify: func(s string) { notes = append(notes, s) }}
	for i := 0; i < 3; i++ {
		b.svc.background(context.Background(), h, false)
	}
	if n := strings.Count(strings.Join(notes, "\n"), clash); n != 1 {
		t.Errorf("said %d times in one process: %q", n, notes)
	}
}

// /sync: copies earlier agents left in sync-conflicts/ have a line of their
// own, not the words of this agent's unsent copies (earlier agents put
// replaced versions there too).
func TestTexts5_OlderAgentsCopiesHaveTheirOwnLine(t *testing.T) {
	_, a, _, _, _ := lossPair(t)
	a.use()
	a.writeFile("sync-conflicts/2025-01-01/fact.local-120000.md", "older agent's copy\n")
	a.writeFile("sync-conflicts/2025-01-01/fact.server-120000.md", "older agent's copy\n")
	a.writeFile("sync-unsent/2026-11-01/note.local-120000.md", "this agent's copy\n")
	st := a.svc.Status()
	unsent := "Copies of memory and settings content that was not sent: 1 — in " + homePath(a.svc.Paths.UnsentDir())
	older := "Copies kept by earlier versions of the agent: 2 — in " + homePath(a.svc.Paths.ConflictsDir())
	if !strings.Contains(st, unsent) || !strings.Contains(st, older) || strings.Count(st, "content that was not sent") != 1 {
		t.Errorf("/sync:\n%s", st)
	}
}

// The files of a line are named in alphabetical order, whatever order the
// exchange met them in.
func TestTexts5_FilesOfALineInOrder(t *testing.T) {
	i18n.SetLocale("en")
	p := Paths{ConfigDir: t.TempDir()}
	mem := p.MemoryDir()
	files := func(names ...string) []string {
		var out []string
		for _, n := range names {
			out = append(out, filepath.Join(mem, n))
		}
		return out
	}
	r := Report{
		DeletedThere: []areaCount{{kind: KindMemory, root: mem, files: files("n4.md", "n2.md", "n3.md")}},
		Resent:       []areaCount{{kind: KindMemory, root: mem, files: files("b.md", "a.md")}},
		Collisions:   []string{"Z.md", "A.md"},
	}
	for _, f := range files("y.md", "x.md") {
		r.Unsent = append(r.Unsent, unsentCopy{kind: KindMemory, root: mem, path: f, why: reasonBoth})
	}
	out := strings.Join(keepNotices(r, p), "\n")
	for _, want := range []string{
		strings.Join([]string{homePath(files("n2.md")[0]), homePath(files("n3.md")[0]), homePath(files("n4.md")[0])}, ", "),
		homePath(files("a.md")[0]) + ", " + homePath(files("b.md")[0]),
		homePath(files("x.md")[0]) + ", " + homePath(files("y.md")[0]),
		"A.md, Z.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
}

// /sync in Russian: the last error is said in Russian for the errors sync
// itself makes — another key, no key on the server, an HTTP status, no
// connection; an error it does not know stays as it is.
func TestTexts5_LastErrorInThePersonsLanguage(t *testing.T) {
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	s := &Service{Paths: Paths{ConfigDir: t.TempDir()}, Cmd: "/sync"}
	conn := (&url.Error{Op: "Get", URL: "https://api.example/agents-vbai/sync/identity", Err: errors.New("dial tcp: connection refused")}).Error()
	for raw, want := range map[string]string{
		errKeyChanged.Error():                                  "ключ синхронизации на сервере — не ключ этого компьютера",
		ErrKeyMismatch.Error():                                 "ключ синхронизации на сервере — не ключ этого компьютера",
		errNoServerKey.Error():                                 "на сервере нет ключа синхронизации",
		ErrNoIdentity.Error():                                  "на сервере нет ключа синхронизации",
		(&StatusError{Code: 404}).Error():                      "сервер синхронизации ответил ошибкой (HTTP 404)",
		(&StatusError{Code: 502, Body: "bad gateway"}).Error(): "сервер синхронизации ответил ошибкой (HTTP 502)",
		conn:                                  "не получилось связаться с сервером синхронизации",
		"open /x/.execai/a.md: something odd": "open /x/.execai/a.md: something odd",
	} {
		st := &State{Enabled: true, LastError: raw}
		if err := st.Save(s.Paths); err != nil {
			t.Fatal(err)
		}
		out := s.Status()
		if !strings.Contains(out, "Последняя ошибка: "+want) {
			t.Errorf("%q: /sync says:\n%s", raw, out)
		}
		if want != raw && strings.Contains(out, "sync:") {
			t.Errorf("%q: the English error text is shown:\n%s", raw, out)
		}
	}
}

// Row 2b: an earlier version that came from the server is said as just
// that — the cause is not known (a restore on the server, an older agent's
// pack), so no other computer is named — and in Russian in the plural for
// several files.
func TestTexts5_EarlierVersionFromTheServer(t *testing.T) {
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
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	out := now(b)
	if !strings.Contains(out, "пришли с сервера в прежних версиях: здесь теперь они") || !strings.Contains(out, "(файлов: 2)") ||
		strings.Contains(out, "другом компьютере") || strings.Contains(out, "теперь она") {
		t.Errorf("ru, two files:\n%s", out)
	}
	if !hasCopyOf(ProjectDir(pB), "build: make all\n") || !hasCopyOf(ProjectDir(pB), "deploy from R8\n") {
		t.Errorf("the newer versions are not kept: %v", rvList(ProjectDir(pB)))
	}
}

// A project marked deleted on the server comes back from this computer:
// one line for the project that says the whole of it went back up, with how
// many files — not a line about one file "deleted on the server".
func TestTexts5_ProjectBackIsOneLine(t *testing.T) {
	stub, _, b, _, pB := lossPair(t)
	r3Tombstone(t, stub)
	now(b) // the very files the mark deleted: kept, unsent
	writeProjectFile(t, pB, "new-note.md", "written after the mark\n", time.Time{})
	out := now(b)
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Fatalf("the project did not come back:\n%s", out)
	}
	want := "The project in " + homePath(pB) + " is marked deleted on the server, but this computer has"
	if strings.Count(out, want) != 1 || !strings.Contains(out, "the whole project is sent back up for all your computers (files: 3)") {
		t.Errorf("not one line for the project:\n%s", out)
	}
	if strings.Contains(out, "deleted on the server but changed here") || strings.Contains(out, "new-note.md") {
		t.Errorf("said of a file:\n%s", out)
	}
}

// historyCopies — the history versions of name in dir.
func historyCopies(t *testing.T, dir, name string) []string {
	t.Helper()
	var out []string
	for _, v := range historyVersions(dir, name) {
		out = append(out, v.path)
	}
	return out
}

// The lines of row 2c name the history copy itself: its path for one file,
// for several the deepest folder that holds all of them.
func TestTexts5_HistoryCopyIsNamed(t *testing.T) {
	_, a, b, pA, _ := lossPair(t)
	_ = os.Remove(filepath.Join(ProjectDir(pA), "notes.md"))
	now(a)
	out := now(b)
	b.use()
	area := filepath.Join(b.svc.Paths.HistoryDir(), "projects", "ws-1")
	copies := historyCopies(t, area, "notes.md")
	if len(copies) != 1 || !strings.Contains(out, "kept for 30 days: "+homePath(copies[0])+".") {
		t.Errorf("copies %v; said:\n%s", copies, out)
	}
	writeProjectFile(t, pA, "sub/x.md", "x\n", time.Time{})
	writeProjectFile(t, pA, "sub/y.md", "y\n", time.Time{})
	now(a)
	now(b)
	_ = os.RemoveAll(filepath.Join(ProjectDir(pA), "sub"))
	now(a)
	out = now(b)
	b.use()
	if len(historyCopies(t, filepath.Join(area, "sub"), "x.md")) != 1 ||
		!strings.Contains(out, "kept for 30 days in "+homePath(filepath.Join(area, "sub"))+".") {
		t.Errorf("said:\n%s", out)
	}
}

// A version the history holds already is not written again; the line names
// that copy.
func TestTexts5_HistoryCopyThereAlreadyIsNamed(t *testing.T) {
	p := Paths{ConfigDir: t.TempDir()}
	root := filepath.Join(t.TempDir(), ".execai")
	pl := place{kind: KindProjectMemory, root: root, rel: "notes.md", scope: "ws-1"}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pl.file(), []byte("v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old, err := keepHistoryCopy(p, &historyIndex{}, time.Now().Add(-time.Hour), pl, []byte("v1\n"))
	if err != nil || old == "" {
		t.Fatal(old, err)
	}
	var rep Report
	k := &keeper{paths: p, now: time.Now, rep: &rep, hist: &historyIndex{}}
	if err := k.remove(pl, hashOf([]byte("v1\n")), keepHistory, reasonDeletedThere); err != nil {
		t.Fatal(err)
	}
	if rep.History != 0 || rep.historyOf[pl.file()] != old {
		t.Errorf("history %d, named %q, want %q", rep.History, rep.historyOf[pl.file()], old)
	}
}

// /project bind of a folder with .execai/ of its own to a project marked
// deleted on the server: the line that the whole project went back up is
// the one line, without «sent to the server» before it.
func TestTexts5_BindBringingTheProjectBackIsOneLine(t *testing.T) {
	stub, _, b, _, _ := lossPair(t)
	r3Tombstone(t, stub)
	own := t.TempDir()
	writeProjectFile(t, own, "EXECAI.md", "this clone's own notes\n", time.Time{})
	rebind(stub, 1, own)
	b.use()
	out := b.svc.SyncProject(context.Background(), "ws-1", own, "Alpha")
	if !strings.Contains(out, "the whole project (one file) is sent back up") || strings.Contains(out, "sent to the server") {
		t.Errorf("bind said:\n%s", out)
	}
	if n := len(strings.Split(strings.TrimSpace(out), "\n")); n != 1 {
		t.Errorf("%d lines:\n%s", n, out)
	}
}
