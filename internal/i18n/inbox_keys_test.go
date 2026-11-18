package i18n_test

import (
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
)

// inboxKeys — every string of the "web → agent" channel (execai serve and
// the editor panel taking tasks from the site).
var inboxKeys = []string{
	"inbox.err.parse",
	"inbox.lock.busy", "inbox.lock.heldByIDE", "inbox.lock.heldByServe",
	"inbox.lock.signalFailed", "inbox.lock.stopTimeout",
	"inbox.plan.refused", "inbox.plan.serveWarn",
	"inbox.result.interrupted", "inbox.result.noText",
	"inbox.task.bindingsFailed", "inbox.task.dirMissing", "inbox.task.noDir",
	"inbox.task.notBound", "inbox.task.empty", "inbox.task.stopped",
	"inbox.task.timeout", "inbox.task.editorClosed",
	"inbox.serve.refused", "inbox.serve.statusIDE", "inbox.serve.stopIDE",
	"inbox.ide.otherProject", "inbox.ide.anotherWindow", "inbox.ide.taken",
	"inbox.ide.bindWhich",
}

// TestInboxKeys_AllLocales — each inbox string exists in all five languages,
// is its own translation (not the English fallback) and keeps the same
// placeholders as English.
func TestInboxKeys_AllLocales(t *testing.T) {
	defer i18n.SetLocale("en")
	verbs := func(s string) string {
		var b strings.Builder
		for i := 0; i < len(s)-1; i++ {
			if s[i] == '%' {
				b.WriteByte(s[i+1])
				i++
			}
		}
		return b.String()
	}
	en := map[string]string{}
	i18n.SetLocale("en")
	for _, k := range inboxKeys {
		v := i18n.T(k)
		if v == k || strings.TrimSpace(v) == "" {
			t.Errorf("en: %s missing", k)
		}
		en[k] = v
	}
	for _, l := range []string{"ru", "de", "es", "zh"} {
		i18n.SetLocale(l)
		for _, k := range inboxKeys {
			v := i18n.T(k)
			if v == k || strings.TrimSpace(v) == "" || v == en[k] {
				t.Errorf("%s: %s missing or not translated", l, k)
				continue
			}
			if verbs(v) != verbs(en[k]) {
				t.Errorf("%s: %s placeholders %q, en %q", l, k, verbs(v), verbs(en[k]))
			}
		}
	}
}
