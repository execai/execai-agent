package chat

import (
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// EXECAI_LOCALE is set for this process: it wins over the locale of
// config.json, which sync shares between computers (third loss probe).
func TestInitI18nLocale_EnvWinsOverTheConfig(t *testing.T) {
	defer i18n.SetLocale("en")
	t.Setenv("EXECAI_LOCALE", "en_US.UTF-8")
	initI18nLocale(&config.Config{Locale: "ru"})
	if got := i18n.Locale(); got != "en" {
		t.Errorf("locale %q, want en from EXECAI_LOCALE", got)
	}
	t.Setenv("EXECAI_LOCALE", "")
	initI18nLocale(&config.Config{Locale: "ru"})
	if got := i18n.Locale(); got != "ru" {
		t.Errorf("locale %q, want ru from config.json", got)
	}
}
