package chat

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// updateAvailableMsg — event "a fresh version is in prod".
type updateAvailableMsg struct {
	hint string
}

// updateLatestMsg — we are on the latest version (shown briefly in status_message).
type updateLatestMsg struct{}

// updateChannel — which channel to check for updates. Default is "stable":
// a branch-independent alias in the bucket (execai/stable/) that always
// points at the current release, whatever dev branch (R5, R6, …) produced it.
// Old binaries keep working: R5/latest is mirrored alongside stable.
// Override for testing: EXECAI_UPDATE_CHANNEL=R5 (path segment under execai/).
func updateChannel() string {
	if v := strings.TrimSpace(os.Getenv("EXECAI_UPDATE_CHANNEL")); v != "" {
		return v + "/latest"
	}
	return "stable"
}

func updateVersionURL() string {
	return "https://storage.yandexcloud.net/execai-agent-prod/execai/" + updateChannel() + "/VERSION.txt"
}

func updateInstallURL() string {
	return "https://storage.yandexcloud.net/execai-agent-prod/execai/" + updateChannel() + "/install.sh"
}

// checkForUpdateCmd — pulls VERSION.txt from the prod bucket and compares it with the
// actual version of the running binary (agentVersion, baked in via -ldflags -X main.version).
//
// Previously we compared the archive SHA256SUMS with the local installed_arch_sha stamp
// written by install.sh. Problem: if the user has two binaries in PATH (one
// fresh from install.sh, another old one in a different directory), the stamp would be new
// while the old one was running — and the UI lied "latest version" with an outdated binary.
//
// Now we compare the ACTUAL binary version. No mistake is possible: agentVersion
// in the binary and VERSION.txt in the bucket of the same release match only if the binary
// really is of the same build.
func (m *tuiModel) checkForUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 8 * time.Second}
		resp, err := client.Get(updateVersionURL())
		if err != nil || resp == nil {
			return nil
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil || len(body) == 0 {
			return nil
		}
		remoteVer := strings.TrimSpace(string(body))
		if remoteVer == "" {
			return nil
		}
		localVer := strings.TrimSpace(agentVersion)
		// dev build (no ldflags) — do not spam with updates.
		if localVer == "" || localVer == "dev" {
			return nil
		}
		if localVer == remoteVer {
			return updateLatestMsg{}
		}
		return updateAvailableMsg{
			hint: "🔔 Доступна новая версия execai: " + localVer + " → " + remoteVer + "\n" +
				"   Обновить:  curl -fsSL " + updateInstallURL() + " | bash\n" +
				"   (Windows:  iwr -useb " + strings.Replace(updateInstallURL(), "install.sh", "install.ps1", 1) + " | iex)",
		}
	}
}
