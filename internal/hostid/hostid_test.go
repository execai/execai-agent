package hostid

import (
	"os"
	"path/filepath"
	"testing"
)

// Прогон fallback-пути (когда machineid не сработал). Первый вызов создаёт файл,
// второй читает тот же ID.
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
	// файл создан
	path := filepath.Join(dir, "execai", "host_id")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("файл не создан: %v", err)
	}
	// второй вызов должен вернуть ТО ЖЕ значение
	id2, err := getOrCreateFallback()
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("fallback не стабилен: %q vs %q", id1, id2)
	}
}

// Get() всегда должен что-то вернуть — либо machineid либо fallback.
func TestGet_NeverEmpty(t *testing.T) {
	id, err := Get()
	if err != nil {
		t.Fatalf("Get err: %v", err)
	}
	if id == "" {
		t.Fatal("Get returned empty")
	}
}
