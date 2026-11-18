package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// EXECAI_LOCALE is set for this process: it wins over the locale of
// config.json, which sync shares between computers (third loss probe).
func TestSetCLILocale_EnvWinsOverTheConfig(t *testing.T) {
	defer i18n.SetLocale("en")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	base, _ := os.UserConfigDir()
	_ = os.MkdirAll(filepath.Join(base, "execai"), 0o700)
	if err := os.WriteFile(filepath.Join(base, "execai", "config.json"), []byte(`{"locale":"ru"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXECAI_LOCALE", "en_US.UTF-8")
	setCLILocale()
	if got := i18n.Locale(); got != "en" {
		t.Errorf("locale %q, want en from EXECAI_LOCALE", got)
	}
	t.Setenv("EXECAI_LOCALE", "")
	setCLILocale()
	if got := i18n.Locale(); got != "ru" {
		t.Errorf("locale %q, want ru from config.json", got)
	}
}
