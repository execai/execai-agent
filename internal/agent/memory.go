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
// Project-specific memory is separate, in CWD: EXECAI.md (a single file,
// smaller scope).
//
// Backward compat: the old ~/.config/execai/EXECAI.md is loaded if the new
// memory/MEMORY.md structure has not been created yet.
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadMemory assembles the memory text for the system prompt. Returns an
// empty string if no source exists.
func LoadMemory(cwd string) string {
	var b strings.Builder

	// Project memory (in CWD, single file).
	for _, candidate := range []string{
		filepath.Join(cwd, "EXECAI.md"),
		filepath.Join(cwd, ".execai", "EXECAI.md"),
	} {
		if data, err := os.ReadFile(candidate); err == nil && len(data) > 0 {
			fmt.Fprintf(&b, "# Project memory (%s)\n\n%s\n\n", candidate, string(data))
			break
		}
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
