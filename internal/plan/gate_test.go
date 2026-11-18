package plan

import (
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
	"github.com/velesbsdllc/agent-vbai/internal/llmpick"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

func TestDecide(t *testing.T) {
	active, _ := Parse([]byte(bodyActive), t0)
	trial, _ := Parse([]byte(bodyTrial), t0)
	grace, _ := Parse([]byte(bodyGrace), t0)
	expired, _ := Parse([]byte(bodyExpired), t0)
	flexible, _ := Parse([]byte(bodyFlexible), t0)
	none, _ := Parse([]byte(bodyNone), t0)
	loggedOut := Plan{LoggedOut: true, CheckedAt: t0}

	cases := []struct {
		name     string
		p        *Plan
		signedIn bool
		own      bool
		now      time.Time
		allowed  bool
		reason   Reason
	}{
		{"active plan", &active, true, false, t0, true, ReasonActive},
		{"trial", &trial, true, false, t0, true, ReasonActive},
		{"grace", &grace, true, false, t0, true, ReasonActive},
		{"expired refused", &expired, true, false, t0, false, ReasonNoPlan},
		{"flexible refused", &flexible, true, false, t0, false, ReasonNoPlan},
		{"no plan refused", &none, true, false, t0, false, ReasonNoPlan},
		{"own source without plan", &none, true, true, t0, true, ReasonOwnSource},
		{"own source signed out", nil, false, true, t0, true, ReasonOwnSource},
		{"own source, plan expired", &expired, true, true, t0, true, ReasonOwnSource},
		{"own source, never verified", nil, true, true, t0, true, ReasonOwnSource},
		{"not signed in", nil, false, false, t0, false, ReasonNotLoggedIn},
		{"401 from billing", &loggedOut, true, false, t0, false, ReasonNotLoggedIn},
		{"never verified", nil, true, false, t0, false, ReasonUnverified},
		{"cache 6 days old", &active, true, false, t0.Add(6 * 24 * time.Hour), true, ReasonActive},
		{"cache 8 days old", &active, true, false, t0.Add(8 * 24 * time.Hour), false, ReasonUnverified},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := Decide(c.p, c.signedIn, c.own, c.now)
			if v.Allowed != c.allowed || v.Reason != c.reason {
				t.Errorf("Decide = %+v, want allowed=%v reason=%v", v, c.allowed, c.reason)
			}
			if v.Allowed && v.Message() != "" {
				t.Errorf("allowed verdict carries a message: %q", v.Message())
			}
			if !v.Allowed && v.Message() == "" {
				t.Error("refusal without a message")
			}
		})
	}
}

func TestVerdictMessage_Locales(t *testing.T) {
	defer i18n.SetLocale("en")

	i18n.SetLocale("ru")
	msg := Verdict{Reason: ReasonNoPlan}.Message()
	for _, want := range []string{
		"Модели ExecAI доступны по подписке",
		"первые 3 дня за 1 ₽",
		SubscribeURL,
		"Свои ключи и локальные модели работают без подписки",
		"/connect zai|kimi|openai|anthropic|openrouter|custom",
		"/source ollama",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("ru no-plan message lacks %q:\n%s", want, msg)
		}
	}
	if got := (Verdict{Reason: ReasonNotLoggedIn}).Message(); !strings.Contains(got, "/login") ||
		!strings.Contains(got, "Для моделей ExecAI войдите") ||
		!strings.Contains(got, "Свои ключи работают без входа") || !strings.Contains(got, "/connect") {
		t.Errorf("ru signed-out message: %q", got)
	}
	if got := (Verdict{Reason: ReasonUnverified}).Message(); !strings.Contains(got, SubscribeURL) {
		t.Errorf("ru unverified message must still say how to subscribe: %q", got)
	}

	i18n.SetLocale("en")
	msg = Verdict{Reason: ReasonNoPlan}.Message()
	for _, want := range []string{"ExecAI models", "1 ₽", SubscribeURL, "Your own keys and local models",
		"/connect zai|kimi|openai|anthropic|openrouter|custom", "/source ollama"} {
		if !strings.Contains(msg, want) {
			t.Errorf("en no-plan message lacks %q:\n%s", want, msg)
		}
	}
	if got := (Verdict{Reason: ReasonNotLoggedIn}).Message(); !strings.Contains(got, "/login") ||
		!strings.Contains(got, "ExecAI models") || !strings.Contains(got, "without signing in") {
		t.Errorf("en signed-out message: %q", got)
	}
}

