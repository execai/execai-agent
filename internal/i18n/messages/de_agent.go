// Package messages/de_agent — German strings of the agent loop and
// the model catalog that the person sees: a declined tool, the scope of a
// permission, working without the catalog. Merged into the "de" catalog
// via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deAgentMessages = map[string]string{
	"agent.tool.declined":          "Der Nutzer hat die Ausführung dieses Werkzeugs abgelehnt.",
	"agent.tool.noApprovalChannel": "execai kann für diesen Aufruf hier keine Bestätigung einholen",
	"agent.approve.scope":          "die Erlaubnis gilt für: %s",
	"llm.offline.fallback":         "⚠ Offline — der Modellkatalog ist nicht erreichbar, ich nutze die eingebaute Liste. Grund: %v",
	"llm.offline.cache":            "ℹ Ich nutze den zwischengespeicherten Modellkatalog (Alter: %s). Netzwerk nicht erreichbar: %v",
	"llm.models.unauthorized":      "401 von /billing-vbai/models_public — die Anmeldung ist abgelaufen, melde dich erneut an (im Chat: /login)",
	"llm.models.status":            "der Modellkatalog antwortete %d: %s",
	"llm.models.notJSON":           "die Antwort des Modellkatalogs ist keine Liste: %s",
	"llm.models.none":              "weder Netzwerk noch ein gespeicherter Modellkatalog verfügbar",
	"llm.err.unauthorized":         "401 von %s — %s",
	"llm.err.status":               "der Server antwortete %d: %s",
	"llm.err.server":               "der Server meldete einen Fehler: %v",
	"llm.err.aichatUnauthorized":   "401 von /aichat-vbai/stream — das Token ist abgelaufen, führe execai login aus",
	"llm.err.sse":                  "Serverfehler im SSE-Stream: %v",
	"llm.err.anthropicKey":         "401 von der Anthropic-API — falscher Schlüssel. Prüfe /subscriptions",
	"llm.err.providerKey":          "401 von %s — falscher API-Schlüssel. Prüfe /subscriptions und /connect",
	"llm.err.providerStatus":       "%s antwortete %d: %s",
	"llm.billing.limit":            "⏱  LIMIT ERREICHT",
	"llm.billing.funds":            "💰 GUTHABEN REICHT NICHT",
	"llm.billing.inactive":         "⚠  TARIF INAKTIV",
	"llm.billing.unavailable":      "Der Dienst ist aus Abrechnungsgründen vorübergehend nicht verfügbar.",
	"llm.billing.hint":             "Tarif wechseln: Bereich „Tarif“ in deinem ExecAI-Webkonto (%s)",
	"llm.cli.claudeMissing":        "claude CLI nicht gefunden (PATH, ~/.local/bin, Erweiterungen von VS Code/Cursor). Installation: https://docs.claude.com/en/docs/claude-code/quickstart",
	"llm.cli.claudeNotFound":       "claude CLI nicht gefunden",
	"llm.cli.claudeNoModels":       "claude hat kein Modell geliefert",
	"llm.cli.codexMissing":         "codex CLI nicht im PATH gefunden. Installation: https://github.com/openai/codex\nNach der Installation: codex login",
	"llm.cli.codexNotFound":        "codex CLI nicht im PATH gefunden",
	"llm.cli.codexNoModels":        "codex hat kein Modell geliefert",
	"llm.cli.noAnswer":             "die CLI hat sich ohne Antwort beendet",
	"llm.cli.emptyResult":          "leeres Ergebnis",
	"llm.cli.emptyAnswer":          "leere Antwort",
	"llm.cli.images":               "Bilder",
	"llm.cli.noToolResult":         "kein Ergebnis: claude wurde vorher beendet",
	"llm.ollama.down":              "Ollama %s ist nicht erreichbar",
	"llm.ollama.start":             "Starte es: ollama serve",
	"llm.ollama.noModel":           "Ollama: Modell %q ist nicht installiert (ollama pull %s?). %s",
	"llm.ollama.status":            "Ollama antwortete %d: %s",
	"llm.ollama.tagsStatus":        "Ollama /api/tags antwortete %d: %s",
	"llm.ollama.tagsParse":         "Ollama /api/tags: Antwort nicht lesbar",
	"llm.ollama.noModels":          "In Ollama gibt es keine Modelle. Installiere eins: `ollama pull llama3.2`",
	"llm.ollama.localModel":        "lokales Ollama-Modell",
	"agent.memory.readFailed":      "Memory lesen",
	"agent.memory.exportDirFailed": "Exportordner anlegen",
	"agent.memory.writeFailed":     "%s schreiben",
	"agent.permissions.saveFailed": "Erlaubnis nicht gespeichert:",
	"auth.notSignedIn":             "du bist nicht angemeldet. Führe aus: execai login",
}

func init() { i18n.Register("de", deAgentMessages) }
