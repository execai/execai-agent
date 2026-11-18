// Agent memory — modeled after how Claude Code does it:
//
//	~/.config/execai/memory/
//	  MEMORY.md              — index (always in context, short links)
//	  user_role.md           — 1 file per fact, frontmatter + body
//	  feedback_style.md
//	  project_execai.md
//	  reference_jenkins.md
//
// ONLY MEMORY.md (the index) is loaded into the system prompt. The LLM reads
// individual files via Read when it deems them relevant. This design keeps
// the context light and does not bloat the cache prompt.
//
// Project-specific memory is separate, in the project: .execai/EXECAI.md
// (a single file, smaller scope). It lives in .execai/ because that folder
// is what sync carries between the person's computers for a project bound
// to the web chat (internal/sync). EXECAI.md at the project root is read
// too, as the project's instructions: people keep it in git, and older
// versions wrote the project memory there. The agent no longer writes it
// unless the person asks.
//
// Backward compat: the old ~/.config/execai/EXECAI.md is loaded if the new
// memory/MEMORY.md structure has not been created yet.
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/writelock"
)

// Project memory files, relative to the project directory.
const (
	// ProjectMemoryRel — the project memory the agent writes; the same
	// path as internal/sync's ProjectMemoryRel.
	ProjectMemoryRel = ".execai/EXECAI.md"
	// ProjectInstructionsRel — EXECAI.md at the project root: the project's
	// instructions, read every turn, written only when the person asks.
	ProjectInstructionsRel = "EXECAI.md"
)

// ProjectMemoryPath — <cwd>/.execai/EXECAI.md.
func ProjectMemoryPath(cwd string) string {
	return filepath.Join(cwd, filepath.FromSlash(ProjectMemoryRel))
}

// ProjectInstructionsPath — <cwd>/EXECAI.md.
func ProjectInstructionsPath(cwd string) string {
	return filepath.Join(cwd, ProjectInstructionsRel)
}

// LoadMemory assembles the memory text for the system prompt. Returns an
// empty string if no source exists.
func LoadMemory(cwd string) string {
	var b strings.Builder

	// The project: both files when both exist — the root one may be under
	// git while .execai/ is what came from the other computers. The same
	// text in both (copied over by /project memory) goes in once.
	rootPath, memPath := ProjectInstructionsPath(cwd), ProjectMemoryPath(cwd)
	root, mem := readNonEmpty(rootPath), readNonEmpty(memPath)
	if root != "" {
		fmt.Fprintf(&b, "# Project instructions (%s)\n\n%s\n\n", rootPath, root)
	}
	if mem != "" && strings.TrimSpace(mem) != strings.TrimSpace(root) {
		fmt.Fprintf(&b, "# Project memory (%s)\n\n%s\n\n", memPath, mem)
	}

	// User memory — new structure (index).
	base, err := os.UserConfigDir()
	if err == nil {
		indexPath := filepath.Join(base, "execai", "memory", "MEMORY.md")
		if data, err := os.ReadFile(indexPath); err == nil && len(data) > 0 {
			fmt.Fprintf(&b, "# User memory index (%s)\n\n", indexPath)
			b.WriteString("Каждая строка — отдельный файл в той же папке. Читай Read'ом когда факт релевантен.\n\n")
			b.Write(data)
			b.WriteString("\n\n")
			return b.String()
		}
		// Fallback to the legacy single-file EXECAI.md.
		legacyPath := filepath.Join(base, "execai", "EXECAI.md")
		if data, err := os.ReadFile(legacyPath); err == nil && len(data) > 0 {
			fmt.Fprintf(&b, "# User memory (legacy single-file %s)\n\n%s\n\n", legacyPath, string(data))
		}
	}
	return b.String()
}

// RootMemoryCopy — what CopyRootToProjectMemory did.
type RootMemoryCopy int

const (
	// RootMemoryCopied — the root EXECAI.md is now also .execai/EXECAI.md.
	RootMemoryCopied RootMemoryCopy = iota
	// RootMemoryAlreadyThere — .execai/EXECAI.md already holds the same text.
	RootMemoryAlreadyThere
	// RootMemoryTargetDiffers — .execai/EXECAI.md exists with other text and
	// is left alone.
	RootMemoryTargetDiffers
	// RootMemoryNone — there is no root EXECAI.md (or it is blank).
	RootMemoryNone
)

// CopyRootToProjectMemory copies <cwd>/EXECAI.md to <cwd>/.execai/EXECAI.md,
// the project memory sync carries between computers. The root file stays as
// it is — it may be under git — and LoadMemory gives the same text once. An
// existing .execai/EXECAI.md is never overwritten.
func CopyRootToProjectMemory(cwd string) (RootMemoryCopy, error) {
	root := readNonEmpty(ProjectInstructionsPath(cwd))
	if root == "" {
		return RootMemoryNone, nil
	}
	target := ProjectMemoryPath(cwd)
	release := writelock.ForPath(target) // see EnsureUserMemoryIndex
	defer release()
	if cur, err := os.ReadFile(target); err == nil {
		if strings.TrimSpace(string(cur)) == strings.TrimSpace(root) {
			return RootMemoryAlreadyThere, nil
		}
		if strings.TrimSpace(string(cur)) != "" {
			return RootMemoryTargetDiffers, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return RootMemoryNone, err
	}
	if err := os.WriteFile(target, []byte(root), 0o644); err != nil {
		return RootMemoryNone, err
	}
	return RootMemoryCopied, nil
}

// readNonEmpty — the file's text; "" when it is missing, empty or blank.
func readNonEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return ""
	}
	return string(data)
}

// UserMemoryDir returns the path to the user memory directory (creating it if missing).
func UserMemoryDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "execai", "memory")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// EnsureUserMemoryIndex creates `~/.config/execai/memory/MEMORY.md` with a
// template if it does not exist yet. Returns the absolute path.
func EnsureUserMemoryIndex() (string, error) {
	dir, err := UserMemoryDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "MEMORY.md")
	// Looked at and written under one hold of the lock sync's replacements
	// take (internal/writelock): an index sync brings back cannot land
	// between the look and the template.
	release := writelock.ForPath(path)
	defer release()
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	template := `<!-- Индекс твоей памяти. Одна строка на файл. Формат:
     - [Заголовок](slug.md) — короткий хук (что там и когда пригодится)
     Тип и содержимое живут внутри каждого файла (frontmatter). -->

`
	if err := os.WriteFile(path, []byte(template), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
