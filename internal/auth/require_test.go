package auth

import (
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
)

// Signed out, the hint names the program by its name, execai, in the
// interface language — not the old agent-vbai. Same for the 401 of the web
// chat stream.
func TestSignInHints_NameExecai(t *testing.T) {
	defer i18n.SetLocale("en")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	for _, loc := range []string{"en", "ru", "de", "es", "zh"} {
		i18n.SetLocale(loc)
		_, err := Require()
		if err == nil || !strings.Contains(err.Error(), "execai login") || strings.Contains(err.Error(), "agent-vbai") {
			t.Errorf("%s: Require: %v", loc, err)
		}
		if s := i18n.T("llm.err.aichatUnauthorized"); !strings.Contains(s, "execai login") || strings.Contains(s, "agent-vbai") {
			t.Errorf("%s: 401 of the web chat: %s", loc, s)
		}
	}
	i18n.SetLocale("en")
	if _, err := Require(); err == nil || strings.ContainsAny(err.Error(), "абвгдеёжзийклмнопрстуфхцчшщъыьэюя") {
		t.Errorf("en: %v", err)
	}
}
