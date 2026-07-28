// Память агента — по образцу того как это сделано в Claude Code:
//
//   ~/.config/execai/memory/
//     MEMORY.md              — индекс (всегда в контексте, короткие ссылки)
//     user_role.md           — 1 файл на факт, frontmatter + body
//     feedback_style.md
//     project_execai.md
//     reference_jenkins.md
//
// В system prompt подгружается ТОЛЬКО MEMORY.md (индекс). Отдельные файлы
// LLM читает через Read когда сочтёт релевантным. Такой дизайн держит
// контекст лёгким и не перегружает cache-prompt.
//
// Проект-специфичная память — отдельно, в CWD: EXECAI.md (одним файлом,
// scope меньше).
//
// Backward-compat: старый ~/.config/execai/EXECAI.md подгружается если
// новая структура memory/MEMORY.md ещё не создана.
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadMemory собирает текст памяти для system prompt. Возвращает пустую
// строку если ни одного источника нет.
func LoadMemory(cwd string) string {
	var b strings.Builder

	// Project memory (в CWD, одним файлом).
	for _, candidate := range []string{
		filepath.Join(cwd, "EXECAI.md"),
		filepath.Join(cwd, ".execai", "EXECAI.md"),
	} {
		if data, err := os.ReadFile(candidate); err == nil && len(data) > 0 {
			fmt.Fprintf(&b, "# Project memory (%s)\n\n%s\n\n", candidate, string(data))
			break
		}
	}

	// User memory — новая структура (index).
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
		// Fallback на старый single-file EXECAI.md.
		legacyPath := filepath.Join(base, "execai", "EXECAI.md")
		if data, err := os.ReadFile(legacyPath); err == nil && len(data) > 0 {
			fmt.Fprintf(&b, "# User memory (legacy single-file %s)\n\n%s\n\n", legacyPath, string(data))
		}
	}
	return b.String()
}

// UserMemoryDir возвращает путь до директории user-памяти (создаёт если нет).
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

// EnsureUserMemoryIndex создаёт `~/.config/execai/memory/MEMORY.md` с шаблоном
// если его ещё нет. Возвращает абсолютный путь.
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
