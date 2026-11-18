package messages

import (
	"strings"
	"testing"
)

// TestAgentMessages_AllLocales — the agent and model-catalog strings exist
// in all five languages with the same placeholders, translated.
func TestAgentMessages_AllLocales(t *testing.T) {
	cats := map[string]map[string]string{
		"ru": ruAgentMessages, "de": deAgentMessages, "es": esAgentMessages, "zh": zhAgentMessages,
	}
	for k, en := range enAgentMessages {
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
			if _, ok := enAgentMessages[k]; !ok {
				t.Errorf("%s: %s is not in en", loc, k)
			}
		}
	}
}
