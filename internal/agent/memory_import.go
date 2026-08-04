// Импорт памяти из файлов других агентов и экспорт своей.
//
// Зачем это раньше синка: мы выбрали схему, в которой ключ шифрования есть
// только у пользователя, а значит восстановления не существует. Импорт
// превращает потерю ключа из катастрофы в неудобство — память собирается
// заново из того, что и так лежит в репозитории.
//
// Побочная польза: человек, пришедший с Claude Code или Cursor, получает
// работающий контекст в первую минуту, а не начинает с чистого листа.
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ImportSource — один найденный файл чужой памяти.
type ImportSource struct {
	// Path — абсолютный путь к файлу.
	Path string
	// Tool — чей это формат: понятное имя для показа пользователю.
	Tool string
	// Rel — путь относительно каталога поиска, для компактного вывода.
	Rel string
	// SizeBytes — размер: помогает отличить содержательный файл от заглушки.
	SizeBytes int64
	// Preview — первые строки, чтобы человек понял, что именно импортирует.
	Preview string
}

// knownMemoryFiles — где другие агенты держат свои инструкции и память.
// Порядок важен: он определяет, в каком виде это увидит пользователь.
var knownMemoryFiles = []struct {
	rel  string
	tool string
	dir  bool
}{
	{"CLAUDE.md", "Claude Code", false},
	{".claude/CLAUDE.md", "Claude Code", false},
	{".claude/memory", "Claude Code (память)", true},
	{"AGENTS.md", "AGENTS.md (общий стандарт)", false},
	{".cursorrules", "Cursor", false},
	{".cursor/rules", "Cursor (правила)", true},
	{".github/copilot-instructions.md", "GitHub Copilot", false},
	{"EXECAI.md", "execai (проектная память)", false},
	{".windsurfrules", "Windsurf", false},
	{"GEMINI.md", "Gemini CLI", false},
}

// maxPreviewLines — сколько строк показывать в предпросмотре.
const maxPreviewLines = 3

// maxImportFileSize — файлы больше этого не импортируем: обычно это
// не память, а случайно попавший дамп или логи.
const maxImportFileSize = 256 * 1024

// DiscoverMemorySources ищет в каталоге файлы памяти чужих агентов.
// Ничего не читает в память целиком — только заголовки для предпросмотра.
func DiscoverMemorySources(dir string) []ImportSource {
	var out []ImportSource
	seen := map[string]bool{}

	add := func(path, tool string) {
		abs, err := filepath.Abs(path)
		if err != nil || seen[abs] {
			return
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > maxImportFileSize {
			return
		}
		seen[abs] = true
		rel, err := filepath.Rel(dir, abs)
		if err != nil {
			rel = abs
		}
		out = append(out, ImportSource{
			Path: abs, Tool: tool, Rel: rel,
			SizeBytes: info.Size(),
			Preview:   previewOf(abs),
		})
	}

	for _, k := range knownMemoryFiles {
		full := filepath.Join(dir, k.rel)
		if !k.dir {
			add(full, k.tool)
			continue
		}
		// Каталог: берём markdown-файлы внутри, но не спускаемся глубже —
		// рекурсия по .claude/ утащила бы служебные файлы и сессии.
		entries, err := os.ReadDir(full)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".mdc") {
				add(filepath.Join(full, name), k.tool)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out
}

// previewOf возвращает первые содержательные строки файла.
func previewOf(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		// Пропускаем пустые и разделители frontmatter — они ничего не говорят.
		if l == "" || l == "---" {
			continue
		}
		lines = append(lines, l)
		if len(lines) >= maxPreviewLines {
			break
		}
	}
	return strings.Join(lines, " · ")
}

// ImportResult — что получилось после импорта.
type ImportResult struct {
	Imported []string // имена созданных файлов памяти
	Skipped  []string // что пропущено и почему
}

// ImportSources переносит выбранные файлы в память агента.
//
// Каждый источник становится отдельным файлом памяти с frontmatter, в котором
// записано происхождение: через полгода будет непонятно, откуда взялся текст,
// если этого не сохранить. Существующие файлы не перезаписываются.
func ImportSources(srcs []ImportSource) (ImportResult, error) {
	var res ImportResult
	dir, err := UserMemoryDir()
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return res, fmt.Errorf("создание каталога памяти: %w", err)
	}

	for _, s := range srcs {
		body, err := os.ReadFile(s.Path)
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (не прочитать: %v)", s.Rel, err))
			continue
		}
		name := importedFileName(s)
		target := filepath.Join(dir, name)
		if _, err := os.Stat(target); err == nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (файл %s уже есть)", s.Rel, name))
			continue
		}
		content := fmt.Sprintf(`---
name: %s
description: "Импортировано из %s (%s)"
metadata:
  type: project
  imported_from: %s
  imported_tool: %s
---

%s`, strings.TrimSuffix(name, ".md"), s.Tool, s.Rel, s.Path, s.Tool, string(body))

		if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (не записать: %v)", s.Rel, err))
			continue
		}
		res.Imported = append(res.Imported, name)
	}

	if len(res.Imported) > 0 {
		if err := appendToIndex(dir, srcs, res.Imported); err != nil {
			return res, err
		}
	}
	return res, nil
}

