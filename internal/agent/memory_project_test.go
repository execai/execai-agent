package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

// A project with both files: the root EXECAI.md (project instructions,
// often under git) and .execai/EXECAI.md (the project memory that sync
// brings from the other computers). Both reach the model; the same text in
// both is given once.
func TestLoadMemory_RootAndExecaiBothLoaded(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, "EXECAI.md"), "ROOT: build with make\n")
	writeFile(t, filepath.Join(cwd, ".execai", "EXECAI.md"), "SYNCED: staging is on host b\n")

	mem := LoadMemory(cwd)
	if !strings.Contains(mem, "ROOT: build with make") {
		t.Errorf("root EXECAI.md missing:\n%s", mem)
	}
	if !strings.Contains(mem, "SYNCED: staging is on host b") {
		t.Errorf(".execai/EXECAI.md missing while a root EXECAI.md exists — memory synced from another computer is invisible:\n%s", mem)
	}

	// Copied over (/project memory) and not changed since: once, not twice.
	writeFile(t, filepath.Join(cwd, ".execai", "EXECAI.md"), "ROOT: build with make\n")
	if n := strings.Count(LoadMemory(cwd), "ROOT: build with make"); n != 1 {
		t.Errorf("the same text loaded %d times", n)
	}

	// Only one of them — as before.
	only := t.TempDir()
	writeFile(t, filepath.Join(only, ".execai", "EXECAI.md"), "ONLY SYNCED\n")
	if !strings.Contains(LoadMemory(only), "ONLY SYNCED") {
		t.Error(".execai/EXECAI.md alone is not loaded")
	}
}

// /project memory: the root EXECAI.md is copied into .execai/ and stays;
// a second run has nothing to do; an existing, different .execai/EXECAI.md
// is never overwritten; no root file — nothing happens.
func TestCopyRootToProjectMemory(t *testing.T) {
	cwd := t.TempDir()
	if res, err := CopyRootToProjectMemory(cwd); err != nil || res != RootMemoryNone {
		t.Fatalf("no root file: %v %v", res, err)
	}
	writeFile(t, ProjectInstructionsPath(cwd), "root memory\n")
	if res, err := CopyRootToProjectMemory(cwd); err != nil || res != RootMemoryCopied {
		t.Fatalf("copy: %v %v", res, err)
	}
	if got := readNonEmpty(ProjectMemoryPath(cwd)); got != "root memory\n" {
		t.Errorf(".execai/EXECAI.md = %q", got)
	}
	if got := readNonEmpty(ProjectInstructionsPath(cwd)); got != "root memory\n" {
		t.Errorf("root file changed: %q", got)
	}
	if res, _ := CopyRootToProjectMemory(cwd); res != RootMemoryAlreadyThere {
		t.Errorf("second run: %v", res)
	}
	writeFile(t, ProjectMemoryPath(cwd), "synced from another computer\n")
	if res, _ := CopyRootToProjectMemory(cwd); res != RootMemoryTargetDiffers {
		t.Errorf("different target: %v", res)
	}
	if got := readNonEmpty(ProjectMemoryPath(cwd)); got != "synced from another computer\n" {
		t.Errorf("target overwritten: %q", got)
	}
}

// The prompt sends the model to .execai/EXECAI.md and keeps it out of the
// root file unless asked.
func TestSystemPrompt_ProjectMemoryPath(t *testing.T) {
	cwd := t.TempDir()
	p := SystemPrompt(cwd, nil, "")
	if !strings.Contains(p, ProjectMemoryPath(cwd)) {
		t.Errorf("prompt does not name %s", ProjectMemoryPath(cwd))
	}
	if strings.Contains(p, "\n    "+filepath.Join(cwd, "EXECAI.md")+"\n") {
		t.Error("prompt still gives the root EXECAI.md as the project memory")
	}
}
