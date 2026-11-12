package messages

import (
	"strings"
	"testing"
)

// TestIDEMessages_AllLocales — the panel's permission descriptions and
// source list exist in all five languages with the same placeholders and
// are translated.
func TestIDEMessages_AllLocales(t *testing.T) {
	all := map[string]map[string]string{
		"en": enIDEMessages, "ru": ruIDEMessages, "de": deIDEMessages,
		"es": esIDEMessages, "zh": zhIDEMessages,
	}
	for k, en := range enIDEMessages {
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
		for k := range cat {
			if _, ok := enIDEMessages[k]; !ok {
				t.Errorf("%s: %s is not in en", loc, k)
			}
		}
		if !strings.Contains(cat["ide.approve.foreverDesc"], "permissions.json") {
			t.Errorf("%s: the always option does not name permissions.json", loc)
		}
	}
}
