package sync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// An editor window is its own process: a new one (a new Service) does not
// repeat a line about a copy made earlier when nothing changed since — the
// copy is told by the exchange that made it. What is left out (a link, a
// file over the limit) is told once per process, as in the terminal: it
// stays left out until the cause is gone, so each new window says it once.
func TestNotices_ACopyIsToldOnceNotPerWindow(t *testing.T) {
	stub, a, b, pA, pB := lossPair(t)
	writeProjectFile(t, pA, "EXECAI.md", "build: make\nfrom A\n", time.Time{})
	now(a)
	writeProjectFile(t, pB, "EXECAI.md", "build: make\nfrom B, older\n", past)
	b.use()
	var first []string
	b.svc.background(context.Background(), Hooks{Notify: func(s string) { first = append(first, s) }}, false)
	if !strings.Contains(strings.Join(first, "\n"), ".local-") {
		t.Fatalf("the first window was not told about the copy: %q", first)
	}
	// A second window: a new process, nothing changed since.
	svc2, err := New(stub.URL)
	if err != nil {
		t.Fatal(err)
	}
	var second []string
	svc2.background(context.Background(), Hooks{Notify: func(s string) { second = append(second, s) }}, false)
	if len(second) != 0 {
		t.Errorf("a new window repeats: %q", second)
	}
	// Left out: once per process — the second process says it once again.
	if err := os.Symlink(t.TempDir(), filepath.Join(ProjectDir(pB), "linked")); err != nil {
		t.Skip("no symlinks:", err)
	}
	count := func(svc *Service) int {
		n := 0
		for i := 0; i < 3; i++ {
			svc.background(context.Background(), Hooks{Notify: func(s string) {
				if strings.Contains(s, "symbolic link") {
					n++
				}
			}}, i == 1)
		}
		return n
	}
	if n := count(svc2); n != 1 {
		t.Errorf("left out: told %d times in one window", n)
	}
	svc3, _ := New(stub.URL)
	if n := count(svc3); n != 1 {
		t.Errorf("left out: told %d times in a new window", n)
	}
}
