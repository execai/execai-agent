// Package messages/de_subs — deutsche Strings für die Abo-/Quellen-Befehle
// (/connect, /disconnect, /source, /subscriptions).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deSubsMessages = map[string]string{
	// === /disconnect ===
	"subs.noSubs":              "keine Abos vorhanden",
	"subs.disconnect.notFound": "Abo %q nicht gefunden",
	"subs.disconnect.none":     "Keine verbundenen Abos.",
	"subs.disconnect.header":   "/disconnect <provider>. Verbunden:\n",
	"subs.saveError":           "Fehler beim Speichern: %s",
	"subs.disconnected":        "✓ %s getrennt",

	// === /source (Liste) ===
	"subs.useDeprecated":     "ℹ /use — veraltet, nutze /source\n\n",
	"subs.source.listHeader": "Verfügbare Quellen (/source <name>):\n",
	"subs.source.listExecai": "  • execai  — unser Billing (Standard)\n",
	"subs.source.listItem":   "  • %-8s — Abo %s\n",
	"subs.source.listFooter": "\nTipp: tippe '/source ' und drücke Tab für das Menü.",

	// === /source <name> Hilfe bei Fehler ===
	"subs.howto.zai": "\n\nSo verbindest du Z.ai:\n" +
		"  1. Hol dir den Key auf https://z.ai/manage-apikey/apikey-list\n" +
		"     (Bereich Individual Coding Plan → Plan Overview)\n" +
		"  2. /connect zai sk-zai-XXXXXX\n" +
		"  3. /source zai",
	"subs.howto.anthropic": "\n\nSo verbindest du Anthropic:\n" +
		"  1. Hol dir einen API-Key auf https://console.anthropic.com/settings/keys\n" +
		"     (Format sk-ant-... Pay-per-Token-Billing)\n" +
		"  2. /connect anthropic sk-ant-XXXXXX\n" +
		"  3. /source anthropic",
	"subs.sourceSwitched": "✓ Quelle: %s · Modell: %s",

	// === /connect (ohne Argumente) ===
	"subs.connect.usageShort": "Verwendung: /connect <provider> <api_key>\n" +
		"Unterstützt: zai (Z.ai Coding Plan)\n" +
		"Tipp: '/connect ' + Tab zeigt das Provider-Menü.",

	// === /subscriptions ===
	"subs.list.empty": "Keine verbundenen Abos.\n" +
		"Verbinden: /connect zai (Z.ai Coding Plan)\n" +
		"Aktuelle Quelle: ExecAI (unser Billing)",
	"subs.list.header":         "Verbundene Abos (aktiv: %s):\n",
	"subs.list.switchHint":     "\nWechseln: /source <provider>  (oder /source execai → unser Billing)\n",
	"subs.list.disconnectHint": "Trennen:  /disconnect <provider>",

	// === Provider-Beschreibungen (/source-Menü) ===
	"subs.provider.execai":    "unser Billing (Standard)",
	"subs.provider.zai":       "Z.ai GLM Coding Plan",
	"subs.provider.kimi":      "Kimi Code Coding Plan (K3/K2.7, Abo kimi.com/code)",
	"subs.provider.kimiapi":   "Moonshot Platform (Pay-per-Token, platform.moonshot.ai)",
	"subs.provider.anthropic": "Anthropic API (sk-ant-…)",
	"subs.provider.openai":    "OpenAI Platform (sk-… von platform.openai.com, Pay-per-Token)",
	"subs.provider.codexcli":  "lokales OpenAI Codex CLI (ChatGPT-Plus/Pro-Kontingent)",
	"subs.provider.claudecli": "lokales Claude Code (Kontingent des Pro/Max-Abos)",
	"subs.provider.ollama":    "lokaler Ollama-Runner (localhost:11434)",

	// === Menü-Hints ===
	"subs.hint.connected":     "verbunden",
	"subs.hint.connectedPlan": "verbunden · %s",
	"subs.hint.notConnected":  "nicht verbunden — /connect %s",
	"subs.hint.remove":        "Abo entfernen",

	// === /connect-Menü-Hints ===
	"subs.connectHint.zai":       "Z.ai GLM Coding Plan (Coding-Plan-API-Key)",
	"subs.connectHint.kimi":      "Kimi Code Coding Plan (K3/K2.7, Key von kimi.com/code/console)",
	"subs.connectHint.kimiapi":   "Moonshot Platform Pay-per-Token (Key von platform.moonshot.ai)",
	"subs.connectHint.anthropic": "Claude API (sk-ant-... von console.anthropic.com)",
	"subs.connectHint.openai":    "OpenAI Platform Pay-per-Token (sk-… von platform.openai.com)",
	"subs.connectHint.codexcli":  "lokales OpenAI Codex CLI (ohne Key, `codex` muss installiert sein)",
	"subs.connectHint.claudecli": "lokales Claude Code (ohne Key, `claude` muss installiert sein)",
	"subs.connectHint.ollama":    "lokaler Ollama-Runner (localhost:11434, ohne Key)",

	// === /connect-Ablauf ===
	"subs.connect.usage": "Verwendung: /connect <provider> <api_key> [base_url]\n" +
		"Unterstützt: zai (Z.ai Coding Plan)\n" +
		"Beispiel:  /connect zai sk-zai-XXXXX\n" +
		"           /connect zai sk-zai-XXXXX https://open.bigmodel.cn/api/paas/v4  (für CN)",
	"subs.connect.claudecliOK": "✓ claude-cli verbunden (Kontingent aus deinem Pro/Max-Abo via Claude-Code-OAuth).\n" +
		"Wechsle mit: /source claude-cli\n" +
		"\n" +
		"Einschränkungen:\n" +
		"  • execai-tools (Bash/Read/Write) funktionieren NICHT — das claude CLI nutzt seine EIGENEN Tools.\n" +
		"  • Modellwahl — extern über `claude config set defaultModel <id>`.\n" +
		"  • Der Verlauf wird als Plain-Text-Prompt übergeben (ohne session-id).",
	"subs.connect.codexcliOK": "✓ codex-cli verbunden (Kontingent aus deinem ChatGPT-Plus/Pro-Abo via OpenAI-Codex-OAuth).\n" +
		"Wechsle mit: /source codex-cli\n" +
		"\n" +
		"Voraussetzungen:\n" +
		"  • `codex` im PATH installiert (github.com/openai/codex).\n" +
		"  • `codex login` mit deinem ChatGPT-Konto ausgeführt.\n" +
		"\n" +
		"Einschränkungen:\n" +
		"  • execai-tools funktionieren NICHT — codex nutzt seine EIGENEN Tools.\n" +
		"  • Kein Streaming — codex exec liefert den fertigen Text.",
	"subs.connect.ollamaHelp": "Ollama — 2 Verbindungsmodi:\n" +
		"\n" +
		"🌩  CLOUD (ollama.com):\n" +
		"   Modelle glm-5.2, qwen3-coder-30b u. a. laufen auf deren Servern.\n" +
		"   Anthropic-kompatibler Endpoint. API-Key nötig von https://ollama.com/settings/keys\n" +
		"   Verwendung:  /connect ollama <api-key>\n" +
		"\n" +
		"🏠  LOCAL (eigenes Ollama):\n" +
		"   Modelle via `ollama pull <name>`, laufen lokal, kostenlos.\n" +
		"   OpenAI-kompatibler Endpoint. Ohne Key.\n" +
		"   Verwendung:  /connect ollama local\n" +
		"                /connect ollama local http://192.168.1.10:11434  (eigene URL)",
	"subs.connect.ollamaLocalOK": "✓ ollama (local) verbunden: %s\nModelle (%d):  %s\n\nWechsle mit: /source ollama",
	"subs.connect.ollamaCloudOK": "✓ ollama (cloud) verbunden: %s\n" +
		"Modelle: glm-5.2, qwen3-coder-30b, kimi-k2 u. a. (siehe https://ollama.com/library)\n" +
		"\nWechsle mit: /source ollama\n" +
		"Modell wechseln:  /model",
	"subs.connect.unsupported": "Provider %q wird noch nicht unterstützt. Verfügbar: zai, anthropic, openai, kimi (Kimi Code Coding Plan), kimi-api (Moonshot Platform Pay-per-Token), claude-cli, codex-cli, ollama",

	"subs.connect.example.zai":       "Beispiel: /connect zai sk-zai-XXXXX",
	"subs.connect.example.anthropic": "Beispiel: /connect anthropic sk-ant-XXXXX  (Key von https://console.anthropic.com/settings/keys)",
	"subs.connect.example.kimi":      "Beispiel: /connect kimi sk-XXXXX  (Kimi Code Coding Plan von https://www.kimi.com/code/console)",
	"subs.connect.example.kimiapi":   "Beispiel: /connect kimi-api sk-XXXXX  (Moonshot Platform Pay-per-Token von https://platform.moonshot.ai/console/api-keys)",
	"subs.connect.example.openai":    "Beispiel: /connect openai sk-proj-XXXXX  (Key von https://platform.openai.com/api-keys)",
	"subs.connect.keyRequired":       "API-Key ist erforderlich. %s",

	"subs.connect.kimiApiRejected": "✗ Key von api.moonshot.ai abgelehnt (HTTP %d).\n" +
		"Vermutlich hast du einen Kimi-Code-Key statt eines Moonshot-Platform-Keys angegeben.\n\n" +
		"Die Keys sind unterschiedlich:\n" +
		"  • Kimi Code (Abo):          /connect kimi <key>  — von https://www.kimi.com/code/console\n" +
		"  • Moonshot Pay-per-Token:   /connect kimi-api <key> — von https://platform.moonshot.ai/console/api-keys",
	"subs.connect.openaiRejected": "✗ Key von api.openai.com abgelehnt (HTTP %d).\n" +
		"Prüfe: Key vollständig kopiert? Noch gültig (nicht widerrufen)?\n" +
		"Key holen: https://platform.openai.com/api-keys",
	"subs.connect.availableModels": "\n  Verfügbare Modelle: %s",
	"subs.connected.via":           "✓ %s verbunden über %s.%s\nWechsle mit:  /source %s",
	"subs.connected":               "✓ %s verbunden. Wechsle mit:  /source %s",

	// === Z.ai open platform (zai-api) + ToS-предупреждение по Coding Plan ===
	"subs.provider.zaiapi":        "Z.ai open platform (Pay-per-Token, api.z.ai)",
	"subs.connectHint.zaiapi":     "Z.ai Pay-per-Token (Schlüssel von z.ai/manage-apikey) — ohne Tool-Beschränkung",
	"subs.connect.example.zaiapi": "Beispiel: /connect zai-api XXXXX  (Z.ai-open-platform-Schlüssel von https://z.ai/manage-apikey/apikey-list)",
	"subs.connect.zaiApiRejected": "✗ api.z.ai hat die Anfrage abgelehnt (HTTP %d).\n" +
		"Z.ai vergibt EINEN Schlüssel für Coding Plan und offene Plattform,\n" +
		"es ist also fast sicher kein falscher Schlüssel, sondern ein Guthaben von\n" +
		"null: das Abo wird getrennt von der offenen Plattform abgerechnet.\n" +
		"\n" +
		"Aufladen unter https://z.ai/manage-apikey/apikey-list, oder beim Abo\n" +
		"über /connect zai bleiben (siehe Hinweis zu dessen Bedingungen).",
	"subs.connect.zaiToSWarning": "⚠ Zu den Bedingungen des GLM Coding Plan.\n" +
		"Z.ai beschränkt das Abo auf eine geschlossene Liste von Tools (Claude Code, Cursor,\n" +
		"Cline, Roo Code, OpenCode, Pi, Crush, Goose und einige mehr) — execai steht nicht darauf.\n" +
		"Ihre Richtlinie erlaubt Rate-Limiting, das Einfrieren des Kontos und eine Sperre nach\n" +
		"drei Verstößen. Es ist DEIN Konto, nicht unseres — entscheide selbst.\n" +
		"\n" +
		"Alternative ohne Beschränkung — Z.ai open platform, Abrechnung pro Token:\n" +
		"  /connect zai-api <key>   — dieselben GLM-Modelle, keine Tool-Liste.",
	"subs.connect.zaiKeyReused": "der Coding-Plan-Schlüssel wird wiederverwendet, es ist derselbe",
}

func init() { i18n.Register("de", deSubsMessages) }
