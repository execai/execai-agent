package config

import (
	"os"
	"path/filepath"
	"testing"
)

func legacyDirs(t *testing.T) (base, oldDir, newDir string) {
	t.Helper()
	base = t.TempDir()
	oldDir = filepath.Join(base, "agent-vbai")
	newDir = filepath.Join(base, "execai")
	for _, d := range []string{oldDir, newDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readOr(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "<missing>"
	}
	return string(data)
}

// BUG-3 regression: the legacy directory was the source of «resurrection»
// of stale creds after auth.Logout(). After the copy the legacy directory
// is no longer where Dir() looks — and nothing of it is deleted: it is
// moved aside whole, with whatever else it held (Н-8 of the second loss
// review: it was removed with everything in it).
func TestMigrateLegacy_MovesLegacyAsideAfterCopy(t *testing.T) {
	base, oldDir, newDir := legacyDirs(t)
	oldCreds := `{"token":"OLD","email":"a@b","saved_at":"2026-01-01T00:00:00Z"}`
	mustWrite(t, filepath.Join(oldDir, "credentials.json"), oldCreds)
	mustWrite(t, filepath.Join(oldDir, "notes-of-my-own.md"), "kept\n")

	migrateLegacy(base, newDir)

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("the legacy folder %s is still where Dir() looks", oldDir)
	}
	if got := readOr(filepath.Join(newDir, "credentials.json")); got != oldCreds {
		t.Fatalf("creds in the new folder: %q", got)
	}
	aside := filepath.Join(base, "agent-vbai.migrated")
	if readOr(filepath.Join(aside, "notes-of-my-own.md")) != "kept\n" || readOr(filepath.Join(aside, "credentials.json")) != oldCreds {
		t.Fatalf("the legacy folder was not kept whole at %s", aside)
	}
}

// Fresh creds already in the new directory are not overwritten; the legacy
// directory is moved aside anyway (otherwise the next Logout+Dir() would
// bring the old version back), its files kept.
func TestMigrateLegacy_TargetExists(t *testing.T) {
	base, oldDir, newDir := legacyDirs(t)
	freshCreds := `{"token":"FRESH","email":"a@b"}`
	mustWrite(t, filepath.Join(newDir, "credentials.json"), freshCreds)
	mustWrite(t, filepath.Join(oldDir, "credentials.json"), "STALE")

	migrateLegacy(base, newDir)

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatal("the legacy folder is still where Dir() looks")
	}
	if got := readOr(filepath.Join(newDir, "credentials.json")); got != freshCreds {
		t.Fatalf("fresh creds overwritten by the stale ones: %q", got)
	}
	if readOr(filepath.Join(base, "agent-vbai.migrated", "credentials.json")) != "STALE" {
		t.Fatal("the legacy file was not kept")
	}
}

// No legacy directory → migrateLegacy does nothing and does not fail.
func TestMigrateLegacy_NoLegacy(t *testing.T) {
	base := t.TempDir()
	newDir := filepath.Join(base, "execai")
	os.MkdirAll(newDir, 0o700)
	migrateLegacy(base, newDir) // must not panic
}

// The real scenario: Logout deletes creds, Dir() is called: no
// resurrection from legacy (it was moved aside by the first migration).
func TestMigrateLegacy_NoResurrectionAfterLogout(t *testing.T) {
	base, oldDir, newDir := legacyDirs(t)
	mustWrite(t, filepath.Join(oldDir, "credentials.json"), "STALE-JWT")
	migrateLegacy(base, newDir)
	mustWrite(t, filepath.Join(newDir, "credentials.json"), "FRESH-JWT") // logged in
	os.Remove(filepath.Join(newDir, "credentials.json"))                 // Logout
	migrateLegacy(base, newDir)
	if _, err := os.Stat(filepath.Join(newDir, "credentials.json")); !os.IsNotExist(err) {
		t.Fatal("creds came back after Logout+Dir() (BUG-3 regression)")
	}
}

// An interrupted migration (here: a legacy file that cannot be read; the
// same as a crash or a full disk between the copies) loses nothing: the
// legacy directory stays where it was, the copy already made stays, and
// the next call finishes the migration. A second migrated folder does not
// take the place of the first.
func TestMigrateLegacy_InterruptedLosesNothing(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads everything")
	}
	base, oldDir, newDir := legacyDirs(t)
	mustWrite(t, filepath.Join(oldDir, "config.json"), `{"api_base":"https://old"}`)
	mustWrite(t, filepath.Join(oldDir, "credentials.json"), "OLD-JWT")
	if err := os.Chmod(filepath.Join(oldDir, "credentials.json"), 0); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "agent-vbai.migrated"), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(base, "agent-vbai.migrated", "earlier.txt"), "earlier\n")

	migrateLegacy(base, newDir)
	if readOr(filepath.Join(oldDir, "config.json")) == "<missing>" {
		t.Fatal("the legacy folder was touched although a copy did not go through")
	}
	if readOr(filepath.Join(newDir, "config.json")) != `{"api_base":"https://old"}` {
		t.Fatal("the copy that went through is not there")
	}
	if _, err := os.Stat(filepath.Join(newDir, "credentials.json")); !os.IsNotExist(err) {
		t.Fatal("a target was written although its source could not be read")
	}
	entries, _ := os.ReadDir(newDir)
	for _, e := range entries {
		if e.Name() != "config.json" {
			t.Errorf("left in the new folder: %s", e.Name())
		}
	}

	_ = os.Chmod(filepath.Join(oldDir, "credentials.json"), 0o600)
	migrateLegacy(base, newDir)
	if readOr(filepath.Join(newDir, "credentials.json")) != "OLD-JWT" {
		t.Fatal("the next call did not finish the migration")
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatal("the legacy folder is still where Dir() looks")
	}
	if readOr(filepath.Join(base, "agent-vbai.migrated", "earlier.txt")) != "earlier\n" ||
		readOr(filepath.Join(base, "agent-vbai.migrated-2", "credentials.json")) != "OLD-JWT" {
		t.Fatal("a folder moved aside earlier was replaced")
	}
}
