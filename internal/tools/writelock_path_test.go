package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/writelock"
)

// The write and edit tools, called as the model calls them (Default
// registry, Execute with JSON arguments), wait for the lock sync's
// replacements hold in the folders sync carries — a project's .execai/ and
// the personal memory — and do not wait anywhere else. Sync holding the lock
// is played by the test: while it holds it, the file must not change.
func TestTools_WriteAndEditWaitForSyncsLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	base, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no config folder here")
	}
	cfg := filepath.Join(base, "execai")
	proj := filepath.Join(home, "work", "app")
	files := map[string]string{
		"project memory":   filepath.Join(proj, ".execai", "EXECAI.md"),
		"a memory note":    filepath.Join(cfg, "memory", "fact.md"),
		"the memory index": filepath.Join(cfg, "memory", "MEMORY.md"),
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("before\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reg := Default(proj)
	call := func(name string, args map[string]any) error {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("no tool %s", name)
		}
		raw, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), raw)
		return err
	}
	for what, file := range files {
		for _, tc := range []struct {
			tool string
			args map[string]any
			want string
		}{
			{"Write", map[string]any{"path": file, "content": "written by the model\n"}, "written by the model\n"},
			{"Edit", map[string]any{"path": file, "old_string": "written by the model", "new_string": "edited by the model"}, "edited by the model\n"},
		} {
			t.Run(tc.tool+" on "+what, func(t *testing.T) {
				release := writelock.Hold(cfg)
				done := make(chan error, 1)
				go func() { done <- call(tc.tool, tc.args) }()
				select {
				case err := <-done:
					release()
					t.Fatalf("the tool wrote while sync held the lock (err %v)", err)
				case <-time.After(150 * time.Millisecond):
				}
				if data, _ := os.ReadFile(file); strings.Contains(string(data), tc.want) {
					release()
					t.Fatal("the file changed while sync held the lock")
				}
				release()
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("the tool never wrote after the lock was let go")
				}
				if data, _ := os.ReadFile(file); string(data) != tc.want {
					t.Errorf("after: %q", data)
				}
			})
		}
	}
	t.Run("a file sync does not carry does not wait", func(t *testing.T) {
		release := writelock.Hold(cfg)
		defer release()
		other := filepath.Join(proj, "main.go")
		done := make(chan error, 1)
		go func() { done <- call("Write", map[string]any{"path": other, "content": "package main\n"}) }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("a write outside the synced folders waited for sync's lock")
		}
	})
}