// importedFileName делает имя файла памяти из пути источника.
func importedFileName(s ImportSource) string {
	base := strings.ToLower(s.Rel)
	base = strings.TrimSuffix(base, ".md")
	base = strings.TrimSuffix(base, ".mdc")
	repl := strings.NewReplacer("/", "_", "\\", "_", ".", "_", " ", "_")
	base = strings.Trim(repl.Replace(base), "_")
	if base == "" {
		base = "memory"
	}
	return "imported_" + base + ".md"
}

// appendToIndex дописывает строки в MEMORY.md. Индекс — единственное, что
// всегда попадает в системный промт, поэтому запись без строки в нём для
// агента невидима.
func appendToIndex(dir string, srcs []ImportSource, imported []string) error {
	path := filepath.Join(dir, "MEMORY.md")
	byName := map[string]ImportSource{}
	for _, s := range srcs {
		byName[importedFileName(s)] = s
	}

	var b strings.Builder
	existing, err := os.ReadFile(path)
	if err == nil {
		b.Write(existing)
		if !strings.HasSuffix(string(existing), "\n") {
			b.WriteString("\n")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("чтение индекса: %w", err)
	}

	for _, name := range imported {
		s := byName[name]
		fmt.Fprintf(&b, "- [📥 %s](%s) — импортировано из %s.\n",
			strings.TrimSuffix(strings.TrimPrefix(name, "imported_"), ".md"), name, s.Tool)
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// ExportMemory складывает всю память в каталог обычными markdown-файлами.
// Нужно, чтобы память не была заложником нашего формата: её можно унести,
// положить в репозиторий или просто прочитать глазами.
func ExportMemory(targetDir string) ([]string, error) {
	src, err := UserMemoryDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil, fmt.Errorf("чтение памяти: %w", err)
	}
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return nil, fmt.Errorf("создание каталога выгрузки: %w", err)
	}

	var written []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(targetDir, e.Name()), data, 0o600); err != nil {
			return written, fmt.Errorf("запись %s: %w", e.Name(), err)
		}
		written = append(written, e.Name())
	}
	sort.Strings(written)
	return written, nil
}

// === Область записи: что синкается, а что остаётся на машине ===

// Запись памяти может быть помечена как локальная — тогда она НЕ уезжает на
// сервер ни в каком виде, даже зашифрованной. Это нужно для того, что человек
// не хочет отдавать наружу в принципе: пути, куски конфигов, заметки про
// инфраструктуру.
//
// Помечается полем во frontmatter:
//
//	---
//	name: local_notes
//	sync: false        # или local: true — понимаем оба написания
//	---
//
// Значение по умолчанию — синкать: иначе люди будут удивляться, почему память
// не появилась на второй машине. Отказ должен быть явным.
const (
	syncFalseMarker  = "sync: false"
	localTrueMarker  = "local: true"
	frontmatterFence = "---"
)

// IsLocalOnly сообщает, помечена ли запись как несинкаемая.
// Разбираем только frontmatter: слова «sync: false» в теле записи — это текст,
// а не директива, и реагировать на них нельзя.
func IsLocalOnly(content string) bool {
	fm, ok := frontmatterOf(content)
	if !ok {
		return false
	}
	for _, line := range strings.Split(fm, "\n") {
		l := strings.TrimSpace(line)
		if l == syncFalseMarker || l == localTrueMarker {
			return true
		}
	}
	return false
}

// frontmatterOf возвращает содержимое блока между первыми двумя «---».
func frontmatterOf(content string) (string, bool) {
	trimmed := strings.TrimLeft(content, "\r\n \t")
	if !strings.HasPrefix(trimmed, frontmatterFence) {
		return "", false
	}
	rest := trimmed[len(frontmatterFence):]
	end := strings.Index(rest, "\n"+frontmatterFence)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// SyncableMemoryFiles — записи, которые можно синкать: всё из каталога памяти,
// кроме помеченных локальными. Индекс MEMORY.md синкается всегда: без него
// остальные записи для агента невидимы.
func SyncableMemoryFiles() ([]string, []string, error) {
	dir, err := UserMemoryDir()
	if err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("чтение памяти: %w", err)
	}
	var syncable, local []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if IsLocalOnly(string(data)) {
			local = append(local, e.Name())
			continue
		}
		syncable = append(syncable, e.Name())
	}
	sort.Strings(syncable)
	sort.Strings(local)
	return syncable, local, nil
}
