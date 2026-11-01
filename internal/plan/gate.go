package plan

import (
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llmpick"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// SubscribeURL — where the gate sends people to pick a plan.
const SubscribeURL = "https://chat.execai.ru/settings/payment?utm_source=agent&utm_medium=cli&utm_campaign=gate"

// Reason explains a gate verdict.
type Reason int

const (
	// ReasonActive — the plan is active.
	ReasonActive Reason = iota
	// ReasonOwnSource — the person's own source (a provider key, a local CLI,
	// a custom endpoint, Ollama): no plan and no login needed.
	ReasonOwnSource
	// ReasonNoPlan — signed in, but no active plan (none, pay-as-you-go
	// balance only, or expired).
	ReasonNoPlan
	// ReasonNotLoggedIn — no ExecAI token, or billing rejected it (401).
	ReasonNotLoggedIn
	// ReasonUnverified — billing could not be reached and the saved answer
	// is missing or older than OfflineGrace.
	ReasonUnverified
)

// Verdict is the gate's answer for one model call.
type Verdict struct {
	Allowed bool
	Reason  Reason
	// Fallback — the own CLI source (claude-cli, codex-cli) whose binary is
	// missing, so the call went to the ExecAI models; the refusal says so.
	Fallback string
}

// Route — where the next model call really goes, as the gate needs it.
type Route struct {
	// Own — the person's own source: no plan and no login needed.
	Own bool
	// Fallback — an own CLI source without its binary: llmpick.Client falls
	// back to the ExecAI backend, so the call is checked like the ExecAI
	// models. Empty otherwise.
	Fallback string
}

// Decide is the gate itself, free of I/O.
//
//	p        — the latest known answer from billing, nil if there is none;
//	signedIn — an ExecAI token is present;
//	own      — the active source is the person's own, not the ExecAI models;
//	now      — the moment of the model call.
func Decide(p *Plan, signedIn, own bool, now time.Time) Verdict {
	switch {
	case own:
		return Verdict{Allowed: true, Reason: ReasonOwnSource}
	case !signedIn, p != nil && p.LoggedOut:
		return Verdict{Reason: ReasonNotLoggedIn}
	case p == nil:
		return Verdict{Reason: ReasonUnverified}
	case p.Active(now):
		return Verdict{Allowed: true, Reason: ReasonActive}
	case p.Granted():
		// The plan was active, but the answer is older than OfflineGrace.
		return Verdict{Reason: ReasonUnverified}
	default:
		return Verdict{Reason: ReasonNoPlan}
	}
}

// Message is the text shown instead of a model answer when the gate says
// no. Empty for an allowed call.
func (v Verdict) Message() string {
	if v.Allowed {
		return ""
	}
	var msg string
	switch v.Reason {
	case ReasonNotLoggedIn:
		msg = i18n.T("plan.gate.notLoggedIn")
	case ReasonUnverified:
		msg = i18n.T("plan.gate.unverified") + "\n" + i18n.Tf("plan.gate.noPlan", SubscribeURL)
	default:
		msg = i18n.Tf("plan.gate.noPlan", SubscribeURL)
	}
	if v.Fallback != "" {
		msg = FallbackNote(v.Fallback) + "\n" + msg
	}
	return msg
}

// FallbackNote — one line: the CLI behind provider is not installed, so the
// agent falls back to the ExecAI models.
func FallbackNote(provider string) string {
	bin := strings.TrimSuffix(provider, "-cli")
	return i18n.Tf("plan.gate.fallback", bin)
}

// RouteOf tells where the next call goes. Own — any connected active
// subscription: a provider key (Z.ai, Kimi, OpenAI, Anthropic, OpenRouter…),
// a local CLI (Claude Code, Codex), a custom endpoint, Ollama local or cloud.
// Those work without an ExecAI plan and without a login; only the ExecAI
// models need a plan. Mirrors llmpick.Client, including its fallback: no
// active subscription, or a CLI source whose binary is missing, means the
// ExecAI backend.
func RouteOf(subs *subscriptions.Store) Route {
	a := subs.ActiveSubscription()
	if a == nil {
		return Route{}
	}
	switch a.Provider {
	case subscriptions.SourceClaudeCLI, subscriptions.SourceCodexCLI:
		if !llmpick.CLIAvailable(a.Provider) {
			return Route{Fallback: a.Provider}
		}
	}
	return Route{Own: true}
}

// OwnSource — RouteOf(subs).Own.
func OwnSource(subs *subscriptions.Store) bool { return RouteOf(subs).Own }

// OnExecAI reads subscriptions.json and reports whether the active source
// goes to the ExecAI models — the condition for background plan refreshes
// (Checker.Start) in processes that do not own the store in memory.
func OnExecAI() bool {
	subs, _ := subscriptions.Load()
	return !RouteOf(subs).Own
}
