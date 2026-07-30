// Welcome screen — onboarding on first launch. The `.welcome_seen` marker
// lives in the config directory. It is created after the first display, and
// follow-up launches skip the greeting.
package welcome

import (
	"os"
	"path/filepath"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// MaybeWelcome returns the greeting text on the FIRST launch, otherwise an
// empty string. The marker is written immediately upon display.
func MaybeWelcome() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return i18n.T("welcome.text")
	}
	dir := filepath.Join(base, "execai")
	_ = os.MkdirAll(dir, 0o700)
	marker := filepath.Join(dir, ".welcome_seen")
	if _, err := os.Stat(marker); err == nil {
		return ""
	}
	_ = os.WriteFile(marker, []byte("1"), 0o644)
	return i18n.T("welcome.text")
}
