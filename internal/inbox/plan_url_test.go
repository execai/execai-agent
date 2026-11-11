package inbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// A task refused by the plan answers the web chat with this installation's
// plan page.
func TestPlanRefusal_OnThisInstallation(t *testing.T) {
	i18n.SetLocale("en")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(filepath.Join(dir, "execai"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "execai", "config.json"), []byte(`{"api_base":"https://apidev.example.com"}`), 0o600)
	if err := config.SaveCredentials(&config.Credentials{Token: "t", WebBase: "https://web.example.org"}); err != nil {
		t.Fatal(err)
	}
	if text := PlanRefusal(); !strings.Contains(text, "https://web.example.org/settings/payment?") || strings.Contains(text, "chat.execai.ru") {
		t.Errorf("refusal: %s", text)
	}
}
