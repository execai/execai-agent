// Package messages/en_subs — English strings for subscription/source commands
// (/connect, /disconnect, /source, /subscriptions). Merged into the "en"
// catalog via i18n.Register (see en.go for the base catalog).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var enSubsMessages = map[string]string{
	// === /disconnect ===
	"subs.noSubs":              "no subscriptions",
	"subs.disconnect.notFound": "subscription %q not found",
	"subs.disconnect.none":     "No connected subscriptions.",
	"subs.disconnect.header":   "/disconnect <provider>. Connected:\n",
	"subs.saveError":           "failed to save: %s",
	"subs.disconnected":        "✓ %s disconnected",

	// === /source (bare list) ===
	"subs.useDeprecated":     "ℹ /use — deprecated, use /source\n\n",
	"subs.source.listHeader": "Available sources (/source <name>):\n",
	"subs.source.listExecai": "  • execai  — our billing (default)\n",
	"subs.source.listItem":   "  • %-8s — %s subscription\n",
	"subs.source.listFooter": "\nTip: type '/source ' and press Tab for a menu.",

	// === /source <name> how-to on failure ===
	"subs.howto.zai": "\n\nHow to connect Z.ai:\n" +
		"  1. Get a key at https://z.ai/manage-apikey/apikey-list\n" +
		"     (Individual Coding Plan → Plan Overview section)\n" +
		"  2. /connect zai sk-zai-XXXXXX\n" +
		"  3. /source zai",
	"subs.howto.anthropic": "\n\nHow to connect Anthropic:\n" +
		"  1. Get an API key at https://console.anthropic.com/settings/keys\n" +
		"     (format sk-ant-... pay-per-token billing)\n" +
		"  2. /connect anthropic sk-ant-XXXXXX\n" +
		"  3. /source anthropic",
	"subs.sourceSwitched": "✓ source: %s · model: %s",

	// === /connect (bare) ===
	"subs.connect.usageShort": "Usage: /connect <provider> <api_key>\n" +
		"Supported: zai (Z.ai Coding Plan)\n" +
		"Tip: '/connect ' + Tab shows the provider menu.",

	// === /subscriptions ===
	"subs.list.empty": "No connected subscriptions.\n" +
		"Connect: /connect zai (Z.ai Coding Plan)\n" +
		"Current source: ExecAI (our billing)",
	"subs.list.header":         "Connected subscriptions (active: %s):\n",
	"subs.list.switchHint":     "\nSwitch:     /source <provider>  (or /source execai → our billing)\n",
	"subs.list.disconnectHint": "Disconnect: /disconnect <provider>",

	// === Provider descriptions (/source picker) ===
	"subs.provider.execai":    "our billing (default)",
	"subs.provider.zai":       "Z.ai GLM Coding Plan",
	"subs.provider.kimi":      "Kimi Code Coding Plan (K3/K2.7, kimi.com/code subscription)",
	"subs.provider.kimiapi":   "Moonshot Platform (pay-per-token, platform.moonshot.ai)",
	"subs.provider.anthropic": "Anthropic API (sk-ant-…)",
	"subs.provider.openai":    "OpenAI Platform (sk-… from platform.openai.com, pay-per-token)",
	"subs.provider.codexcli":  "local OpenAI Codex CLI (ChatGPT Plus/Pro quota)",
	"subs.provider.claudecli": "local Claude Code (Pro/Max subscription quota)",
	"subs.provider.ollama":    "local Ollama runner (localhost:11434)",

	// === Picker hints ===
	"subs.hint.connected":     "connected",
	"subs.hint.connectedPlan": "connected · %s",
	"subs.hint.notConnected":  "not connected — /connect %s",
	"subs.hint.remove":        "remove subscription",

	// === /connect picker hints ===
	"subs.connectHint.zai":       "Z.ai GLM Coding Plan (Coding Plan API key)",
	"subs.connectHint.kimi":      "Kimi Code Coding Plan (K3/K2.7, key from kimi.com/code/console)",
	"subs.connectHint.kimiapi":   "Moonshot Platform pay-per-token (key from platform.moonshot.ai)",
	"subs.connectHint.anthropic": "Claude API (sk-ant-... from console.anthropic.com)",
	"subs.connectHint.openai":    "OpenAI Platform pay-per-token (sk-… from platform.openai.com)",
	"subs.connectHint.codexcli":  "local OpenAI Codex CLI (no key, `codex` must be installed)",
	"subs.connectHint.claudecli": "local Claude Code (no key, `claude` must be installed)",
	"subs.connectHint.ollama":    "local Ollama runner (localhost:11434, no key)",

	// === /connect flow ===
	"subs.connect.usage": "Usage: /connect <provider> <api_key> [base_url]\n" +
		"Supported: zai (Z.ai Coding Plan)\n" +
		"Example:  /connect zai sk-zai-XXXXX\n" +
		"          /connect zai sk-zai-XXXXX https://open.bigmodel.cn/api/paas/v4  (for CN)",
	"subs.connect.claudecliOK": "✓ claude-cli connected (quota from your Pro/Max subscription via Claude Code OAuth).\n" +
		"Switch: /source claude-cli\n" +
		"\n" +
		"Limitations:\n" +
		"  • execai-tools (Bash/Read/Write) do NOT work — the claude CLI runs its OWN tools.\n" +
		"  • Model selection — via `claude config set defaultModel <id>` externally.\n" +
		"  • History is passed as a plain-text prompt (no session-id).",
	"subs.connect.codexcliOK": "✓ codex-cli connected (quota from your ChatGPT Plus/Pro subscription via OpenAI Codex OAuth).\n" +
		"Switch: /source codex-cli\n" +
		"\n" +
		"Requirements:\n" +
		"  • `codex` installed in PATH (github.com/openai/codex).\n" +
		"  • `codex login` done with your ChatGPT account.\n" +
		"\n" +
		"Limitations:\n" +
		"  • execai-tools do NOT work — codex runs its OWN tools.\n" +
		"  • No streaming — codex exec returns the final text.",
	"subs.connect.ollamaHelp": "Ollama — 2 connection modes:\n" +
		"\n" +
		"🌩  CLOUD (ollama.com):\n" +
		"   Models glm-5.2, qwen3-coder-30b and more run on their servers.\n" +
		"   Anthropic-compatible endpoint. Requires an API key from https://ollama.com/settings/keys\n" +
		"   Usage:  /connect ollama <api-key>\n" +
		"\n" +
		"🏠  LOCAL (your own Ollama):\n" +
		"   Models via `ollama pull <name>`, run locally, free.\n" +
		"   OpenAI-compatible endpoint. No key.\n" +
		"   Usage:  /connect ollama local\n" +
		"           /connect ollama local http://192.168.1.10:11434  (custom URL)",
	"subs.connect.ollamaLocalOK": "✓ ollama (local) connected: %s\nModels (%d):  %s\n\nSwitch: /source ollama",
	"subs.connect.ollamaCloudOK": "✓ ollama (cloud) connected: %s\n" +
		"Models: glm-5.2, qwen3-coder-30b, kimi-k2 and more (see https://ollama.com/library)\n" +
		"\nSwitch: /source ollama\n" +
		"Change model:  /model",
	"subs.connect.unsupported": "provider %q is not supported yet. Available: zai, anthropic, openai, kimi (Kimi Code Coding Plan), kimi-api (Moonshot Platform pay-per-token), claude-cli, codex-cli, ollama",

	"subs.connect.example.zai":       "Example: /connect zai sk-zai-XXXXX",
	"subs.connect.example.anthropic": "Example: /connect anthropic sk-ant-XXXXX  (key from https://console.anthropic.com/settings/keys)",
	"subs.connect.example.kimi":      "Example: /connect kimi sk-XXXXX  (Kimi Code Coding Plan from https://www.kimi.com/code/console)",
	"subs.connect.example.kimiapi":   "Example: /connect kimi-api sk-XXXXX  (Moonshot Platform pay-per-token from https://platform.moonshot.ai/console/api-keys)",
	"subs.connect.example.openai":    "Example: /connect openai sk-proj-XXXXX  (key from https://platform.openai.com/api-keys)",
	"subs.connect.keyRequired":       "API key is required. %s",

	"subs.connect.kimiApiRejected": "✗ key rejected by api.moonshot.ai (HTTP %d).\n" +
		"You may have used a Kimi Code key instead of a Moonshot Platform one.\n\n" +
		"The keys are different:\n" +
		"  • Kimi Code (subscription):  /connect kimi <key>  — from https://www.kimi.com/code/console\n" +
		"  • Moonshot pay-per-token:    /connect kimi-api <key> — from https://platform.moonshot.ai/console/api-keys",
	"subs.connect.openaiRejected": "✗ key rejected by api.openai.com (HTTP %d).\n" +
		"Check: is the key copied in full, is it still valid (not revoked).\n" +
		"Get a key: https://platform.openai.com/api-keys",
	"subs.connect.availableModels": "\n  Available models: %s",
	"subs.connected.via":           "✓ %s connected via %s.%s\nSwitch:  /source %s",
	"subs.connected":               "✓ %s connected. Switch:  /source %s",
}

func init() { i18n.Register("en", enSubsMessages) }
