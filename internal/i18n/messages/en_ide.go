// Package messages/en_ide — English strings the agent sends to the editor
// panel (execai ide): the permission buttons and the list of sources to
// connect.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enIDEMessages = map[string]string{
	// The descriptions under the permission buttons; the buttons themselves
	// are approve.* (shared with the terminal chat).
	"ide.approve.onceDesc":    "allow only this call",
	"ide.approve.sessionDesc": "until the end of this chat",
	"ide.approve.exactDesc":   "exactly this call until the end of the chat",
	"ide.approve.foreverDesc": "remember this permission — it is written to permissions.json",
	"ide.approve.denyDesc":    "do not run it; the agent gets a refusal",

	"ide.connect.withKey":    "%s (key)",
	"ide.connect.noKey":      "%s (no key)",
	"ide.connect.zaiAPI":     "Z.ai platform, pay-per-token (key)",
	"ide.connect.openrouter": "OpenRouter — every vendor with one key (key)",
	"ide.connect.ollama":     "Ollama — local or cloud (key for the cloud)",
	"ide.connect.custom":     "Custom provider — any OpenAI- or Anthropic-compatible address",

	// Notices and errors the panel shows (execai ide), the security levels
	// and the errors of the sign-in and connect paths from other packages.
	"ide.err.effort":            "effort: off|low|medium|high|max or a number",
	"ide.err.positive":          "a positive number is needed",
	"ide.maxIter.set":           "tool iterations per turn: %d",
	"ide.login.startFailed":     "could not start the sign-in",
	"ide.login.saveFailed":      "could not save the token",
	"ide.login.expired":         "the confirmation code expired — try again",
	"ide.login.timeout":         "the sign-in was not confirmed in time",
	"ide.connect.keyRejectedBy": "%s rejected the key (HTTP %d)",
	"ide.connect.noModels":      "connected: %s (%s) — the server sent no model list (HTTP %d); reconnect with the model IDs",
	"ide.connect.ollamaKey":     "ollama.com did not accept the key (HTTP %d)",
	"ide.connect.ollamaDown":    "Ollama at %s did not answer (HTTP %d) — is it running?",
	"ide.connect.unknown":       "unknown source: %s",
	"ide.connect.needKey":       "the provider key is needed",
	"ide.connect.keyRejected":   "key rejected (HTTP %d) — check that it is a %s key",
	"ide.connect.done":          "connected: %s",
	"ide.connect.models":        " — models available: %d",
	"ide.connect.unverified":    " (key saved; it is checked on the first request)",
	"ide.turn.queued":           "turn queued: other chats are still answering",
	"ide.err.chdir":             "cannot change to the folder %s",
	"ide.err.noModel":           "could not pick a model — check /model in execai",
	"ide.err.noModelShort":      "no model",
	"ide.err.badMessage":        "cannot parse the message: %s",
	"ide.err.noChat":            "no such chat: %s",
	"ide.subs.unreadable":       "subscriptions.json could not be read (%s) — working on the base source; the subscriptions are not lost, but the file needs fixing",
	"ide.err.noSuchModel":       "the current source has no such model: %s",
	"ide.err.unknownLevel":      "unknown level: %s",
	"ide.security.set":          "security level: %s",
	"ide.logout.done":           "signed out of ExecAI (the subscriptions stay)",
	"ide.disconnect.done":       "disconnected: %s",
	"ide.resume.none":           "nothing to continue — this is the first chat of the project",
	"ide.resume.failed":         "cannot open the last chat: %s",
	"ide.chat.openFailed":       "cannot open the chat: %s",
	"ide.wake.none":             "no wake-up was scheduled",
	"ide.err.unknownCommand":    "unknown command: %s",
	"ide.err.unknownType":       "unknown type: %s",
	"ide.chat.busy":             "this chat is still answering — wait for the turn to end or stop it",
	"security.level.light":      "light — stay out of the way",
	"security.level.deep":       "standard — the project silently, outside it with a question",
	"security.level.paranoid":   "strict — ask about everything",
	"connect.err.api":           "interface type %q is not supported: openai (chat/completions) or anthropic (messages)",
	"connect.err.noURL":         "a server address is needed, e.g. http://localhost:1234/v1",
	"connect.err.badURL":        "address %q does not look like an http(s) URL",
	"subs.err.nameLatin":        "the name needs Latin letters or digits (the id for /source is made of it)",
	"subs.err.nameLong":         "the name is longer than 40 characters",
	"subs.err.builtin":          "%q is a built-in source — pick another name",
	"subs.err.notLoaded":        "subscriptions are not loaded",
	"subs.err.notConnected":     "subscription %q is not connected — first /connect %s",
	"auth.err.linkStart":        "agent-link/start returned %d: %s",
	"auth.err.linkPoll":         "agent-link/poll returned %d: %s",
	"auth.err.pollFailed":       "polling failed 10 times in a row",
	"auth.err.linkExpired":      "link expired — run /login again",
}

func init() { i18n.Register("en", enIDEMessages) }
