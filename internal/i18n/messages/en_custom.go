// Package messages/en_custom — English strings for live model lists and the
// custom provider (/connect custom). Merged into the "en" catalog.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enCustomMessages = map[string]string{
	// === live model lists ===
	"catalog.cliDefault":               "Default",
	"catalog.cliDefaultDesc":           "no --model: the model from the CLI's own settings",
	"subs.connect.modelsCount":         "Models reported by the provider: %d — /model to pick.",
	"subs.connect.noModels":            "The provider reported no models. Type one yourself: /model <id> — it is remembered for this source.",
	"subs.connect.ollamaLocalFailed":   "✗ Ollama at %s did not answer (HTTP %d). Is it running? `ollama serve`",
	"subs.connect.ollamaCloudRejected": "✗ ollama.com rejected the key (HTTP %d). Keys: https://ollama.com/settings/keys",
	"ui.model.listEmpty":               "This source has not reported any models yet. Type one: /model <id>",
	"ui.model.typed":                   "Model %q is not in the provider's list — using it as typed and remembering it for this source.",
	"hint.connect.custom":              "your own provider: any OpenAI- or Anthropic-compatible address",
	"subs.connectHint.custom":          "your own provider: protocol, address, key — step by step",
	"subs.hint.custom":                 "custom (%s) · %s",
	"subs.custom.usage": "Usage:\n" +
		"  /connect custom                                — step by step\n" +
		"  /connect custom <name> <openai|anthropic> <base_url> [api_key] [--models a,b] [--header \"Name: value\"]\n" +
		"Examples:\n" +
		"  /connect custom \"LM Studio\" openai http://localhost:1234/v1\n" +
		"  /connect custom deepseek openai https://api.deepseek.com sk-xxx\n" +
		"  /connect custom corp anthropic https://llm.corp.local sk-xxx --header \"X-Tenant: dev\"",

	// === /connect custom: step by step ===
	"custom.wiz.intro":              "Connecting your own provider — any server that speaks the OpenAI or the Anthropic API. Esc cancels.",
	"custom.wiz.name":               "Step 1/4. What should it be called? For example: LM Studio, vLLM, deepseek, corp-gateway (latin letters — the name becomes the id for /source).",
	"custom.wiz.idIs":               "id: %s  (switch to it later with /source <id>)",
	"custom.wiz.api":                "Step 2/4. Interface type:\n  1 — OpenAI-compatible (POST …/chat/completions): LM Studio, vLLM, llama.cpp, LiteLLM, DeepSeek, Groq, Together, Mistral…\n  2 — Anthropic-compatible (POST …/v1/messages): LiteLLM, Claude gateways, MiniMax, DeepSeek…\nType 1 or 2.",
	"custom.wiz.urlOpenAI":          "Step 3/4. Server address (base URL), e.g. http://localhost:1234/v1, http://gpu-box:8000/v1, https://api.deepseek.com",
	"custom.wiz.urlAnthropic":       "Step 3/4. Server address (base URL) without /v1/messages, e.g. https://llm.corp.local, https://api.deepseek.com/anthropic",
	"custom.wiz.key":                "Step 4/4. API key — Enter for none (local servers often have none). Stored only on this machine and never written to the input history.",
	"custom.wiz.noKey":              "(no key)",
	"custom.wiz.keyRejected":        "✗ The server rejected the key (HTTP %d). Type it again, or Esc to cancel.",
	"custom.wiz.models":             "The server did not report a model list (GET …/models: HTTP %d). Type the model IDs separated by commas, or Enter to skip and set one later with /model <id>.",
	"custom.wiz.cancelled":          "Connecting a custom provider cancelled.",
	"custom.wiz.placeholder.name":   "provider name, e.g. LM Studio",
	"custom.wiz.placeholder.api":    "1 — OpenAI-compatible, 2 — Anthropic-compatible",
	"custom.wiz.placeholder.url":    "base URL, e.g. http://localhost:1234/v1",
	"custom.wiz.placeholder.key":    "API key, or Enter for none",
	"custom.wiz.placeholder.models": "model IDs separated by commas, or Enter",
	"custom.keyRejected":            "✗ %[2]s rejected the key (HTTP %[1]d).",
	"custom.connected":              "✓ Connected: %s (id %s, %s protocol, %s)",
	"custom.connectedModels":        "Models: %d, current: %s — /model to pick.",
	"custom.connectedHint":          "Switch back later: /source %[1]s · remove: /disconnect %[1]s",
}

func init() { i18n.Register("en", enCustomMessages) }
