package main

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func cliHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", "")
	return home
}

func cliWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// snapshot lists every file and folder under root with its size.
func snapshot(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, _ := d.Info()
		size := int64(0)
		if info != nil && !d.IsDir() {
			size = info.Size()
		}
		lines = append(lines, p+"|"+strconv.FormatInt(size, 10))
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// failReader fails the test if anything tries to read an answer.
type failReader struct{ t *testing.T }

func (r failReader) Read([]byte) (int, error) {
	r.t.Error("a question was asked")
	return 0, io.EOF
}

func TestRunImport_DryRunWritesNothing(t *testing.T) {
	home := cliHome(t)
	proj := filepath.Join(home, "work", "app")
	cliWrite(t, filepath.Join(proj, "CLAUDE.md"), "prod on R5")
	cliWrite(t, filepath.Join(proj, ".cursorrules"), "answer briefly")
	cliWrite(t, filepath.Join(home, ".codex", "AGENTS.md"), "global codex rules")

	before := snapshot(t, home)
	var out bytes.Buffer
	err := runImport(importOptions{dir: proj, from: "all", dryRun: true}, failReader{t}, &out, true)
	if err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, home); after != before {
		t.Errorf("--dry-run changed files:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	for _, want := range []string{"CLAUDE.md", ".cursorrules", "~/.codex/AGENTS.md", "→ imported_"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("plan does not show %q:\n%s", want, out.String())
		}
	}
}

// In CI there is nobody to answer: no questions, the import just happens.
func TestRunImport_NoTTYNoQuestions(t *testing.T) {
	home := cliHome(t)
	proj := filepath.Join(home, "app")
	cliWrite(t, filepath.Join(proj, "CLAUDE.md"), "prod on R5")

	var out bytes.Buffer
	if err := runImport(importOptions{dir: proj, from: "all"}, failReader{t}, &out, false); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(home, ".config", "execai", "memory"))
	var imported int
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "imported_") {
			imported++
		}
	}
	if imported != 1 {
		t.Errorf("imported %d files, want 1\n%s", imported, out.String())
	}
}

// In a terminal the user is asked, and "n" leaves everything as it was.
func TestRunImport_TTYAsksAndNoMeansNo(t *testing.T) {
	home := cliHome(t)
	proj := filepath.Join(home, "app")
	cliWrite(t, filepath.Join(proj, "CLAUDE.md"), "prod on R5")

	before := snapshot(t, home)
	var out bytes.Buffer
	if err := runImport(importOptions{dir: proj, from: "all"}, strings.NewReader("n\n"), &out, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[Y/n]") {
		t.Errorf("no question in a terminal:\n%s", out.String())
	}
	if after := snapshot(t, home); after != before {
		t.Errorf("answer n changed files")
	}

	// --yes skips the question even in a terminal.
	out.Reset()
	if err := runImport(importOptions{dir: proj, from: "all", yes: true}, failReader{t}, &out, true); err != nil {
		t.Fatal(err)
	}
}

func TestRunImport_From(t *testing.T) {
	home := cliHome(t)
	proj := filepath.Join(home, "app")
	cliWrite(t, filepath.Join(proj, "CLAUDE.md"), "prod on R5")
	cliWrite(t, filepath.Join(proj, ".cursorrules"), "answer briefly")

	var out bytes.Buffer
	if err := runImport(importOptions{dir: proj, from: "cursor", dryRun: true}, failReader{t}, &out, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "CLAUDE.md") || !strings.Contains(out.String(), ".cursorrules") {
		t.Errorf("--from cursor shows the wrong files:\n%s", out.String())
	}
	if err := runImport(importOptions{dir: proj, from: "vim"}, failReader{t}, &out, false); err == nil {
		t.Error("unknown --from accepted")
	}
}
