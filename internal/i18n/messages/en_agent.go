// Package messages/en_agent — English strings of the agent loop and
// the model catalog that the person sees: a declined tool, the scope of a
// permission, working without the catalog. Merged into the "en" catalog
// via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enAgentMessages = map[string]string{
	"agent.tool.declined":          "The user declined to run this tool.",
	"agent.tool.noApprovalChannel": "execai cannot ask for confirmation of this call here",
	"agent.approve.scope":          "the permission will cover: %s",
	"llm.offline.fallback":         "⚠ Working offline — the model catalog is unavailable, using the built-in list. Reason: %v",
	"llm.offline.cache":            "ℹ Using the cached model catalog (age: %s). The network is unavailable: %v",
	"llm.models.unauthorized":      "401 from /billing-vbai/models_public — the session has expired, sign in again (in the chat: /login)",
	"llm.models.status":            "the model catalog answered %d: %s",
	"llm.models.notJSON":           "the model catalog answer is not a list: %s",
	"llm.models.none":              "neither the network nor a cached model catalog is available",
	"llm.err.unauthorized":         "401 from %s — %s",
	"llm.err.status":               "the server answered %d: %s",
	"llm.err.server":               "the server returned an error: %v",
	"llm.err.aichatUnauthorized":   "401 from /aichat-vbai/stream — the token has expired, run execai login",
	"llm.err.sse":                  "server error in the SSE stream: %v",
	"llm.err.anthropicKey":         "401 from the Anthropic API — wrong key. Check /subscriptions",
	"llm.err.providerKey":          "401 from %s — wrong API key. Check /subscriptions and /connect",
	"llm.err.providerStatus":       "%s answered %d: %s",
	"llm.billing.limit":            "⏱  LIMIT REACHED",
	"llm.billing.funds":            "💰 NOT ENOUGH FUNDS",
	"llm.billing.inactive":         "⚠  PLAN INACTIVE",
	"llm.billing.unavailable":      "The service is temporarily unavailable for billing reasons.",
	"llm.billing.hint":             "Change the plan: the \"Plan\" section of your ExecAI web account (%s)",
	"llm.cli.claudeMissing":        "claude CLI not found (PATH, ~/.local/bin, VS Code/Cursor extensions). Install: https://docs.claude.com/en/docs/claude-code/quickstart",
	"llm.cli.claudeNotFound":       "claude CLI not found",
	"llm.cli.claudeNoModels":       "claude returned no models",
	"llm.cli.codexMissing":         "codex CLI not found in PATH. Install: https://github.com/openai/codex\nAfter installing: codex login",
	"llm.cli.codexNotFound":        "codex CLI not found in PATH",
	"llm.cli.codexNoModels":        "codex returned no models",
	"llm.cli.noAnswer":             "the CLI exited without answering",
	"llm.cli.emptyResult":          "empty result",
	"llm.cli.emptyAnswer":          "empty answer",
	"llm.cli.images":               "images",
	"llm.cli.noToolResult":         "no result: claude exited early",
	"llm.ollama.down":              "Ollama %s is unavailable",
	"llm.ollama.start":             "Start it: ollama serve",
	"llm.ollama.noModel":           "Ollama: model %q is not installed (ollama pull %s?). %s",
	"llm.ollama.status":            "Ollama answered %d: %s",
	"llm.ollama.tagsStatus":        "Ollama /api/tags answered %d: %s",
	"llm.ollama.tagsParse":         "Ollama /api/tags: cannot read the answer",
	"llm.ollama.noModels":          "Ollama has no models. Install one: `ollama pull llama3.2`",
	"llm.ollama.localModel":        "local Ollama model",
	"agent.memory.readFailed":      "reading the memory",
	"agent.memory.exportDirFailed": "creating the export folder",
	"agent.memory.writeFailed":     "writing %s",
	"agent.permissions.saveFailed": "could not save the permission:",
	"auth.notSignedIn":             "you are not signed in. Run: execai login",
}

func init() { i18n.Register("en", enAgentMessages) }
