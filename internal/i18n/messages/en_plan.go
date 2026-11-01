// Package messages/en_plan — English strings for the subscription gate and
// /plan. Merged into the "en" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enPlanMessages = map[string]string{
	// === gate ===
	"plan.gate.noPlan": "ExecAI models come with a plan: any plan, the first 3 days cost 1 ₽.\n" +
		"Subscribe: %s\n" +
		"Your own keys and local models work without a plan: " +
		"/connect zai|kimi|openai|anthropic|openrouter|custom <key>, /source ollama",
	"plan.gate.notLoggedIn": "For ExecAI models, sign in: run execai and confirm in the browser (in chat: /login).\n" +
		"Your own keys work without signing in: " +
		"/connect zai|kimi|openai|anthropic|openrouter|custom <key>, /source ollama",
	"plan.gate.fallback": "%s binary not found — falling back to the ExecAI models.",
	"plan.gate.unverified": "Could not verify your ExecAI plan: billing is unreachable and the last " +
		"successful check is older than 7 days (or there was none).",

	// === /plan ===
	"plan.show.header":         "ExecAI plan",
	"plan.show.state":          "  State       : %s",
	"plan.show.tariff":         "  Plan        : %s",
	"plan.show.validUntil":     "  Valid until : %s",
	"plan.show.checked":        "  Checked     : %s",
	"plan.show.checkedOffline": "  Checked     : %s — billing unreachable, showing the saved answer",
	"plan.show.allowed":        "  Allowed     : %s",
	"plan.show.features":       "  Sync and web console: %s",
	"plan.show.subscribe":      "Subscribe: %s",

	"plan.state.active":     "active",
	"plan.state.trial":      "active (trial)",
	"plan.state.cancelled":  "active, without auto-renewal",
	"plan.state.grace":      "grace: the last charge failed, access until %s",
	"plan.state.none":       "no active plan",
	"plan.state.flexible":   "no active plan (pay-as-you-go balance only)",
	"plan.state.expired":    "expired",
	"plan.state.signedOut":  "not signed in",
	"plan.state.unknown":    "unknown: billing has not answered yet",
	"plan.state.unverified": "unverified: billing unreachable, the last check is older than 7 days",

	"plan.allowed.all":     "every source",
	"plan.allowed.ownOnly": "your own keys and local models; ExecAI models need a plan",
	"plan.features.yes":    "available",
	"plan.features.no":     "requires the Work or Max plan",

	// === execai serve ===
	"plan.serve.refused":   "✗ task %s refused by the plan check, the model was not called",
	"plan.serve.startWarn": "⚠ Tasks on the ExecAI models will be refused until this is resolved:",

	"hint.plan": "ExecAI plan: state and what it allows",
}

func init() { i18n.Register("en", enPlanMessages) }
