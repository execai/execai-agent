package i18n_test

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
)

// fleetKeys — every string of subagents and the agents list (E101).
var fleetKeys = []string{
	"fleet.ask.prefix", "fleet.card.write", "fleet.card.background",
	"fleet.state.queued", "fleet.state.running", "fleet.state.waiting",
	"fleet.state.done", "fleet.state.failed", "fleet.state.cancelled",
	"fleet.started", "fleet.result.timeout", "fleet.result.cancelled",
	"fleet.result.stopped", "fleet.result.limit", "fleet.wait.none",
	"fleet.wait.unknown", "fleet.note.header", "fleet.note.item",
	"fleet.err.model", "fleet.err.cwdOutside", "fleet.err.cwdMissing",
	"fleet.tui.header", "fleet.tui.more", "fleet.tui.listHeader",
	"fleet.tui.listEmpty", "fleet.tui.listHint", "fleet.tui.openHeader",
	"fleet.tui.openEmpty", "fleet.tui.notFound", "fleet.tui.stopping",
	"fleet.tui.notRunning", "fleet.tui.usage", "fleet.tui.autoReply",
	"fleet.tui.cardDone", "fleet.tui.approveAgent", "fleet.tui.stoppedChildren",
	"status.agents", "hint.agents", "fleet.ide.notFound", "fleet.ide.stopping",
	"fleet.extend.raised", "fleet.extend.resumed", "fleet.extend.ceiling", "fleet.extend.finished",
	"fleet.extend.prompt", "fleet.tui.steps", "fleet.tui.atLimit", "fleet.tui.moreHint",
	"fleet.tui.limitShow", "fleet.tui.limitSet", "fleet.tui.limitBad",
}

var verbRe = regexp.MustCompile(`%[0-9.]*[a-zA-Z%]`)

// TestFleetKeys_AllLocales — each fleet string exists in all five
// languages, is translated (not the English fallback, except where a word is
// the same) and keeps the English placeholders.
func TestFleetKeys_AllLocales(t *testing.T) {
	defer i18n.SetLocale("en")
	same := map[string]bool{"fleet.note.item": true, "fleet.tui.openHeader": true}
	verbs := func(s string) string {
		v := verbRe.FindAllString(s, -1)
		sort.Strings(v)
		return strings.Join(v, " ")
	}
	en := map[string]string{}
	i18n.SetLocale("en")
	for _, k := range fleetKeys {
		v := i18n.T(k)
		if v == k || strings.TrimSpace(v) == "" {
			t.Errorf("en: %s missing", k)
		}
		en[k] = v
	}
	for _, l := range []string{"ru", "de", "es", "zh"} {
		i18n.SetLocale(l)
		for _, k := range fleetKeys {
			v := i18n.T(k)
			if v == k || strings.TrimSpace(v) == "" || (v == en[k] && !same[k]) {
				t.Errorf("%s: %s missing or not translated", l, k)
				continue
			}
			if verbs(v) != verbs(en[k]) {
				t.Errorf("%s: %s placeholders %q, en %q", l, k, verbs(v), verbs(en[k]))
			}
		}
	}
}

// Every fleet.* key defined in English is in the list above, so a new string
// cannot skip the check.
func TestFleetKeys_ListIsComplete(t *testing.T) {
	listed := map[string]bool{}
	for _, k := range fleetKeys {
		listed[k] = true
	}
	for _, k := range i18n.Keys("en") {
		if strings.HasPrefix(k, "fleet.") && !listed[k] {
			t.Errorf("%s is not in fleetKeys", k)
		}
	}
}
