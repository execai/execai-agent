package i18n_test

import (
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
)

// TestHelpBody_AllLocales — help.body exists, is multi-line and contains no
// raw escape artifacts ("\\n" as text) in any locale.
func TestHelpBody_AllLocales(t *testing.T) {
	for _, l := range []string{"en", "ru", "es", "de", "zh"} {
		i18n.SetLocale(l)
		s := i18n.T("help.body")
		if s == "help.body" {
			t.Errorf("%s: help.body missing", l)
			continue
		}
		if !strings.Contains(s, "\n") {
			t.Errorf("%s: help.body is single-line — broken escape?", l)
		}
		if strings.Contains(s, `\n`) {
			t.Errorf("%s: help.body contains literal backslash-n", l)
		}
		if !strings.Contains(s, "/source") {
			t.Errorf("%s: help.body doesn't mention /source", l)
		}
	}
	i18n.SetLocale("en")
}

// TestImportKeys_RuEn — every string of the import flow (E100) exists in
// Russian and English, with the same placeholders in both.
func TestImportKeys_RuEn(t *testing.T) {
	keys := []string{
		"import.kind.memory", "import.kind.rules", "import.kind.personal",
		"import.kind.global", "import.kind.globalLegacy", "import.kind.agentsStd",
		"import.kind.pathRules", "import.kind.projectMemory",
		"import.skip.secret", "import.skip.already", "import.skip.exists",
		"import.skip.read", "import.skip.write",
		"import.badFrom", "import.fm.description", "import.indexLine",
		"import.offer.question", "import.offer.more", "import.offer.yes", "import.offer.yesDesc",
		"import.offer.no", "import.offer.noDesc", "import.offer.declined",
		"import.cmd.short", "import.cmd.long", "import.cmd.flagDryRun",
		"import.cmd.flagFrom", "import.cmd.flagYes", "import.cmd.nothing",
		"import.cmd.found", "import.cmd.notTransferred", "import.cmd.dryRunDone",
		"import.cmd.nothingNew", "import.cmd.confirm", "import.cmd.cancelled",
		"import.cmd.done", "memory.nothingToImport",
	}
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
	defer i18n.SetLocale("en")
	got := map[string]map[string]string{}
	for _, l := range []string{"ru", "en"} {
		i18n.SetLocale(l)
		got[l] = map[string]string{}
		for _, k := range keys {
			v := i18n.T(k)
			if v == k || strings.TrimSpace(v) == "" {
				t.Errorf("%s: %s missing", l, k)
			}
			got[l][k] = v
		}
	}
	for _, k := range keys {
		if verbs(got["ru"][k]) != verbs(got["en"][k]) {
			t.Errorf("%s: placeholders differ: ru %q vs en %q", k, verbs(got["ru"][k]), verbs(got["en"][k]))
		}
		if got["ru"][k] == got["en"][k] && !strings.Contains(k, "short") {
			t.Errorf("%s: ru is the English text", k)
		}
	}
	if !strings.Contains(got["ru"]["import.cmd.confirm"], "[Y/n]") || !strings.Contains(got["en"]["import.cmd.confirm"], "[Y/n]") {
		t.Error("import.cmd.confirm must show [Y/n]")
	}
}

// planKeys — every user-facing string of the subscription gate and /plan.
var planKeys = []string{
	"plan.gate.noPlan", "plan.gate.notLoggedIn", "plan.gate.unverified",
	"plan.show.header", "plan.show.state", "plan.show.tariff", "plan.show.validUntil",
	"plan.show.checked", "plan.show.checkedOffline", "plan.show.allowed",
	"plan.show.features", "plan.show.subscribe",
	"plan.state.active", "plan.state.trial", "plan.state.cancelled", "plan.state.grace",
	"plan.state.none", "plan.state.flexible", "plan.state.expired", "plan.state.signedOut",
	"plan.state.unknown", "plan.state.unverified",
	"plan.allowed.all", "plan.allowed.ownOnly", "plan.gate.fallback",
	"plan.features.yes", "plan.features.no",
	"plan.serve.refused", "plan.serve.startWarn",
	"hint.plan",
}

// TestPlanKeys_RuEn — the gate speaks both languages: each key exists in en
// and has its own ru text (not the en fallback).
func TestPlanKeys_RuEn(t *testing.T) {
	defer i18n.SetLocale("en")
	for _, k := range planKeys {
		i18n.SetLocale("en")
		en := i18n.T(k)
		if en == k || en == "" {
			t.Errorf("en: %s missing", k)
			continue
		}
		i18n.SetLocale("ru")
		ru := i18n.T(k)
		if ru == k || ru == "" || ru == en {
			t.Errorf("ru: %s missing or not translated", k)
		}
	}
}
