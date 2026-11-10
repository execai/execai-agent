package config

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-3 regression: the legacy directory was the source of "resurrection" of
// stale creds after auth.Logout(). This test guarantees that migrateLegacy
// removes the legacy directory after copying, so Dir() does not migrate again.
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
	// Legacy holds old creds. Simulate the situation.
	oldCreds := `{"token":"OLD","email":"a@b","saved_at":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(oldDir, "credentials.json"), []byte(oldCreds), 0o600); err != nil {
		t.Fatal(err)
	}

	migrateLegacy(base, newDir)

	// 1. The legacy directory is gone (otherwise resurrection).
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("legacy папка %s должна быть удалена, но существует", oldDir)
	}
	// 2. The file was copied into the new directory.
	data, err := os.ReadFile(filepath.Join(newDir, "credentials.json"))
	if err != nil {
		t.Fatalf("новый creds не создан: %v", err)
	}
	if string(data) != oldCreds {
		t.Fatalf("контент не тот: %q", data)
	}
}

// Even if fresh creds already exist in the new directory, legacy is removed
// anyway. Otherwise the next Logout+Dir() would restore the old version.
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

	// Legacy removed.
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatal("legacy не удалена")
	}
	// Fresh creds NOT overwritten by the stale ones.
	data, _ := os.ReadFile(filepath.Join(newDir, "credentials.json"))
	if string(data) != freshCreds {
		t.Fatalf("свежий creds перезаписан старым: %q", data)
	}
}

// No legacy directory → migrateLegacy does nothing and does not fail.
func TestMigrateLegacy_NoLegacy(t *testing.T) {
	base := t.TempDir()
	newDir := filepath.Join(base, "execai")
	os.MkdirAll(newDir, 0o700)
	migrateLegacy(base, newDir) // must not panic
}

// Simulation of the real scenario: Logout deletes creds, Dir() is called,
// there must be NO resurrection from legacy (because legacy was removed
// during the previous migration).
func TestMigrateLegacy_NoResurrectionAfterLogout(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, "agent-vbai")
	newDir := filepath.Join(base, "execai")
	os.MkdirAll(oldDir, 0o700)
	os.MkdirAll(newDir, 0o700)

	os.WriteFile(filepath.Join(oldDir, "credentials.json"), []byte("STALE-JWT"), 0o600)

	// First call — migration.
	migrateLegacy(base, newDir)

	// The user logged in → new creds.
	os.WriteFile(filepath.Join(newDir, "credentials.json"), []byte("FRESH-JWT"), 0o600)

	// Simulate Logout → removal of credentials.json.
	os.Remove(filepath.Join(newDir, "credentials.json"))

	// Second Dir()/migrateLegacy call — legacy must already be gone after the
	// first migration, so there will be no resurrection.
	migrateLegacy(base, newDir)

	if _, err := os.Stat(filepath.Join(newDir, "credentials.json")); !os.IsNotExist(err) {
		t.Fatal("после Logout+Dir() creds воскресли (BUG-3 regression)")
	}
}
