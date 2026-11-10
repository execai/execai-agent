package messages

import (
	"strings"
	"testing"
)

// TestMachineMessages_AllLocales — every string of /start, /machine and
// /phone exists in all five languages with the same placeholders, is
// translated, and says nothing the public texts must not say.
func TestMachineMessages_AllLocales(t *testing.T) {
	all := map[string]map[string]string{
		"en": enMachineMessages, "ru": ruMachineMessages, "de": deMachineMessages,
		"es": esMachineMessages, "zh": zhMachineMessages,
	}
	if len(enMachineMessages) < 40 {
		t.Fatalf("only %d machine strings in en", len(enMachineMessages))
	}
	for k, en := range enMachineMessages {
		for loc, cat := range all {
			if loc == "en" {
				continue
			}
			v, ok := cat[k]
			if !ok || strings.TrimSpace(v) == "" {
				t.Errorf("%s: %s missing", loc, k)
				continue
			}
			if verbs(v) != verbs(en) {
				t.Errorf("%s: %s placeholders %q, en %q", loc, k, verbs(v), verbs(en))
			}
			if v == en && !untranslatable[k] {
				t.Errorf("%s: %s is the English text", loc, k)
			}
		}
	}
	for loc, cat := range all {
		for k, v := range cat {
			if _, ok := enMachineMessages[k]; !ok {
				t.Errorf("%s: %s is not in en", loc, k)
			}
			low := strings.ToLower(v)
			for _, banned := range []string{
				"бесплат", "free", "kostenlos", "gratis", "免费", "vpn",
				"claude", "cursor", "codex", "openai", "anthropic", "telegram", "whatsapp", "google", "apple",
			} {
				if strings.Contains(low, banned) {
					t.Errorf("%s: %s says %q", loc, k, banned)
				}
			}
		}
	}
	// The sync step says what does not travel and that the password cannot
	// be recovered.
	for loc, cat := range all {
		if !strings.Contains(cat["start.sync.notWhat"], "api_base") {
			t.Errorf("%s: start.sync.notWhat does not name api_base", loc)
		}
	}
}

// untranslatable — keys whose text happens to be the same word in some
// language ("plan" in Spanish, "Sync" in German).
var untranslatable = map[string]bool{"machine.account.in": true, "machine.label.sync": true}
