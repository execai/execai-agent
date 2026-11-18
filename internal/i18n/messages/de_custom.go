// Package messages/de_custom — German strings for live model lists and the
// custom provider (/connect custom). Merged into the "de" catalog.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deCustomMessages = map[string]string{
	"catalog.cliDefault":               "Standard",
	"catalog.cliDefaultDesc":           "ohne --model: das Modell aus den Einstellungen des CLI",
	"subs.connect.modelsCount":         "Vom Anbieter gemeldete Modelle: %d — Auswahl mit /model.",
	"subs.connect.noModels":            "Der Anbieter hat keine Modelle gemeldet. Gib selbst eins an: /model <id> — es wird für diese Quelle gespeichert.",
	"subs.connect.ollamaLocalFailed":   "✗ Ollama unter %s antwortet nicht (HTTP %d). Läuft sie? `ollama serve`",
	"subs.connect.ollamaCloudRejected": "✗ ollama.com hat den Key abgelehnt (HTTP %d). Keys: https://ollama.com/settings/keys",
	"ui.model.listEmpty":               "Diese Quelle hat noch keine Modelle gemeldet. Gib eins an: /model <id>",
	"ui.model.typed":                   "Modell %q steht nicht in der Liste des Anbieters — wird wie eingegeben verwendet und für diese Quelle gespeichert.",
	"hint.connect.custom":              "eigener Anbieter: jede OpenAI- oder Anthropic-kompatible Adresse",
	"subs.connectHint.custom":          "eigener Anbieter: Protokoll, Adresse, Key — Schritt für Schritt",
	"subs.hint.custom":                 "eigen (%s) · %s",
	"subs.custom.usage": "Verwendung:\n" +
		"  /connect custom                                — Schritt für Schritt\n" +
		"  /connect custom <Name> <openai|anthropic> <base_url> [api_key] [--models a,b] [--header \"Name: Wert\"]\n" +
		"Beispiele:\n" +
		"  /connect custom \"LM Studio\" openai http://localhost:1234/v1\n" +
		"  /connect custom deepseek openai https://api.deepseek.com sk-xxx\n" +
		"  /connect custom corp anthropic https://llm.corp.local sk-xxx --header \"X-Tenant: dev\"",

	"custom.wiz.intro":              "Eigenen Anbieter verbinden — jeder Server mit OpenAI- oder Anthropic-API. Esc bricht ab.",
	"custom.wiz.name":               "Schritt 1/4. Wie soll er heißen? Zum Beispiel: LM Studio, vLLM, deepseek, corp-gateway (lateinische Buchstaben — der Name wird zur ID für /source).",
	"custom.wiz.idIs":               "ID: %s  (später wechseln mit /source <id>)",
	"custom.wiz.api":                "Schritt 2/4. Schnittstellentyp:\n  1 — OpenAI-kompatibel (POST …/chat/completions): LM Studio, vLLM, llama.cpp, LiteLLM, DeepSeek, Groq, Together, Mistral…\n  2 — Anthropic-kompatibel (POST …/v1/messages): LiteLLM, Claude-Gateways, MiniMax, DeepSeek…\nGib 1 oder 2 ein.",
	"custom.wiz.urlOpenAI":          "Schritt 3/4. Serveradresse (Base-URL), z. B. http://localhost:1234/v1, http://gpu-box:8000/v1, https://api.deepseek.com",
	"custom.wiz.urlAnthropic":       "Schritt 3/4. Serveradresse (Base-URL) ohne /v1/messages, z. B. https://llm.corp.local, https://api.deepseek.com/anthropic",
	"custom.wiz.key":                "Schritt 4/4. API-Key — Enter, wenn keiner nötig ist (lokale Server haben meist keinen). Wird nur auf diesem Rechner gespeichert und nie in den Eingabeverlauf geschrieben.",
	"custom.wiz.noKey":              "(kein Key)",
	"custom.wiz.keyRejected":        "✗ Der Server hat den Key abgelehnt (HTTP %d). Noch einmal eingeben oder Esc zum Abbrechen.",
	"custom.wiz.models":             "Der Server hat keine Modellliste geliefert (GET …/models: HTTP %d). Gib Modell-IDs durch Kommas getrennt ein oder Enter, um später /model <id> zu nutzen.",
	"custom.wiz.cancelled":          "Verbinden eines eigenen Anbieters abgebrochen.",
	"custom.wiz.placeholder.name":   "Name des Anbieters, z. B. LM Studio",
	"custom.wiz.placeholder.api":    "1 — OpenAI-kompatibel, 2 — Anthropic-kompatibel",
	"custom.wiz.placeholder.url":    "Base-URL, z. B. http://localhost:1234/v1",
	"custom.wiz.placeholder.key":    "API-Key oder Enter für keinen",
	"custom.wiz.placeholder.models": "Modell-IDs mit Kommas oder Enter",
	"custom.keyRejected":            "✗ %[2]s hat den Key abgelehnt (HTTP %[1]d).",
	"custom.connected":              "✓ Verbunden: %s (ID %s, Protokoll %s, %s)",
	"custom.connectedModels":        "Modelle: %d, aktuell: %s — Auswahl mit /model.",
	"custom.connectedHint":          "Später zurück: /source %[1]s · entfernen: /disconnect %[1]s",
}

func init() { i18n.Register("de", deCustomMessages) }
