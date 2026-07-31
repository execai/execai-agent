// Package messages/de_misc — deutsche Strings des Misc-Batches (tui.go,
// plain REPL, compact, agent loop, welcome).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deMiscMessages = map[string]string{
	// === Boot / login flow (tui.go) ===
	"ui.boot.noLogin": "ℹ Arbeit ohne ExecAI-Konto — Quelle: %s · Modell: %s.\n" +
		"  /login — ExecAI-Konto verbinden (unser Katalog mit ~34 Modellen + Abrechnung).",
	"ui.login.intro": "Hallo! Zum Anmelden musst du den Agenten im Browser bestätigen (wie bei gh auth login).\n" +
		"Falls der Device-Flow aus irgendeinem Grund nicht funktioniert — kannst du hier ein JWT-Token (eyJ…) einfügen und Enter drücken.\n\n" +
		"ℹ Ein ExecAI-Konto ist OPTIONAL: du kannst ohne Anmeldung mit deinem eigenen Abo arbeiten.\n" +
		"  /connect kimi <key>   → /source kimi     (Kimi Code)\n" +
		"  /connect zai <key>    → /source zai      (Z.ai GLM)\n" +
		"  /connect openai <key> → /source openai   (OpenAI API)\n" +
		"  Außerdem: anthropic, kimi-api, claude-cli, codex-cli, ollama — /connect zeigt alles.",
	"ui.login.staleToken":          "Das alte Token ist auf %s ungültig. Starte Device-Flow für eine neue Anmeldung…",
	"ui.login.startFlow":           "Starte Device-Flow für eine neue Anmeldung…",
	"ui.login.deviceFlowOpen":      "Im Browser öffnen und bestätigen:\n\n  %s\n\nCode (falls du ihn manuell eingibst): %s\n\nDrücke Enter / [y], um den Browser automatisch zu öffnen. [n], um ihn selbst zu öffnen.",
	"ui.boot.modelsFallbackFailed": "konnte die Modellliste weder abrufen noch als Fallback zusammenstellen",
	"ui.welcome":                   "execai %s · %s/%s · %s · %s\nGib eine Aufgabe ein. /model — Modelle, /help — Befehle, /quit — Beenden.",

	// === Stream errors / token expiry ===
	"ui.stream.tokenExpiredHint": "→ Wechsle /source zai|ollama|anthropic, oder /login zum erneuten Bestätigen.",
	"ui.stream.tokenExpiredFlow": "ExecAI-Token abgelaufen — starte Device-Flow. Bestätige im Browser.",

	// === /compact ===
	"ui.compact.historyNote": "[Verlauf zuvor komprimiert (%d Nachrichten): %s]",
	"ui.compact.done":        "📦 Verlauf komprimiert: %d Nachrichten → 1 Summary (~%d Zeichen)",
	"ui.compact.working":     "komprimiere den Verlauf…",
	"ui.compact.tooShort":    "der Verlauf ist noch kurz — nichts zu komprimieren (es werden >%d Nachrichten benötigt)",
	"ui.compact.truncated":   "…(gekürzt)",
	"ui.compact.promptSystem": "Du bist ein Kontext-Kompressor für einen KI-Agenten. Du bekommst das Transkript eines Gesprächs. " +
		"Gib eine KURZE Zusammenfassung (≤500 Wörter) zurück, die Folgendes bewahrt:\n" +
		"  • zentrale Entscheidungen und ihre Gründe\n" +
		"  • wichtige Dateipfade und Befehle\n" +
		"  • Ergebnisse von Tool-Calls, die später nützlich sein könnten\n" +
		"  • Fehler und wie sie gelöst wurden\n" +
		"Lass Geplauder und Bestätigungen weg. Schreibe auf Deutsch, im Telegrammstil.",
	"ui.compact.promptUser": "Komprimiere dieses Gespräch:\n\n%s",

	// === Autoloop ===
	"ui.autoloop.defaultPrompt": "mach weiter",
	"ui.autoloop.wake":          "🌙 autoloop: Aufwachen in %s (%s) → Prompt: %q",

	// === /paste ===
	"ui.paste.empty":     "Keine Einfügungen in dieser Sitzung. Ctrl+V mit einem großen Textblock → Marker.",
	"ui.paste.header":    "Einfügungen (Ctrl+V ≥200 Zeichen oder mit \\n):\n",
	"ui.paste.showHint":  "\nAnzeigen: /paste show <N>",
	"ui.paste.notNumber": "keine Zahl: %s",
	"ui.paste.notFound":  "Einfügung #%d existiert nicht",
	"ui.paste.usage":     "Verwendung: /paste [list|show <N>]",

	// === /whoami ===
	"ui.whoami.notLoggedIn": "(nicht angemeldet — /login)",

	// === /classic & /mouse ===
	"ui.classic.on":  "✓ classic TUI ON — starte execai neu (/quit → execai). Alt-Screen + fixierte Statusleiste, Shift+Ziehen zum Kopieren.",
	"ui.classic.off": "✓ Ink-Style (Standard) — starte execai neu. Verlauf im Scrollback, native Auswahl und Scrollen.",
	"ui.mouse.off":   "🖱  Mauserfassung OFF — die Maus markiert Text, das Menü reagiert nicht auf Klicks. Aktivieren: /mouse on",
	"ui.mouse.on":    "🖱  Mauserfassung ON — Rad scrollt, Klicks im Menü. Text markieren: Shift+Ziehen. Aus: /mouse off",

	// === /effort ===
	"ui.effort.pickerHint": "Effort-Picker: ←/→ wählen, Enter bestätigen, Esc abbrechen",
	"ui.effort.current":    "Effort jetzt: %s (%d Tokens)\nÄndern: /effort <off|low|medium|high|max>\n  off=0  low=1024  medium=4096  high=8192  max=32000\nFunktioniert für Anthropic-kompatible Quellen (Z.ai, Kimi, Anthropic, ollama-cloud, claude-cli).",
	"ui.effort.set":        "✓ effort=%s (%d Tokens)",

	// === /max-iterations ===
	"ui.maxIter.current": "Max iterations jetzt: %d\nLimit für Tool-Use-Iterationen pro Zug. Bei Erschöpfung — sanfter Stopp, der Nutzer kann 'mach weiter' sagen.\nÄndern: /max-iterations <N>  (empfohlen 20-200; Standard 40)",
	"ui.maxIter.usage":   "/max-iterations <N>  wobei N von 1 bis 500 geht (empfohlen 20-200)",

	// === /loop ===
	"ui.loop.status":      "🔁 loop: alle %s — %q. /loop stop zum Anhalten",
	"ui.loop.inactive":    "loop ist inaktiv. Verwendung: /loop <Intervall> <Prompt>  (Beispiel: /loop 5m prüfe den Build-Status)",
	"ui.loop.notRunning":  "loop läuft ohnehin nicht",
	"ui.loop.stopped":     "🔁 loop angehalten",
	"ui.loop.usage":       "/loop <Intervall> <Prompt>  (z. B.: /loop 5m prüfe den Build-Status)",
	"ui.loop.badInterval": "Intervall %s nicht parsbar — es braucht etwas wie 30s, 5m, 1h",
	"ui.loop.started":     "🔁 loop gestartet: alle %s — %q\nAnhalten: /loop stop",

	// === /log ===
	"ui.log.none":   "noch kein Log: %s",
	"ui.log.header": "📜 Letzte %d Anfragen (%s):\n",

	// === /usage & /cd ===
	"ui.usage.fetchFailed": "Usage konnte nicht abgerufen werden: %s",
	"ui.cd.current":        "cwd jetzt: %s",

	// === Sessions ===
	"ui.session.new":         "neues Gespräch",
	"ui.sessions.header":     "\nGespeicherte Gespräche (neueste oben):\n",
	"ui.sessions.empty":      "(leer)\n",
	"ui.sessions.switchHint": "\nWechseln: /resume <Nummer|id>",
	"ui.resume.notFound":     "Gespräch %q nicht gefunden. /sessions — Liste",
	"ui.resume.loadFailed":   "konnte nicht geladen werden: %s",
	"ui.resume.resumed":      "wir machen weiter: %s",
	"ui.title.renamed":       "umbenannt: %s",

	// === /permissions ===
	"ui.perms.toolsEmpty": "  always allowed tools: (leer)\n",
	"ui.perms.exactCount": "  always allowed exact commands: %d Einträge\n",
	"ui.perms.resetHint":  "\nZum Zurücksetzen — lösche die Datei manuell oder führe aus: rm ~/.config/execai/permissions.json",

	// === /model ===
	"ui.model.notFound": "Modell %q nicht gefunden",
	"ui.model.switched": "gewechselt zu %s/%s — %s (Verlauf bleibt erhalten)",

	// === Approve ===
	"ui.approve.denied":       "abgelehnt",
	"ui.approve.allowedTool":  "erlaubt: alle %s-Aufrufe in dieser Sitzung",
	"ui.approve.allowedExact": "erlaubt: dieser Befehl in dieser Sitzung",
	"ui.approve.navHint":      "← → oder Tab — wechseln, Enter — auswählen, Esc — ablehnen",

	// === Tool-call summaries ===
	"ui.toolSummary.write": "Write %s  (%d Bytes)",

	// === Plain REPL (chat.go) ===
	"plain.err.fetchModels":    "Modellliste konnte nicht abgerufen werden",
	"plain.err.emptyModels":    "der Server hat eine leere Modellliste zurückgegeben",
	"plain.err.pickModelEmpty": "es konnte kein Modell gewählt werden (leere Liste?)",
	"plain.err.pickModel":      "es konnte kein Modell gewählt werden",
	"plain.commands":           "Befehle: /model — Modell wählen, /clear — Verlauf löschen, /quit — Beenden.",
	"plain.historyCleared":     "(Verlauf gelöscht)",
	"plain.modelSwitchHint":    "\nWechseln: /model <Nummer> oder /model <model_name>",
	"plain.modelNotFound":      "(Modell %q nicht gefunden. /model — Liste ansehen)",
	"plain.modelSwitched":      "(gewechselt zu %s/%s — %s; Verlauf bleibt erhalten)",
	"plain.errorPrefix":        "Fehler:",
	"plain.modelsHeader":       "\nVerfügbare Modelle (★ — primary, • — current):",

	// === Agent loop (internal/agent) ===
	"loop.iterationLimit": "⚠ Limit von %d Iterationen erreicht — die Aufgabe ist nicht abgeschlossen. Sag „mach weiter“, um %d weitere zu gewähren, oder formuliere sie um.",

	// === Welcome screen (first launch) ===
	"welcome.text": `Hallo! Das ist execai — ein CLI-Agent für die Entwicklung.

Was er kann:
  • Dateien lesen/schreiben/bearbeiten (Read, Write, Edit)
  • suchen (Grep, Glob, LS, Tree)
  • Shell-Befehle ausführen (Bash) — read-only ohne Nachfrage, der Rest mit Rückfrage
  • HTTP-Anfragen stellen (WebFetch — ohne Browser; ein echter Browser kommt separat)
  • eine To-do-Liste führen (TodoWrite)

Gedächtnis:
  • ./EXECAI.md           — Projektgedächtnis (Repo-Kontext)
  • ~/.config/execai/EXECAI.md — deine persönlichen Einstellungen
Beide Dateien werden in jeder Sitzung automatisch in den System-Prompt geladen.

Befehle:
  /model               — Modellliste
  /model <num|Teilstring> — wechseln (Verlauf bleibt erhalten)
  /clear               — Verlauf löschen
  /help                — diese Nachricht
  /quit                — Beenden

Tipp: Enter — senden, Shift+Enter — Zeilenumbruch.`,

	// === WebSearch / WebFetch tools ===
	"tool.websearch.noLogin": "Die Websuche ist nicht verfügbar: Sie läuft über das ExecAI-Gateway und erfordert ein ExecAI-Konto.\n" +
		"Ohne Login bleibt der lokale Browser — öffne mit WebFetch beliebige URLs und folge den zurückgegebenen Links.\n" +
		"Mit /login schaltest du die Suche frei (und damit auch den ExecAI-Modellkatalog).",
	"tool.websearch.sources": "Quellen:",
	"tool.websearch.empty":   "Die Suche lieferte kein Ergebnis. Formuliere die Anfrage um oder öffne eine konkrete Seite mit WebFetch.",
}

func init() {
	i18n.Register("de", deMiscMessages)
}
