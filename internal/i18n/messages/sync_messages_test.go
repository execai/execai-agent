package messages

import (
	"sort"
	"strings"
	"testing"
)

// verbs — the formatting verbs of a template, sorted, so that explicit
// argument indexes (%[2]s) may reorder words between languages.
func verbs(s string) string {
	var out []string
	for i := 0; i < len(s)-1; i++ {
		if s[i] != '%' {
			continue
		}
		j := i + 1
		if s[j] == '%' {
			i = j
			continue
		}
		idx := ""
		if s[j] == '[' {
			k := strings.IndexByte(s[j:], ']')
			idx = s[j : j+k+1]
			j += k + 1
		}
		out = append(out, idx+string(s[j]))
		i = j
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// TestSyncMessages_AllLocales — every sync string exists in all five
// languages with the same placeholders, and none of them is the English
// text left untranslated. The setup texts stay plain: no crypto jargon.
func TestSyncMessages_AllLocales(t *testing.T) {
	cats := map[string]map[string]string{
		"ru": ruSyncMessages, "de": deSyncMessages, "es": esSyncMessages, "zh": zhSyncMessages,
	}
	for k, en := range enSyncMessages {
		for loc, cat := range cats {
			v, ok := cat[k]
			if !ok || strings.TrimSpace(v) == "" {
				t.Errorf("%s: %s missing", loc, k)
				continue
			}
			if verbs(v) != verbs(en) {
				t.Errorf("%s: %s placeholders %q, en %q", loc, k, verbs(v), verbs(en))
			}
			if v == en && !strings.HasPrefix(k, "sync.usage") {
				t.Errorf("%s: %s is the English text", loc, k)
			}
		}
	}
	for loc, cat := range cats {
		for k := range cat {
			if _, ok := enSyncMessages[k]; !ok {
				t.Errorf("%s: %s is not in en", loc, k)
			}
		}
	}
	for loc, cat := range map[string]map[string]string{"en": enSyncMessages, "ru": ruSyncMessages,
		"de": deSyncMessages, "es": esSyncMessages, "zh": zhSyncMessages} {
		for k, v := range cat {
			if !strings.HasPrefix(k, "sync.on.") && !strings.HasPrefix(k, "sync.password.") && !strings.HasPrefix(k, "sync.keys.") {
				continue
			}
			for _, jargon := range []string{"E2E", "X25519", "age ", "scrypt"} {
				if strings.Contains(v, jargon) {
					t.Errorf("%s: %s says %q", loc, k, jargon)
				}
			}
		}
	}
	// Decision of the owner (01.11.2026): provider keys travel with the
	// config, encrypted. No text may still say they stay behind.
	for loc, cat := range map[string]map[string]string{"en": enSyncMessages, "ru": ruSyncMessages} {
		for k, v := range cat {
			for _, old := range []string{"остаются на этом компьютере", "не передаются", "stay on this computer", "do not go through"} {
				if strings.Contains(v, old) {
					t.Errorf("%s: %s still says %q", loc, k, old)
				}
			}
		}
	}
	if !strings.Contains(ruSyncMessages["sync.what.config"], "ключи провайдеров") ||
		!strings.Contains(ruSyncMessages["sync.keys.exported"], "без сервера и без тарифа") {
		t.Error("the config line must name provider keys; keys export — the fallback without server and plan")
	}
	if !strings.Contains(ruSyncMessages["sync.gate.noPlan"], "«Работа»") || !strings.Contains(ruSyncMessages["sync.gate.noPlan"], "«Макс»") {
		t.Error("the refusal must name the Work and Max plans")
	}
}
