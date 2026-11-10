package chat

import (
	"io"
	"net/http"
	"os"
	"strconv"
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
		// Compare by number, not by inequality: otherwise ANY difference reads as
		// "update available" — including a rollback of the channel, and any local
		// build with a suffix. A dev build of 5.155 was being told to "update" to
		// R5.152.
		if !remoteIsNewer(localVer, remoteVer) {
			return updateLatestMsg{}
		}
		return updateAvailableMsg{
			hint: "🔔 Доступна новая версия execai: " + localVer + " → " + remoteVer + "\n" +
				"   Обновить:  curl -fsSL " + updateInstallURL() + " | bash\n" +
				"   (Windows:  iwr -useb " + strings.Replace(updateInstallURL(), "install.sh", "install.ps1", 1) + " | iex)",
		}
	}
}

// parseVersion pulls the numeric parts out of the version strings we actually
// produce: "R5.152", "5.152", "5.155-dev". Returns false when there is nothing
// numeric to compare — in that case we stay quiet rather than guess.
func parseVersion(v string) ([]int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "R")
	v = strings.TrimPrefix(v, "v")
	// Drop any suffix: "-dev", "-rc1", "+build".
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, len(out) > 0
}

// remoteIsNewer reports whether remote is strictly ahead of local.
func remoteIsNewer(local, remote string) bool {
	l, okL := parseVersion(local)
	r, okR := parseVersion(remote)
	if !okL || !okR {
		return false // unparseable — do not nag
	}
	for i := 0; i < len(l) || i < len(r); i++ {
		var a, b int
		if i < len(l) {
			a = l[i]
		}
		if i < len(r) {
			b = r[i]
		}
		if a != b {
			return b > a
		}
	}
	return false
}
