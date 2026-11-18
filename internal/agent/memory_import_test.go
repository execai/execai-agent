package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverMemorySources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "# Проект\nПрод на R5, ключи не синкаем.")
	writeFile(t, filepath.Join(dir, ".cursorrules"), "Отвечай кратко. Go, не Python.")
	writeFile(t, filepath.Join(dir, ".cursor", "rules", "style.mdc"), "Комментарии по-английски.")
	writeFile(t, filepath.Join(dir, ".github", "copilot-instructions.md"), "Используй табы.")
	// Шум, который подхватывать нельзя.
	writeFile(t, filepath.Join(dir, "README.md"), "обычный readme")
	writeFile(t, filepath.Join(dir, "empty.md"), "")
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), `{"a":1}`)

	got := DiscoverMemorySources(dir)
	found := map[string]bool{}
	for _, s := range got {
		found[s.Rel] = true
	}

	for _, want := range []string{"CLAUDE.md", ".cursorrules", ".github/copilot-instructions.md"} {
		if !found[want] {
			t.Errorf("не найден %s", want)
		}
	}
	// README — не память агента, его импорт затащил бы в контекст мусор.
	if found["README.md"] {
		t.Error("README.md попал в источники памяти")
	}
	// JSON из .claude/ — служебный файл, не память.
	if found[".claude/settings.json"] {
		t.Error("settings.json попал в источники памяти")
	}
	for _, s := range got {
		if s.Preview == "" {
			t.Errorf("%s без предпросмотра — пользователь не поймёт, что импортирует", s.Rel)
		}
		if s.Tool == "" {
			t.Errorf("%s без имени инструмента", s.Rel)
		}
	}
}

// Пустые файлы и гигантские дампы — не память. Первые бесполезны, вторые
// обычно логи, случайно попавшие в каталог.
func TestDiscoverMemorySources_SkipsEmptyAndHuge(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "")
	writeFile(t, filepath.Join(dir, "AGENTS.md"), strings.Repeat("x", maxImportFileSize+1))

	if got := DiscoverMemorySources(dir); len(got) != 0 {
		t.Errorf("ожидалось 0 источников, получено %d: %+v", len(got), got)
	}
}

func TestImportSources(t *testing.T) {
	// Подменяем каталог памяти на временный.
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)

	src := t.TempDir()
	writeFile(t, filepath.Join(src, "CLAUDE.md"), "Прод на R5. Ключи подписок не синкаем.")

	sources := DiscoverMemorySources(src)
	if len(sources) != 1 {
		t.Fatalf("ожидался 1 источник, получено %d", len(sources))
	}

	res, err := ImportSources(sources)
	if err != nil {
		t.Fatalf("импорт: %v", err)
	}
	if len(res.Imported) != 1 {
		t.Fatalf("импортировано %d файлов, ожидался 1 (пропущено: %v)", len(res.Imported), res.Skipped)
	}

	memDir, _ := UserMemoryDir()
	data, err := os.ReadFile(filepath.Join(memDir, res.Imported[0]))
	if err != nil {
		t.Fatalf("чтение импортированного: %v", err)
	}
	text := string(data)

	if !strings.Contains(text, "Прод на R5") {
		t.Error("содержимое исходника потерялось")
	}
	// Через полгода будет непонятно, откуда взялся текст, если не записать.
	if !strings.Contains(text, "imported_from") || !strings.Contains(text, "Claude Code") {
		t.Error("не сохранено происхождение записи")
	}

	// Индекс — единственное, что попадает в системный промт. Запись без строки
	// в нём для агента невидима.
	index, err := os.ReadFile(filepath.Join(memDir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("индекс не создан: %v", err)
	}
	if !strings.Contains(string(index), res.Imported[0]) {
		t.Error("импортированный файл не попал в индекс MEMORY.md")
	}
}

