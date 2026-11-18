package sync

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/crypto"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Replacing the server's key with this computer's (/sync on, choice 2) —
// rule П7 of the E105 loss review: what it deletes from the server is named
// before the choice, it deletes only the version it saw, and never what is
// left out here.

// overwriteSetup — A with key KA and projects Alpha (ws-1) and Beta (ws-2)
// synced; C with a key of its own and only Alpha bound, with files.
func overwriteSetup(t *testing.T) (stub *stubServer, a, c *machine, a1, a2, c1 string) {
	i18n.SetLocale("en")
	stub = newStub(t)
	stub.workspaces = []Workspace{{ID: "ws-1", Name: "Alpha"}, {ID: "ws-2", Name: "Beta"}}
	a = newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	a1, a2, c1 = t.TempDir(), t.TempDir(), t.TempDir()
	stub.mu.Lock()
	stub.bindings = []Binding{
		{AgentID: "agent-a", WorkspaceID: "ws-1", LocalPath: a1},
		{AgentID: "agent-a", WorkspaceID: "ws-2", LocalPath: a2},
		{AgentID: "agent-c", WorkspaceID: "ws-1", LocalPath: c1},
	}
	stub.mu.Unlock()
	writeProjectFile(t, a1, "EXECAI.md", "alpha from A\n", time.Time{})
	writeProjectFile(t, a2, "EXECAI.md", "beta from A\n", time.Time{})
	now(a)
	c = newMachine(t, stub, "c")
	_, _, _ = crypto.LoadOrGenerate(c.dir())
	writeProjectFile(t, c1, "EXECAI.md", "alpha from C\n", time.Time{})
	return stub, a, c, a1, a2, c1
}

// The question names the project whose memory is not on this computer
// (Beta), not the one this computer overwrites (Alpha). After choice 2 Beta
// is gone from the server only, A keeps its files, and they go back to the
// server after an edit there.
func TestOverwrite_NamesTheProjectsItDeletes(t *testing.T) {
	stub, a, c, a1, a2, c1 := overwriteSetup(t)
	steps := c.converse(c.svc.OnFlow(), password, "2")
	prompt := steps[1].Prompt
	if !strings.Contains(prompt, "\"Beta\"") || strings.Contains(prompt, "\"Alpha\"") {
		t.Fatalf("the question does not name what goes: %q", prompt)
	}
	if bl := stub.blob(KindProjectMemory, "ws-2"); bl == nil || !bl.deleted {
		t.Fatal("Beta was not deleted from the server after the person agreed")
	}
	if readProjectFile(c1, "EXECAI.md") != "alpha from C\n" {
		t.Fatalf("C's Alpha: %q", readProjectFile(c1, "EXECAI.md"))
	}
	a.converse(a.svc.OnFlow(), password, "1")
	if readProjectFile(a2, "EXECAI.md") != "beta from A\n" {
		t.Fatalf("LOSS: A's Beta: %q", readProjectFile(a2, "EXECAI.md"))
	}
	if !strings.Contains(rvAll(ProjectDir(a1)), "alpha from A\n") {
		t.Errorf("LOSS: A's Alpha is not kept: %v", rvList(ProjectDir(a1)))
	}
	writeProjectFile(t, a2, "EXECAI.md", "beta from A, edited\n", time.Time{})
	now(a)
	if bl := stub.blob(KindProjectMemory, "ws-2"); bl == nil || bl.deleted {
		t.Error("Beta did not go back to the server after an edit")
	}
}

