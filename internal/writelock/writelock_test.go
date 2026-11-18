package writelock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// One holder at a time, across open files (as across processes); a second
// take waits for the first to let go.
func TestHold_OneAtATime(t *testing.T) {
	dir := t.TempDir()
	release := Hold(dir)
	got := make(chan time.Time, 1)
	go func() {
		r := Hold(dir)
		got <- time.Now()
		r()
	}()
	time.Sleep(50 * time.Millisecond)
	let := time.Now()
	release()
	select {
	case at := <-got:
		if at.Before(let) {
			t.Error("the second take did not wait for the first")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the second take never got the lock")
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Errorf("no lock file: %v", err)
	}
}

// Synced: the memory folder of this config directory and anything inside a
// .execai folder; nothing else.
func TestSynced(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	cfg := configDir()
	for p, want := range map[string]bool{
		filepath.Join(cfg, "memory", "fact.md"):                    true,
		filepath.Join(cfg, "memory"):                               true,
		filepath.Join(cfg, "memoryx", "fact.md"):                   false,
		filepath.Join(cfg, "config.json"):                          false,
		filepath.Join(home, "work", "app", ".execai", "EXECAI.md"): true,
		filepath.Join(home, "work", "app", "EXECAI.md"):            false,
	} {
		if got := Synced(p); got != want {
			t.Errorf("Synced(%s) = %v", p, got)
		}
	}
}
