package version

import (
	"strings"
	"testing"
)

// The User-Agent is a compliance surface, not cosmetics: Kimi Code and the Z.ai
// GLM Coding Plan both forbid spoofing the client identity, so this string must
// name us honestly and stay stable — providers whitelist clients by it.
func TestUserAgent(t *testing.T) {
	Set("5.145")
	ua := UserAgent()

	if want := "execai-agent/5.145"; !strings.HasPrefix(ua, want) {
		t.Errorf("User-Agent должен начинаться с %q, получено %q", want, ua)
	}
	if !strings.Contains(ua, Repo) {
		t.Errorf("в User-Agent нет ссылки на репозиторий: %q", ua)
	}
	// Never claim to be someone else's client.
	for _, forbidden := range []string{"claude", "Claude", "curl", "python", "node", "Go-http-client"} {
		if strings.Contains(ua, forbidden) {
			t.Errorf("User-Agent маскируется под %q: %q", forbidden, ua)
		}
	}
}

func TestSetIgnoresEmpty(t *testing.T) {
	Set("5.145")
	Set("") // a build without ldflags must not wipe the version
	if got := Get(); got != "5.145" {
		t.Errorf("Set(\"\") затёр версию: %q", got)
	}
}
