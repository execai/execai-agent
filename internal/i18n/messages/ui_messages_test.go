package messages

import (
	"strings"
	"testing"
)

// TestUIMessages_AllLocales — every execai ui string exists in all five
// languages with the same placeholders and is translated.
func TestUIMessages_AllLocales(t *testing.T) {
	cats := map[string]map[string]string{
		"ru": ruUIMessages, "de": deUIMessages, "es": esUIMessages, "zh": zhUIMessages,
	}
	for k, en := range enUIMessages {
		for loc, cat := range cats {
			v, ok := cat[k]
			if !ok || strings.TrimSpace(v) == "" {
				t.Errorf("%s: %s missing", loc, k)
				continue
			}
			if verbs(v) != verbs(en) {
				t.Errorf("%s: %s placeholders %q, en %q", loc, k, verbs(v), verbs(en))
			}
			if v == en {
				t.Errorf("%s: %s is the English text", loc, k)
			}
		}
	}
	for loc, cat := range cats {
		for k := range cat {
			if _, ok := enUIMessages[k]; !ok {
				t.Errorf("%s: %s is not in en", loc, k)
			}
		}
	}
}