// Only the ExecAI source needs a plan; every own source — built-in keys,
// local CLIs, a custom endpoint, Ollama local or cloud — works without one.
func TestOwnSource(t *testing.T) {
	defer stubCLIs(true)()
	mk := func(sub subscriptions.Subscription) *subscriptions.Store {
		return &subscriptions.Store{Active: sub.Provider,
			Subscriptions: map[string]subscriptions.Subscription{sub.Provider: sub}}
	}
	cases := []struct {
		name  string
		store *subscriptions.Store
		want  bool
	}{
		{"nil store", nil, false},
		{"empty active", &subscriptions.Store{}, false},
		{"execai", &subscriptions.Store{Active: "execai"}, false},
		{"active points at a missing subscription", &subscriptions.Store{Active: "zai"}, false},
		{"zai", mk(subscriptions.Subscription{Provider: "zai", APIKey: "k"}), true},
		{"zai-api", mk(subscriptions.Subscription{Provider: "zai-api", APIKey: "k"}), true},
		{"kimi", mk(subscriptions.Subscription{Provider: "kimi", APIKey: "k"}), true},
		{"kimi-api", mk(subscriptions.Subscription{Provider: "kimi-api", APIKey: "k"}), true},
		{"openai", mk(subscriptions.Subscription{Provider: "openai", APIKey: "k"}), true},
		{"anthropic", mk(subscriptions.Subscription{Provider: "anthropic", APIKey: "k"}), true},
		{"openrouter", mk(subscriptions.Subscription{Provider: "openrouter", APIKey: "k"}), true},
		{"claude-cli", mk(subscriptions.Subscription{Provider: "claude-cli"}), true},
		{"codex-cli", mk(subscriptions.Subscription{Provider: "codex-cli"}), true},
		{"custom endpoint", mk(subscriptions.Subscription{Provider: "lm-studio", API: "openai", BaseURL: "http://localhost:1234/v1"}), true},
		{"local ollama", mk(subscriptions.Subscription{Provider: "ollama", Plan: "local", BaseURL: "http://localhost:11434"}), true},
		{"ollama cloud", mk(subscriptions.Subscription{Provider: "ollama", Plan: "cloud", APIKey: "k"}), true},
	}
	for _, c := range cases {
		if got := OwnSource(c.store); got != c.want {
			t.Errorf("%s: OwnSource = %v, want %v", c.name, got, c.want)
		}
	}
}

// stubCLIs pretends the claude/codex binaries are (not) installed and
// returns the restore func.
func stubCLIs(installed bool) func() {
	old := llmpick.CLIAvailable
	llmpick.CLIAvailable = func(string) bool { return installed }
	return func() { llmpick.CLIAvailable = old }
}

// A CLI source without its binary really goes to the ExecAI models
// (llmpick falls back): the gate must see that, not the source's name.
func TestRouteOf_CLIFallback(t *testing.T) {
	defer stubCLIs(false)()
	for _, p := range []string{"claude-cli", "codex-cli"} {
		st := &subscriptions.Store{Active: p, Subscriptions: map[string]subscriptions.Subscription{p: {Provider: p}}}
		if r := RouteOf(st); r.Own || r.Fallback != p {
			t.Errorf("%s without its binary: %+v", p, r)
		}
		if OwnSource(st) {
			t.Errorf("%s without its binary counted as own", p)
		}
	}
	zai := &subscriptions.Store{Active: "zai", Subscriptions: map[string]subscriptions.Subscription{"zai": {Provider: "zai"}}}
	if r := RouteOf(zai); !r.Own || r.Fallback != "" {
		t.Errorf("zai: %+v", r)
	}
}

func TestVerdict_FallbackNote(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("ru")
	msg := Verdict{Reason: ReasonNoPlan, Fallback: "claude-cli"}.Message()
	if !strings.Contains(msg, "Бинарь claude не найден") || !strings.Contains(msg, "модели ExecAI") ||
		!strings.Contains(msg, SubscribeURL) {
		t.Errorf("ru fallback refusal:\n%s", msg)
	}
	if strings.Count(strings.SplitN(msg, "\n", 2)[0], "\n") != 0 {
		t.Error("the note must be one line")
	}
	i18n.SetLocale("en")
	if msg := (Verdict{Reason: ReasonNotLoggedIn, Fallback: "codex-cli"}).Message(); !strings.Contains(msg, "codex binary not found") ||
		!strings.Contains(msg, "/login") {
		t.Errorf("en fallback signed-out:\n%s", msg)
	}
	if msg := (Verdict{Reason: ReasonNoPlan}).Message(); strings.Contains(msg, "not found") {
		t.Errorf("note without a fallback:\n%s", msg)
	}
}

func TestRuMessages_KeyPlaceholder(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("ru")
	for _, r := range []Reason{ReasonNoPlan, ReasonNotLoggedIn} {
		msg := Verdict{Reason: r}.Message()
		if strings.Contains(msg, "<key>") || !strings.Contains(msg, "<ключ>") {
			t.Errorf("ru message must say <ключ>: %q", msg)
		}
	}
}
