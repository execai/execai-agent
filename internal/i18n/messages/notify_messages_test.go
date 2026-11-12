package messages

import (
	"strings"
	"testing"
)

// TestNotifyMessages_AllLocales — every notification string exists in all
// five languages with the same placeholders and is translated; the
// notifications themselves name the product and carry one placeholder at
// most (a folder or a tool — never a prompt).
func TestNotifyMessages_AllLocales(t *testing.T) {
	all := map[string]map[string]string{
		"en": enNotifyMessages, "ru": ruNotifyMessages, "de": deNotifyMessages,
		"es": esNotifyMessages, "zh": zhNotifyMessages,
	}
	for k, en := range enNotifyMessages {
		for loc, cat := range all {
			v, ok := cat[k]
			if !ok || strings.TrimSpace(v) == "" {
				t.Errorf("%s: %s missing", loc, k)
				continue
			}
			if verbs(v) != verbs(en) {
				t.Errorf("%s: %s placeholders %q, en %q", loc, k, verbs(v), verbs(en))
			}
			if loc != "en" && v == en {
				t.Errorf("%s: %s is the English text", loc, k)
			}
		}
	}
	for loc, cat := range all {
		for k, v := range cat {
			if _, ok := enNotifyMessages[k]; !ok {
				t.Errorf("%s: %s is not in en", loc, k)
			}
			low := strings.ToLower(v)
			for _, banned := range []string{"бесплат", "free", "kostenlos", "gratis", "免费", "vpn",
				"claude", "cursor", "codex", "openai", "anthropic", "telegram", "google", "apple"} {
				if strings.Contains(low, banned) {
					t.Errorf("%s: %s says %q", loc, k, banned)
				}
			}
		}
		for _, k := range []string{"notify.turnDone", "notify.approve", "notify.approveAny", "notify.ask", "notify.test",
			"notify.bgDone", "notify.bgStopped", "notify.bgError", "notify.bgLimit"} {
			v := cat[k]
			if !strings.HasPrefix(v, "ExecAI") || strings.Count(v, "%") > 1 || strings.Contains(v, "\n") {
				t.Errorf("%s: %s = %q", loc, k, v)
			}
		}
	}
}