// Повторный импорт не должен затирать уже существующую запись: пользователь
// мог её отредактировать после первого раза.
func TestImportSources_DoesNotOverwrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)

	src := t.TempDir()
	writeFile(t, filepath.Join(src, "CLAUDE.md"), "первая версия")
	sources := DiscoverMemorySources(src)

	if _, err := ImportSources(sources); err != nil {
		t.Fatalf("первый импорт: %v", err)
	}
	memDir, _ := UserMemoryDir()
	name := importedFileName(sources[0])
	// Пользователь поправил запись.
	edited := "---\nname: x\n---\n\nотредактировано человеком"
	if err := os.WriteFile(filepath.Join(memDir, name), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := ImportSources(sources)
	if err != nil {
		t.Fatalf("повторный импорт: %v", err)
	}
	if len(res.Imported) != 0 {
		t.Error("повторный импорт перезаписал существующую запись")
	}
	if len(res.Skipped) != 1 {
		t.Errorf("ожидался 1 пропуск с объяснением, получено %v", res.Skipped)
	}

	data, _ := os.ReadFile(filepath.Join(memDir, name))
	if !strings.Contains(string(data), "отредактировано человеком") {
		t.Error("правка пользователя затёрта импортом")
	}
}

// Память не должна быть заложником нашего формата: её можно унести.
func TestExportMemory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)

	memDir, err := UserMemoryDir()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(memDir, "MEMORY.md"), "- [факт](fact.md) — про прод.")
	writeFile(t, filepath.Join(memDir, "fact.md"), "прод на R5")
	writeFile(t, filepath.Join(memDir, "notes.txt"), "не markdown, не выгружаем")

	out := filepath.Join(t.TempDir(), "export")
	written, err := ExportMemory(out)
	if err != nil {
		t.Fatalf("экспорт: %v", err)
	}
	if len(written) != 2 {
		t.Errorf("выгружено %d файлов, ожидалось 2: %v", len(written), written)
	}
	data, err := os.ReadFile(filepath.Join(out, "fact.md"))
	if err != nil || string(data) != "прод на R5" {
		t.Errorf("содержимое не совпало: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(out, "notes.txt")); err == nil {
		t.Error("не-markdown файл попал в выгрузку")
	}
}

// Пометка «не синкать» — единственный способ сказать, что запись остаётся на
// машине. Значение по умолчанию — синкать, иначе люди будут удивляться,
// почему память не появилась на второй машине.
func TestIsLocalOnly(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"обычная запись", "---\nname: x\n---\n\nтекст", false},
		{"sync: false", "---\nname: x\nsync: false\n---\n\nтекст", true},
		{"local: true", "---\nname: x\nlocal: true\n---\n\nтекст", true},
		{"без frontmatter", "просто текст", false},
		{"незакрытый frontmatter", "---\nname: x\nsync: false", false},
		// Слова в теле записи — это текст, а не директива. Иначе заметка
		// «поставил sync: false» сама себя пометила бы локальной.
		{"упоминание в теле", "---\nname: x\n---\n\nя написал sync: false в конфиге", false},
		{"с пробелами", "---\nname: x\n  sync: false  \n---\n\nтекст", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsLocalOnly(c.content); got != c.want {
				t.Errorf("IsLocalOnly() = %v, ожидалось %v", got, c.want)
			}
		})
	}
}

func TestSyncableMemoryFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)

	dir, _ := UserMemoryDir()
	writeFile(t, filepath.Join(dir, "MEMORY.md"), "- индекс")
	writeFile(t, filepath.Join(dir, "public.md"), "---\nname: public\n---\n\nобычная запись")
	writeFile(t, filepath.Join(dir, "secret.md"), "---\nname: secret\nsync: false\n---\n\nпути и конфиги")
	writeFile(t, filepath.Join(dir, "notes.txt"), "не markdown")

	syncable, local, err := SyncableMemoryFiles()
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(local) != 1 || local[0] != "secret.md" {
		t.Errorf("локальные записи: %v, ожидалось [secret.md]", local)
	}
	// Индекс синкается всегда: без него остальные записи для агента невидимы.
	want := map[string]bool{"MEMORY.md": true, "public.md": true}
	if len(syncable) != len(want) {
		t.Errorf("синкаемые: %v, ожидалось %v", syncable, want)
	}
	for _, f := range syncable {
		if !want[f] {
			t.Errorf("в синк попал лишний файл: %s", f)
		}
		if f == "secret.md" {
			t.Error("помеченная локальной запись попала в синк")
		}
	}
}
