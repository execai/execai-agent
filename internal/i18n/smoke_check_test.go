package i18n_test

import (
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
)

// TestHelpBody_AllLocales — help.body exists, is multi-line and contains no
// raw escape artifacts ("\\n" as text) in any locale.
func TestHelpBody_AllLocales(t *testing.T) {
	for _, l := range []string{"en", "ru", "es", "de", "zh"} {
		i18n.SetLocale(l)
		s := i18n.T("help.body")
		if s == "help.body" {
			t.Errorf("%s: help.body missing", l)
			continue
		}
		if !strings.Contains(s, "\n") {
			t.Errorf("%s: help.body is single-line — broken escape?", l)
		}
		if strings.Contains(s, `\n`) {
			t.Errorf("%s: help.body contains literal backslash-n", l)
		}
		if !strings.Contains(s, "/source") {
			t.Errorf("%s: help.body doesn't mention /source", l)
		}
	}
	i18n.SetLocale("en")
}
