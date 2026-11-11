package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// thisInstallation — the agent configured for another installation: its
// API and the web the sign-in was confirmed on.
func thisInstallation(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "execai"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "execai", "config.json"), []byte(`{"api_base":"https://apidev.example.com"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveCredentials(&config.Credentials{Token: "t", WebBase: "https://web.example.org"}); err != nil {
		t.Fatal(err)
	}
}

// The refusal of the model gate and /plan send to the plan page of this
// installation's web, not to the production site.
func TestPlanLinks_OnThisInstallation(t *testing.T) {
	i18n.SetLocale("en")
	thisInstallation(t)
	want := "https://web.example.org/settings/payment?utm_source=agent&utm_medium=cli&utm_campaign=gate"
	for name, text := range map[string]string{
		"gate":       Verdict{Reason: ReasonNoPlan}.Message(),
		"unverified": Verdict{Reason: ReasonUnverified}.Message(),
	} {
		if !strings.Contains(text, want) || strings.Contains(text, "chat.execai.ru") {
			t.Errorf("%s: %s", name, text)
		}
	}
}