// Only the version this exchange saw is deleted: one written meanwhile
// stays.
func TestOverwrite_DeletesOnlyTheVersionItSaw(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, c := newSide(t, stub, key), newSide(t, stub, mustKey(t))
	a.write(a.mem("MEMORY.md"), "- [Fact](fact.md)\n", time.Time{})
	a.write(a.mem("fact.md"), "v1\n", time.Time{})
	a.run(Options{})
	seen := map[string]int64{}
	shown, err := c.eng.Client.List(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range shown {
		seen[blobKey(m.Kind, m.Name)] = m.Version
	}
	a.write(a.mem("fact.md"), "v2, written meanwhile\n", time.Time{})
	a.run(Options{})
	remote, _ := c.eng.Client.List(context.Background(), 0)
	sc, _ := c.eng.collect(nil, Options{})
	var rep Report
	if err := c.eng.overwrite(context.Background(), c.st, remote, sc, seen, &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.OverwriteLeft) != 1 {
		t.Errorf("left %v, want the file written after the question", rep.OverwriteLeft)
	}
	if bl := stub.blob(KindMemory, memoryName(key, "fact.md")); bl == nil || bl.deleted {
		t.Fatal("a version written after the list was deleted")
	}
	if rep.Deleted != 1 {
		t.Errorf("deleted %d, want the index seen and not changed since", rep.Deleted)
	}
}

// A project left out here (too large) is not «missing»: choice 2 neither
// deletes it nor names it.
func TestOverwrite_LeavesWhatIsLeftOutHere(t *testing.T) {
	stub, _, c, _, _, c1 := overwriteSetup(t)
	big := make([]byte, 1500<<10)
	_, _ = rand.Read(big)
	if err := os.WriteFile(filepath.Join(ProjectDir(c1), "data.bin"), big, 0o600); err != nil {
		t.Fatal(err)
	}
	steps := c.converse(c.svc.OnFlow(), password, "2")
	if strings.Contains(steps[1].Prompt, "\"Alpha\"") {
		t.Errorf("a project left out here is named as deleted: %q", steps[1].Prompt)
	}
	if bl := stub.blob(KindProjectMemory, "ws-1"); bl == nil || bl.deleted {
		t.Error("a project left out here was deleted from the server")
	}
}

// d of the second loss probe, through the path the chat takes (/sync on,
// step by step): between the question and the answer «2» another computer
// starts a new project and edits a memory note. Only what the question
// named, at the version it saw, is deleted; the new project and the edited
// note stay, the person is told, and the next /sync on asks about them
// again: «1» deletes them, at the version shown; «2» leaves them, and then
// they are not asked about any more.
func TestOverwrite_ChangesBetweenTheQuestionAndTheAnswer(t *testing.T) {
	for _, answer := range []string{"1", "2"} {
		t.Run("then "+answer, func(t *testing.T) {
			stub, a, c, _, _, _ := overwriteSetup(t)
			a.use()
			a.writeFile("memory/MEMORY.md", "- [Note](note.md)\n")
			a.writeFile("memory/note.md", "v1\n")
			now(a)
			ctx := context.Background()
			c.use()
			f := c.svc.OnFlow()
			f.Start(ctx)
			q := f.Answer(ctx, password)
			if !strings.Contains(q.Prompt, "\"Beta\"") || strings.Contains(q.Prompt, "\"Gamma\"") {
				t.Fatalf("the question: %q", q.Prompt)
			}
			// Meanwhile, on A.
			gamma := t.TempDir()
			stub.mu.Lock()
			stub.workspaces = append(stub.workspaces, Workspace{ID: "ws-3", Name: "Gamma"})
			stub.bindings = append(stub.bindings, Binding{AgentID: "agent-a", WorkspaceID: "ws-3", LocalPath: gamma})
			stub.mu.Unlock()
			writeProjectFile(t, gamma, "EXECAI.md", "gamma from A\n", time.Time{})
			a.writeFile("memory/note.md", "v2, after the question\n")
			now(a)
			a.use()
			key, _ := crypto.LoadIdentity(a.dir())

			c.use()
			done := f.Answer(ctx, "2")
			if !done.Done() {
				t.Fatalf("not done: %+v", done)
			}
			if bl := stub.blob(KindProjectMemory, "ws-2"); bl == nil || !bl.deleted {
				t.Error("Beta, named in the question, was not deleted")
			}
			if bl := stub.blob(KindProjectMemory, "ws-3"); bl == nil || bl.deleted {
				t.Error("Gamma, started after the question, was deleted")
			}
			if bl := stub.blob(KindMemory, memoryName(key, "note.md")); bl == nil || bl.deleted {
				t.Error("a note edited after the question was deleted")
			}
			if !strings.Contains(done.Text, "Not deleted from the server, because they changed or appeared after the question") ||
				!strings.Contains(done.Text, "\"Gamma\"") {
				t.Errorf("not told: %q", done.Text)
			}

			again := c.converse(c.svc.OnFlow(), answer)
			if !strings.Contains(again[0].Text, "\"Gamma\"") || !strings.Contains(again[0].Text, "changed or appeared after the question") ||
				again[0].Prompt != i18n.T("sync.on.leftChoose") {
				t.Fatalf("not asked again: %+v", again[0])
			}
			gone := answer == "1"
			if bl := stub.blob(KindProjectMemory, "ws-3"); bl == nil || bl.deleted != gone {
				t.Errorf("Gamma after «%s»: %+v", answer, bl)
			}
			if bl := stub.blob(KindMemory, memoryName(key, "note.md")); bl == nil || bl.deleted != gone {
				t.Errorf("the note after «%s»: %+v", answer, bl)
			}
			if gone && !strings.Contains(again[1].Text, "Deleted from the server: 2.") {
				t.Errorf("deleted: %q", again[1].Text)
			}
			last := c.converse(c.svc.OnFlow())
			if strings.Contains(last[0].Text, "\"Gamma\"") {
				t.Errorf("asked about a project not bound here: %+v", last[0])
			}
			// The note left under the old key is still there after «2»: it
			// is asked about on every /sync on until it is decided (round
			// 5); after «1» nothing is left to ask about.
			if asked := strings.Contains(last[0].Text, "a memory file"); asked != !gone || last[0].Done() == asked {
				t.Errorf("after «%s», asked again: %v: %+v", answer, asked, last[0])
			}
		})
	}
}
