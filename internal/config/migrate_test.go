package config

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-3 regression: legacy-папка была источником "воскрешения" протухших
// creds после auth.Logout(). Тест гарантирует что migrateLegacy удаляет
// legacy-папку после копирования, чтобы Dir() не мигрировал повторно.
func TestMigrateLegacy_RemovesLegacyAfterCopy(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "agent-vbai")
	newDir := filepath.Join(base, "execai")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// В legacy — старый creds. Symuluj situacie.
	oldCreds := `{"token":"OLD","email":"a@b","saved_at":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(oldDir, "credentials.json"), []byte(oldCreds), 0o600); err != nil {
		t.Fatal(err)
	}

	migrateLegacy(base, newDir)

	// 1. Legacy папки больше нет (иначе воскрешение).
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("legacy папка %s должна быть удалена, но существует", oldDir)
	}
	// 2. Файл в новой папке скопирован.
	data, err := os.ReadFile(filepath.Join(newDir, "credentials.json"))
	if err != nil {
		t.Fatalf("новый creds не создан: %v", err)
	}
	if string(data) != oldCreds {
		t.Fatalf("контент не тот: %q", data)
	}
}

// Даже если в новой папке уже есть свежие creds, legacy всё равно удаляем.
// Иначе следующий Logout+Dir() восстановит старую версию.
func TestMigrateLegacy_RemovesLegacyEvenWhenTargetExists(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "agent-vbai")
	newDir := filepath.Join(base, "execai")
	os.MkdirAll(oldDir, 0o700)
	os.MkdirAll(newDir, 0o700)

	freshCreds := `{"token":"FRESH","email":"a@b"}`
	os.WriteFile(filepath.Join(newDir, "credentials.json"), []byte(freshCreds), 0o600)
	os.WriteFile(filepath.Join(oldDir, "credentials.json"), []byte("STALE"), 0o600)

	migrateLegacy(base, newDir)

	// Legacy снесена.
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatal("legacy не удалена")
	}
	// Свежий creds НЕ перезаписан старым.
	data, _ := os.ReadFile(filepath.Join(newDir, "credentials.json"))
	if string(data) != freshCreds {
		t.Fatalf("свежий creds перезаписан старым: %q", data)
	}
}

// Нет legacy папки → migrateLegacy ничего не делает и не падает.
func TestMigrateLegacy_NoLegacy(t *testing.T) {
	base := t.TempDir()
	newDir := filepath.Join(base, "execai")
	os.MkdirAll(newDir, 0o700)
	migrateLegacy(base, newDir) // не должно паниковать
}

// Симуляция настоящего сценария: Logout удаляет creds, Dir() вызывается,
// НЕ должно быть воскрешения из legacy (потому что legacy удалена
// на предыдущей миграции).
func TestMigrateLegacy_NoResurrectionAfterLogout(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "agent-vbai")
	newDir := filepath.Join(base, "execai")
	os.MkdirAll(oldDir, 0o700)
	os.MkdirAll(newDir, 0o700)

	os.WriteFile(filepath.Join(oldDir, "credentials.json"), []byte("STALE-JWT"), 0o600)

	// Первый вызов — миграция.
	migrateLegacy(base, newDir)

	// Юзер сделал login → новые creds.
	os.WriteFile(filepath.Join(newDir, "credentials.json"), []byte("FRESH-JWT"), 0o600)

	// Симулируем Logout → удаление credentials.json.
	os.Remove(filepath.Join(newDir, "credentials.json"))

	// Второй вызов Dir()/migrateLegacy — legacy должна быть уже удалена
	// первой миграцией, поэтому воскрешения не будет.
	migrateLegacy(base, newDir)

	if _, err := os.Stat(filepath.Join(newDir, "credentials.json")); !os.IsNotExist(err) {
		t.Fatal("после Logout+Dir() creds воскресли (BUG-3 regression)")
	}
}
