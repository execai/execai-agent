// Package messages/de_ui — German strings for execai ui (the agent panel in
// a browser). Merged into the "de" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deUIMessages = map[string]string{
	"ui.cmd.short": "Agent-Panel in einem eigenen Fenster oder im Browser öffnen (nur auf diesem Computer)",
	"ui.cmd.long": "Dasselbe Chat-Panel wie in VS Code und ExecAI Studio, auf 127.0.0.1.\n\n" +
		"Es öffnet sich als eigenes Fenster — ohne Tabs und Adressleiste —, wenn Google Chrome, Chromium, Microsoft Edge, " +
		"Yandex Browser, Brave oder Vivaldi installiert ist, sonst als Tab des Standardbrowsers. Das Schließen des Fensters beendet das Panel, Strg+C ebenso. " +
		"Ein zweiter Start im selben Ordner zeigt das bereits geöffnete Fenster.\n\n" +
		"Die Adresse enthält einen Einmalschlüssel: Beim ersten Aufruf wird er gegen ein Cookie getauscht, danach öffnet der Link nichts mehr. " +
		"Verbinden kann sich nur dieser Computer. Aufgaben von der Website und die Tarifprüfung funktionieren wie im Editor-Panel.",
	"ui.flag.cwd":             "Projektverzeichnis (Standard: das aktuelle)",
	"ui.flag.port":            "Port auf 127.0.0.1 (Standard: der eigene des Ordners, einer von 23750–23769; ist er belegt, der nächste)",
	"ui.flag.noOpen":          "Browser nicht öffnen, nur die Adresse ausgeben",
	"ui.flag.window":          "nur ein eigenes Fenster: ohne passenden Browser ein Fehler",
	"ui.flag.browser":         "als Tab des Standardbrowsers öffnen, ohne eigenes Fenster",
	"ui.flag.last":            "der Ordner des letzten Starts (so startet die Verknüpfung)",
	"ui.flag.code":            "den Einmalcode ausgeben, mit dem sich ein Fenster am laufenden Panel dieses Ordners anmeldet",
	"ui.flag.installShortcut": "eine Startverknüpfung anlegen (mit --cwd: für diesen Ordner)",
	"ui.flag.removeShortcut":  "die Startverknüpfung entfernen",
	"ui.flags.exclusive":      "--window, --browser und --no-open passen nicht zusammen: wähle eines",
	"ui.flags.oneAction":      "--install-shortcut, --remove-shortcut, --code und --stop passen nicht zusammen: wähle eines",
	"ui.badCwd":               "kein Verzeichnis: %s",
	"ui.address":              "ExecAI-Panel: %s",
	"ui.hint":                 "Der Link funktioniert einmal — der Browser, der ihn öffnet, behält den Zugang, bis execai ui beendet wird. Beenden: Strg+C.",
	"ui.openFailed":           "Browser konnte nicht geöffnet werden (%v) — öffne den Link selbst.",
	"ui.stopped":              "Das Panel ist beendet.",
	"ui.window.opened":        "Ich öffne das ExecAI-Panel in einem eigenen Fenster (%s). Seine Adresse: %s",
	"ui.window.hint":          "Schließe das Fenster oder drücke Strg+C, um das Panel zu beenden.",
	"ui.window.tabFallback":   "Ein eigenes Fenster braucht einen dieser Browser: %s — keiner ist hier installiert, ich öffne einen Tab.",
	"ui.window.none":          "kein Browser für ein eigenes Fenster; geeignet sind: %s",
	"ui.window.noScreen":      "keine grafische Sitzung (DISPLAY und WAYLAND_DISPLAY sind leer) — ein Fenster lässt sich nirgends öffnen",
	"ui.window.notHere":       "Der in den Einstellungen genannte Browser (%s) ist auf diesem Computer nicht vorhanden — ich wähle selbst.",
	"ui.window.startFailed":   "Das Fenster ließ sich in %s nicht öffnen (%v) — ich öffne einen Tab.",
	"ui.window.silent":        "Das Fenster in %s hat das Panel nicht gezeigt. Anmelden kannst du dich mit einem Code (execai ui --code in diesem Ordner) oder mit dem Link:",
	"ui.window.failed":        "das Fenster ließ sich in %s nicht öffnen: %v",
	"ui.last.none":            "kein Projektordner gewählt: Starte execai ui im Projektordner oder lege eine Verknüpfung dafür an — execai ui --install-shortcut --cwd ORDNER",
	"ui.last.noneTitle":       "ExecAI: kein Projektordner gewählt",
	"ui.last.noneNotice":      "Starte execai ui im Projektordner — danach öffnet die Verknüpfung ihn.",
	"ui.shortcut.noLast":      "Die Verknüpfung öffnet den Ordner des letzten Starts. Den gibt es noch nicht: Starte zuerst execai ui im Projektordner oder lege mit --cwd ORDNER eine Verknüpfung für einen Ordner an.",
	"ui.window.closed":        "Das Fenster ist geschlossen — ich beende das Panel.",
	"ui.alarm.usedTwice":      "Mit dem Einmal-Link des Panels kamen zwei: Jemand anderes hat ihn auch. Alle Anmeldungen sind aufgehoben, laufende Durchgänge gestoppt und wartende Berechtigungsanfragen abgelehnt: Die andere Seite könnte dem Agenten eine Nachricht geschickt haben — sieh nach, was die Chats zuletzt getan haben.",
	"ui.alarm.noPage":         "Der Einmal-Link des Panels wurde benutzt, aber danach hat sich keine Panel-Seite verbunden. Alle Anmeldungen sind aufgehoben, laufende Durchgänge gestoppt und wartende Berechtigungsanfragen abgelehnt: Wer hereinkam, könnte dem Agenten eine Nachricht geschickt haben — sieh nach, was die Chats zuletzt getan haben.",
	"ui.alarm.reopen":         "Ich öffne das Fenster neu, mit einem neuen Schlüssel.",
	"ui.alarm.useCode":        "Neu anmelden: Führe in diesem Ordner execai ui --code aus und gib den Code auf der Seite des Panels ein.",
	"ui.already.window":       "Das Panel dieses Ordners ist bereits geöffnet — ich zeige sein Fenster (%s).",
	"ui.already.tab":          "Das Panel dieses Ordners ist bereits geöffnet: %s",
	"ui.already.starting":     "Das Panel dieses Ordners startet bereits.",
	"ui.code.line":            "Einmalcode für die Seite %s: %s (gilt %d Min.)",
	"ui.code.notRunning":      "execai ui läuft für diesen Ordner nicht",
	"ui.flag.stop":            "das Panel beenden, das für diesen Ordner läuft",
	"ui.flag.all":             "zusammen mit --stop: die Panels aller Ordner beenden",
	"ui.flags.allNeedsStop":   "--all gibt es nur zusammen mit --stop",
	"ui.stop.done":            "Das Panel ist beendet: %s",
	"ui.stop.notRunning":      "Für diesen Ordner läuft kein Panel.",
	"ui.stop.noneRunning":     "Es läuft kein Panel.",
	"ui.stop.failed":          "das Panel wurde nicht beendet: Es hält den Ordner %s noch",
	"ui.stop.asked":           "Beendet auf Anforderung von execai ui --stop.",
	"ui.noScreen":             "Keine grafische Sitzung (DISPLAY und WAYLAND_DISPLAY sind leer) — ich starte keinen Browser: Öffne den Link selbst.",
	"ui.shortcut.notOurs":     "unter %s liegt keine Verknüpfung, die execai ui --install-shortcut angelegt hat — ich lasse sie unberührt",
	"ui.shortcut.noHome":      "kein Benutzerordner: Die Verknüpfung lässt sich nirgends ablegen",
	"ui.shortcut.unsupported": "auf diesem System gibt es keine Startverknüpfung",
	"ui.shortcut.comment":     "Das Agent-Panel von ExecAI",
	"ui.shortcut.installed":   "Startverknüpfung angelegt: %s",
	"ui.shortcut.fallback":    "Die Verknüpfung ließ sich nicht anlegen — stattdessen eine Startdatei: %s",
	"ui.shortcut.removed":     "Startverknüpfung entfernt: %s",
	"ui.shortcut.absent":      "Keine Startverknüpfung: %s",
	"ui.shortcut.tabOnly":     "Ein eigenes Fenster braucht einen dieser Browser: %s. Ohne einen solchen öffnet die Verknüpfung einen Tab, und das Panel läuft, bis es beendet wird: execai ui --stop --all.",
	"ui.http.linkUsed": "Dieser Link wurde bereits verwendet. Öffne das Panel in dem Browser, in dem du es zuerst geöffnet hast, " +
		"oder starte execai ui neu, um einen neuen Link zu bekommen.",
	"ui.http.noSession":    "Öffne den Link, den execai ui im Terminal ausgegeben hat.",
	"ui.http.login.code":   "Führe im Terminal, im Projektordner, execai ui --code aus und gib hier den Code ein, den der Befehl ausgibt.",
	"ui.http.login.link":   "Oder öffne den Link, den execai ui im Terminal ausgegeben hat.",
	"ui.http.login.label":  "Einmalcode",
	"ui.http.login.button": "Anmelden",
	"ui.http.login.wrong":  "Der Code passt nicht. Wurde er schon benutzt oder ist er älter als fünf Minuten, führe execai ui --code noch einmal aus.",
	"ui.http.login.down":   "Das Panel antwortet nicht — prüfe, ob execai ui läuft.",
}

func init() { i18n.Register("de", deUIMessages) }
