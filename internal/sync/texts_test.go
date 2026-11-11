package sync

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// What the person reads says what the agent knows for sure (probe of the
// loss fix, item 4).

// /project bind in an empty folder: one line, not «received» and «was
// missing — taken» one after the other.
func TestTexts_BindInAnEmptyFolderSaysItOnce(t *testing.T) {
	stub, _, b, _, _ := lossPair(t)
	fresh := t.TempDir()
	rebind(stub, 1, fresh)
	b.use()
	out := b.svc.SyncProject(context.Background(), "ws-1", fresh, "Alpha")
	if !strings.Contains(out, "received from the server") || strings.Contains(out, "was missing") {
		t.Errorf("bind said: %q", out)
	}
	assertRestored(t, fresh)
}

// A version without some files: what another computer deleted is removed
// here too, said in one line with where its last version is kept (row 2c);
// a deletion mark of the whole project is said as what the server holds,
// not as something a person did.
func TestTexts_KeptAndRemovedSayWhatTheServerHolds(t *testing.T) {
	stub, a, b, pA, _ := lossPair(t)
	_ = os.Remove(filepath.Join(ProjectDir(pA), "notes.md"))
	now(a)
	out := now(b)
	old := NewClient(stub.URL, func() string { return "tok-old" })
	if _, err := old.Delete(context.Background(), KindProjectMemory, "ws-1", stub.blob(KindProjectMemory, "ws-1").version); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "is not in the version that came from the server") || !strings.Contains(out, "notes.md") ||
		!strings.Contains(out, "sync-history") {
		t.Errorf("a removal is not said with where the last version is:\n%s", out)
	}
	out = now(b)
	if strings.Contains(out, "changed both here and on another computer") || !strings.Contains(out, "marked deleted") {
		t.Errorf("not told:\n%s", out)
	}
}

// Two folders of one computer in turn (no other computer involved): the
// line does not speak of another computer having changed the project; it
// says why this folder's edit was not sent (row 3b).
func TestTexts_TwoFoldersOfOneComputer(t *testing.T) {
	stub, a, _, pA, _ := lossPair(t)
	second := t.TempDir()
	rebind(stub, 0, second)
	now(a)
	writeProjectFile(t, second, "EXECAI.md", "build: make test\n", time.Time{})
	now(a)
	writeProjectFile(t, pA, "EXECAI.md", "older, edited later\n", time.Time{})
	rebind(stub, 0, pA)
	out := now(a)
	if strings.Contains(out, "on another computer") {
		t.Errorf("speaks of another computer:\n%s", out)
	}
	if !strings.Contains(out, "this folder is not the one sync compared last time") {
		t.Errorf("not told:\n%s", out)
	}
}

// /sync: «projects (N)» in the line of what syncs and «Projects bound to
// this computer: M» mean what they say — a bound project without files yet
// is bound, not in sync.
func TestTexts_StatusCountsMeanWhatTheySay(t *testing.T) {
	stub, a, _, _, _ := lossPair(t)
	empty := t.TempDir()
	stub.mu.Lock()
	stub.workspaces = append(stub.workspaces, Workspace{ID: "ws-2", Name: "Beta"})
	stub.bindings = append(stub.bindings, Binding{AgentID: "agent-a", WorkspaceID: "ws-2", LocalPath: empty})
	stub.mu.Unlock()
	out := now(a)
	a.use()
	st := a.svc.Status()
	if !strings.Contains(out, "projects (1)") || !strings.Contains(st, "projects (1)") ||
		!strings.Contains(st, "Projects bound to this computer: 2 (Alpha, Beta)") {
		t.Errorf("summary:\n%s\nstatus:\n%s", out, st)
	}
	// A project left out (its .execai/ a link) is not counted as in sync.
	pA := stub.bindings[0].LocalPath
	away := t.TempDir()
	if err := os.Rename(ProjectDir(pA), filepath.Join(away, ".execai")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(away, ".execai"), ProjectDir(pA)); err != nil {
		t.Skip("no symbolic links here")
	}
	out = now(a)
	a.use()
	st = a.svc.Status()
	if !strings.Contains(out, "projects (0)") && strings.Contains(out, "projects (") || !strings.Contains(st, "Projects bound to this computer: 2") ||
		strings.Contains(st, "projects (1)") {
		t.Errorf("a project left out is counted:\n%s\nstatus:\n%s", out, st)
	}
}

// Paths under the home folder are shown with ~.
func TestTexts_HomeIsTilde(t *testing.T) {
	i18n.SetLocale("en")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	p := filepath.Join(home, "work", "app", ".execai")
	if got := homePath(p); got != "~"+string(filepath.Separator)+filepath.Join("work", "app", ".execai") {
		t.Errorf("homePath = %q", got)
	}
	if got := homePath("/elsewhere/x"); got != "/elsewhere/x" {
		t.Errorf("outside home: %q", got)
	}
	line := skipLine(Skip{Path: p, Reason: SkipLink})
	if !strings.Contains(line, "~") || strings.Contains(line, home) {
		t.Errorf("skip line: %q", line)
	}
}

// An exchange stopped by a file here does not say the server could not be
// reached.
func TestTexts_LocalErrorIsNotTheServer(t *testing.T) {
	i18n.SetLocale("en")
	s := &Service{Cmd: "/sync"}
	err := &fs.PathError{Op: "open", Path: "/x/.execai/private", Err: fs.ErrPermission}
	if got := s.failText(err); strings.Contains(got, "Could not reach") || !strings.Contains(got, "stopped on this computer") {
		t.Errorf("failText = %q", got)
	}
	if got := s.failText(&StatusError{Code: 502}); !strings.Contains(got, "Could not reach") {
		t.Errorf("a server error: %q", got)
	}
}

// /project bind in a folder whose file differs from the server's and that
// has a file of its own (s6 of the second loss probe): no line «received»
// next to the line that says the server's version is used here, and no
// line «merged» either — one line for one event (fourth probe); the file
// of its own that went up is said and named (fifth probe).
func TestTexts_BindSaysWhatWentEachWay(t *testing.T) {
	stub, _, b, _, _ := lossPair(t)
	fresh := t.TempDir()
	writeProjectFile(t, fresh, "EXECAI.md", "build: make, this clone\n", time.Time{})
	rebind(stub, 1, fresh)
	b.use()
	out := b.svc.SyncProject(context.Background(), "ws-1", fresh, "Alpha")
	if strings.Contains(out, "received from the server.") || !strings.Contains(out, "this folder is not the one sync compared last time") {
		t.Errorf("only a copy kept, bind said:\n%s", out)
	}
	if n := len(strings.Split(strings.TrimSpace(out), "\n")); n != 1 {
		t.Errorf("%d lines:\n%s", n, out)
	}
	other := t.TempDir()
	writeProjectFile(t, other, "EXECAI.md", "build: make, another clone\n", time.Time{})
	writeProjectFile(t, other, "own.md", "only in this clone\n", time.Time{})
	rebind(stub, 1, other)
	out = b.svc.SyncProject(context.Background(), "ws-1", other, "Alpha")
	if strings.Contains(out, "merged with the server") || !strings.Contains(out, "this folder is not the one sync compared last time") ||
		!strings.Contains(out, "a file the server did not have went up") || !strings.Contains(out, "own.md") {
		t.Errorf("the copy kept and own.md sent, bind said:\n%s", out)
	}
	if n := len(strings.Split(strings.TrimSpace(out), "\n")); n != 2 {
		t.Errorf("%d lines:\n%s", n, out)
	}
}
