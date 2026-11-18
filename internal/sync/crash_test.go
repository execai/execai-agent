package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Partial failures (rule П9 of the E105 loss review): an exchange that
// stops between taking the server's version, writing the files and saving
// the state comes together on the next one — nothing lost, nothing half
// written sent, no copy made twice.

// B is writing A's new version of the project over its unchanged folder
// and cannot write one of the files (here: a folder stands where a file
// goes; the same as a full disk). The exchange does not fail: that project
// waits, its base stays. A send-only round must not send the half-written
// folder; once the obstacle is gone the next full one finishes the write:
// the file A deleted stays deleted, each replaced file is in the history
// once, nothing is sent.
func TestCrash_InterruptedWriteIsFinishedNotSent(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make all\n", time.Time{})
	_ = os.Remove(filepath.Join(ProjectDir(pA), "notes.md"))
	writeProjectFile(t, pA, "plan.md", "the plan\n", time.Time{})
	now(a)
	v := serverVersion(stub)
	block := filepath.Join(ProjectDir(pB), "plan.md")
	if err := os.MkdirAll(block, 0o700); err != nil {
		t.Fatal(err)
	}
	b.use()
	rep, err := b.svc.exchange(context.Background(), Options{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	for _, sk := range rep.Skipped {
		stopped = stopped || (sk.Cause == causeWrite && sk.Path == block)
	}
	if !stopped {
		t.Fatalf("setup: the write did not stop: %+v", rep.Skipped)
	}
	if readProjectFile(pB, "EXECAI.md") != "build: make all\n" || readProjectFile(pB, "notes.md") != "<missing>" {
		t.Fatalf("setup: B is not half written: %v", rvList(ProjectDir(pB)))
	}
	if _, err := b.svc.exchange(context.Background(), Options{PushOnly: true}, time.Second); err != nil {
		t.Fatal(err)
	}
	if serverVersion(stub) != v {
		t.Fatal("a half-written folder was sent")
	}
	_ = os.Remove(block)
	now(b)
	now(b)
	now(b)
	if serverVersion(stub) != v {
		t.Error("finishing the write sent something")
	}
	if readProjectFile(pB, "notes.md") != "<missing>" || readProjectFile(pB, "plan.md") != "the plan\n" ||
		readProjectFile(pB, "EXECAI.md") != "build: make all\n" {
		t.Errorf("B is not A's version: %v", rvList(ProjectDir(pB)))
	}
	if c := copiesIn(ProjectDir(pB)); len(c) != 0 {
		t.Errorf("copies next to the files: %v", c)
	}
	area := filepath.Join(b.svc.Paths.HistoryDir(), "projects", "ws-1")
	for name, old := range map[string]string{"EXECAI.md": "build: make\n", "notes.md": "deploy from R7\n"} {
		vs := historyVersions(area, name)
		if len(vs) != 1 {
			t.Errorf("history of %s: %v, want one version", name, vs)
			continue
		}
		if got, _ := os.ReadFile(vs[0].path); string(got) != old {
			t.Errorf("history of %s holds %q", name, got)
		}
	}
	now(a)
	if readProjectFile(pA, "notes.md") != "<missing>" {
		t.Error("notes.md, deleted on A, came back")
	}
}

// The process ends between writing a memory file and saving the state
// (the state is not saved): the next exchange sees the file equal to the
// server's and only records it — no conflict, no copy, nothing sent.
func TestCrash_StateNotSavedAfterAWrite(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.write(a.mem("fact.md"), "v1\n", time.Time{})
	a.run(Options{})
	b.run(Options{})
	a.write(a.mem("fact.md"), "v2\n", time.Time{})
	a.run(Options{})
	saved := *b.st
	saved.Blobs = map[string]BlobState{}
	for k, v := range b.st.Blobs {
		saved.Blobs[k] = v
	}
	b.run(Options{}) // writes v2 …
	*b.st = saved    // … and the state is lost
	before := len(b.conflictCopies())
	rep := b.run(Options{})
	if rep.Conflicts != 0 || rep.Pushed != 0 || len(b.conflictCopies()) != before {
		t.Errorf("after a lost state: %+v, copies %v", rep, b.conflictCopies())
	}
	if b.read(b.mem("fact.md")) != "v2\n" {
		t.Errorf("fact.md = %q", b.read(b.mem("fact.md")))
	}
}

// Another process holds the exchange lock: nothing is read or written,
// and the lock is not taken from it.
func TestCrash_BusyLockChangesNothing(t *testing.T) {
	_, a, _, pA, _ := lossPair(t)
	a.use()
	release, err := acquireExchange(context.Background(), a.svc.Paths.LockFile(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	writeProjectFile(t, pA, "EXECAI.md", "edit while busy\n", time.Time{})
	before, _ := os.ReadFile(a.svc.Paths.StateFile())
	if _, err := a.svc.exchange(context.Background(), Options{}, 100*time.Millisecond); err != errBusy {
		t.Fatalf("err %v, want busy", err)
	}
	after, _ := os.ReadFile(a.svc.Paths.StateFile())
	if string(before) != string(after) || readProjectFile(pA, "EXECAI.md") != "edit while busy\n" {
		t.Error("a busy exchange changed something")
	}
}
