package hostid

import (
	"os"
	"path/filepath"
	"testing"
)

// Exercise the fallback path (when machineid failed). The first call creates
// the file, the second reads the same ID.
func TestFallbackFile_StableAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	id1, err := getOrCreateFallback()
	if err != nil {
		t.Fatal(err)
	}
	if id1 == "" {
		t.Fatal("empty id")
	}
	if len(id1) < 32 {
		t.Fatalf("id слишком короткий: %q", id1)
	}
	// file created
	path := filepath.Join(dir, "execai", "host_id")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("файл не создан: %v", err)
	}
	// the second call must return the SAME value
	id2, err := getOrCreateFallback()
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("fallback не стабилен: %q vs %q", id1, id2)
	}
}

// Get() must always return something — either machineid or the fallback.
func TestGet_NeverEmpty(t *testing.T) {
	id, err := Get()
	if err != nil {
		t.Fatalf("Get err: %v", err)
	}
	if id == "" {
		t.Fatal("Get returned empty")
	}
}
