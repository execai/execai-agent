package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// The sync refusal without a plan sends to this installation's web.
func TestSyncPlanLink_OnThisInstallation(t *testing.T) {
	i18n.SetLocale("en")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(filepath.Join(dir, "execai"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "execai", "config.json"), []byte(`{"api_base":"https://apidev.example.com"}`), 0o600)
	if err := config.SaveCredentials(&config.Credentials{Token: "t", WebBase: "https://web.example.org"}); err != nil {
		t.Fatal(err)
	}
	text := NoPlan.Message()
	if !strings.Contains(text, "https://web.example.org/settings/payment?utm_source=agent&utm_medium=cli&utm_campaign=sync") ||
		strings.Contains(text, "chat.execai.ru") {
		t.Errorf("refusal: %s", text)
	}
}
