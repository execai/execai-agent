//go:build !windows

package uiwindow

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The launcher script of the macOS bundle, run by a real shell: the folder
// reaches execai as it is, whatever its name, and nothing in the name runs.
func TestShortcut_MacLauncherHostileNames(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	root := t.TempDir()
	dump := filepath.Join(root, "args")
	exe := filepath.Join(root, "bin dir", "exe'c$ai")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	// Stands in for execai: writes its arguments down, one per line, with
	// the line breaks of an argument shown as \n.
	fake := "#!/bin/sh\nfor a in \"$@\"; do printf '%s' \"$a\" | awk 'BEGIN{ORS=\"\\\\n\"} {print}'; printf '\\n'; done > '" + dump + "'\n"
	if err := os.WriteFile(exe, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range append(hostileNames, "new\nline") {
		cwd := filepath.Join(root, "projects", name)
		s, err := PlanShortcut(ShortcutSpec{GOOS: "darwin", Home: filepath.Join(root, "home"), ConfigDir: filepath.Join(root, "cfg"),
			Exe: exe, Cwd: cwd, Comment: "c"})
		if err != nil {
			t.Fatal(err)
		}
		var script []byte
		for _, f := range s.Files {
			if strings.HasSuffix(f.Path, "/Contents/MacOS/execai-ui") {
				script = f.Data
			}
		}
		launcher := filepath.Join(root, "launcher")
		if err := os.WriteFile(launcher, script, 0o755); err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(dump)
		cmd := exec.Command(sh, launcher)
		cmd.Dir = work
		cmd.Env = []string{"SHELL=" + sh, "PATH=" + os.Getenv("PATH"), "HOME=" + filepath.Join(root, "home")}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%q: the launcher failed: %v\n%s", name, err, out)
			continue
		}
		data, err := os.ReadFile(dump)
		if err != nil {
			t.Errorf("%q: execai was not started", name)
			continue
		}
		got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		for i := range got {
			got[i] = strings.TrimSuffix(got[i], `\n`)
			got[i] = strings.ReplaceAll(got[i], `\n`, "\n")
		}
		if want := []string{"ui", "--cwd", cwd}; !slices.Equal(got, want) {
			t.Errorf("%q: execai got %q, want %q", name, got, want)
		}
		if entries, _ := os.ReadDir(work); len(entries) != 0 {
			t.Errorf("%q: the name ran something: %v", name, entries)
		}
	}
}
