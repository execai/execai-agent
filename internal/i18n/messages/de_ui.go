// Package messages/de_ui — German strings for execai ui (the agent panel in
// a browser). Merged into the "de" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deUIMessages = map[string]string{
	"ui.cmd.short": "Agent-Panel im Browser öffnen (nur auf diesem Computer)",
	"ui.cmd.long": "Dasselbe Chat-Panel wie in VS Code und ExecAI Studio, in einem Browser-Tab auf 127.0.0.1.\n\n" +
		"Die Adresse enthält einen Einmalschlüssel: Beim ersten Aufruf wird er gegen ein Cookie getauscht, danach öffnet der Link nichts mehr. " +
		"Verbinden kann sich nur dieser Computer. Aufgaben von der Website und die Tarifprüfung funktionieren wie im Editor-Panel. Beenden: Strg+C.",
	"ui.flag.cwd":    "Projektverzeichnis (Standard: das aktuelle)",
	"ui.flag.port":   "Port auf 127.0.0.1 (Standard: ein freier)",
	"ui.flag.noOpen": "Browser nicht öffnen, nur die Adresse ausgeben",
	"ui.badCwd":      "kein Verzeichnis: %s",
	"ui.address":     "ExecAI-Panel: %s",
	"ui.hint":        "Der Link funktioniert einmal — der Browser, der ihn öffnet, behält den Zugang, bis execai ui beendet wird. Beenden: Strg+C.",
	"ui.openFailed":  "Browser konnte nicht geöffnet werden (%v) — öffne den Link selbst.",
	"ui.stopped":     "Das Panel ist beendet.",
	"ui.http.linkUsed": "Dieser Link wurde bereits verwendet. Öffne das Panel in dem Browser, in dem du es zuerst geöffnet hast, " +
		"oder starte execai ui neu, um einen neuen Link zu bekommen.",
	"ui.http.noSession": "Öffne den Link, den execai ui im Terminal ausgegeben hat.",
}

func init() { i18n.Register("de", deUIMessages) }
