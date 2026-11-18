package ide

import (
	"regexp"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

var cyrillic = regexp.MustCompile(`[А-Яа-яЁё]`)

func testApprover() *ideApprover {
	return &ideApprover{c: newTestChat(), sessionTools: map[string]bool{}, sessionExact: map[string]bool{}}
}

// The permission buttons and their descriptions follow the panel language;
// the values the panel answers with stay the same in every language.
func TestQuestionLabels_Locale(t *testing.T) {
	defer i18n.SetLocale("en")
	values := []string{inbox.AnswerOnce, inbox.AnswerSession, inbox.AnswerExact, inbox.AnswerAlways, inbox.AnswerDeny}

	i18n.SetLocale("en")
	_, _, out := testApprover().question("Bash", "ls")
	if out.Type != "ask" || len(out.Options) != len(values) {
		t.Fatalf("event %+v", out)
	}
	for i, o := range out.Options {
		if o.Value != values[i] {
			t.Errorf("option %d value %q, want %q", i, o.Value, values[i])
		}
		if cyrillic.MatchString(o.Label + o.Description) {
			t.Errorf("en: Russian text in %+v", o)
		}
		if o.Label == "" || o.Description == "" {
			t.Errorf("en: empty option %+v", o)
		}
	}
	if out.Options[0].Label != "Once" || out.Options[1].Label != "All Bash this session" || out.Options[4].Label != "Deny" {
		t.Errorf("en labels %+v", out.Options)
	}

	i18n.SetLocale("ru")
	_, _, out = testApprover().question("Bash", "ls")
	if out.Options[0].Label != "Разово" || out.Options[1].Label != "Весь Bash в сессии" || out.Options[3].Label != "НАВСЕГДА" {
		t.Errorf("ru labels %+v", out.Options)
	}
	if !strings.Contains(out.Options[3].Description, "permissions.json") {
		t.Errorf("ru: the always option must name the file: %q", out.Options[3].Description)
	}
	for i, o := range out.Options {
		if o.Value != values[i] {
			t.Errorf("ru: option %d value %q", i, o.Value)
		}
	}
}

// The "connect a source" list follows the panel language; the ids and the
// set of sources do not change.
func TestConnectableOptions_Locale(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	en := ConnectableOptions()
	i18n.SetLocale("ru")
	ru := ConnectableOptions()
	if len(en) != len(ru) || len(en) != 11 {
		t.Fatalf("en %d, ru %d options", len(en), len(ru))
	}
	for i := range en {
		if en[i].ID != ru[i].ID {
			t.Errorf("option %d: id %q vs %q", i, en[i].ID, ru[i].ID)
		}
		if cyrillic.MatchString(en[i].Label) {
			t.Errorf("en: Russian label %q", en[i].Label)
		}
	}
	byID := func(list []NamedItem, id string) string {
		for _, it := range list {
			if it.ID == id {
				return it.Label
			}
		}
		return ""
	}
	if got := byID(en, subscriptions.SourceCustom); got != "Custom provider — any OpenAI- or Anthropic-compatible address" {
		t.Errorf("en custom: %q", got)
	}
	if got := byID(ru, subscriptions.SourceCustom); got != "Свой провайдер — OpenAI- или Anthropic-совместимый адрес" {
		t.Errorf("ru custom: %q", got)
	}
	if got := byID(en, subscriptions.SourceKimi); got != "Kimi Code (key)" {
		t.Errorf("en kimi: %q", got)
	}
	if got := byID(ru, subscriptions.SourceClaudeCLI); got != "Claude Code CLI (без ключа)" {
		t.Errorf("ru claude-cli: %q", got)
	}
}
