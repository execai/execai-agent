// Package messages/de_machine — deutsche Texte für die Einführung (/start),
// die Maschinenkarte (/machine) und den Link fürs Handy (/phone).
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deMachineMessages = map[string]string{
	"hint.start":        "Erste Schritte: fünf Schritte und was schon erledigt ist",
	"hint.machine":      "meine Maschine: Modelle, Sync, von wo sie erreichbar ist",
	"hint.machineCheck": "prüfen, ob die Modellquellen antworten",
	"hint.phone":        "QR-Code: diese Maschine vom Handy aus rufen",

	// === /start ===
	"start.title":    "Erste Schritte",
	"start.progress": "%d von %d erledigt",
	"start.allDone":  "Alles bereit: diese Maschine ist vollständig eingerichtet.",
	"start.footer": "Meine Maschine: /machine · alle Befehle: /help · diese Liste wieder: /start\n" +
		"Enter — senden, Shift+Enter — neue Zeile.",

	"start.login.title": "Bei ExecAI anmelden",
	"start.login.why":   "Das Konto verbindet diese Maschine mit dem Web-Chat, deinem Handy und deinen anderen Computern.",
	"start.login.cmd":   "Eingeben: /login — dann im Browser bestätigen.",

	"start.models.title": "Modelle verbinden",
	"start.models.why":   "Eigene Provider-Schlüssel oder ein ExecAI-Tarif — Memory und Aufrufe vom Handy funktionieren mit beidem.",
	"start.models.cmd":   "Eingeben: /connect <Provider> — eigener Schlüssel, /plan — ExecAI-Tarif, /machine check — prüfen, ob die Quelle antwortet.",

	"start.sync.title":    "Sync einschalten",
	"start.sync.why":      "Dieselben Einstellungen und dasselbe Memory auf all deinen Computern.",
	"start.sync.what":     "Wird übertragen, auf diesem Computer mit deinem Sync-Passwort verschlüsselt: die Konfiguration mit den Provider-Schlüsseln, dein persönliches Memory, die Dateien verbundener Projekte.",
	"start.sync.notWhat":  "Bleibt hier: api_base (die Serveradresse dieser Maschine) und die Anmeldung.",
	"start.sync.password": "Nur du kennst das Sync-Passwort: wir speichern es nicht und können es nicht wiederherstellen.",
	"start.sync.cmd":      "Eingeben: /sync on (Tarife «Work» und «Max»).",

	"start.project.title": "Einen Ordner mit einem Web-Projekt verbinden",
	"start.project.why":   "Der Ordner wird zum Projekt im Web-Chat: Aufgaben aus dem Web laufen darin, das Projekt-Memory ist gemeinsam.",
	"start.project.cmd":   "Eingeben: /project publish — Projekt aus diesem Ordner machen, /project bind <Name> — mit einem bestehenden verbinden.",

	"start.phone.title": "Den Agenten vom Handy aus rufen",
	"start.phone.why":   "Gib dieser Maschine Aufgaben aus dem Web-Chat oder vom Handy — auch wenn sie mit deinen eigenen Schlüsseln arbeitet.",
	"start.phone.cmd":   "Eingeben: /phone — ein QR-Code fürs Handy. Damit Aufgaben ankommen, execai serve starten.",

	// === /machine ===
	"machine.title":          "Meine Maschine",
	"machine.label.machine":  "Maschine",
	"machine.label.account":  "Konto",
	"machine.label.models":   "Modelle",
	"machine.label.sync":     "Sync",
	"machine.label.presence": "Erreichbar",
	"machine.identity":       "%s · %s · Agent %s",
	"machine.account.in":     "%s · Tarif: %s",
	"machine.account.noPlan": "%s · kein ExecAI-Tarif",
	"machine.account.out":    "nicht angemeldet — /login",

	"machine.source.alive":     "antwortet (geprüft %s)",
	"machine.source.down":      "hat nicht geantwortet (geprüft %s)",
	"machine.source.unchecked": "noch nicht geprüft — /machine check",
	"machine.source.needLogin": "Anmeldung nötig — /login",
	"machine.source.noPlan":    "kein aktiver Tarif — /plan",
	"machine.source.active":    "in Benutzung",

	"machine.sync.off":      "aus — /sync on",
	"machine.sync.on":       "an · letzter Upload: %s",
	"machine.sync.never":    "an · noch nichts hochgeladen",
	"machine.sync.paused":   "pausiert — Details: /sync",
	"machine.sync.inside":   "enthalten: %s",
	"machine.sync.config":   "Konfiguration und Provider-Schlüssel",
	"machine.sync.memory":   "persönliches Memory (Dateien: %d)",
	"machine.sync.projects": "Projekte: %d",
	"machine.sync.empty":    "noch nichts",
	"machine.sync.mode":     "Modus: %s",
	"machine.mode.all":      "alle Projekte",
	"machine.mode.selected": "nur verbundene Ordner",
	"machine.mode.none":     "keine Projekte",

	"machine.presence.serve":      "nimmt Aufgaben an: ja — execai serve (pid %d)",
	"machine.presence.ide":        "nimmt Aufgaben an: ja — ein Editorfenster (pid %d)",
	"machine.presence.noListener": "nimmt Aufgaben an: nein — execai serve starten",
	"machine.presence.reachable":  "vom Web und vom Handy rufen: ja — /phone",
	"machine.presence.blocked":    "vom Web und vom Handy rufen: noch nicht — %s",
	"machine.block.login":         "anmelden (/login)",
	"machine.block.plan":          "braucht den Tarif «Work» oder «Max»",
	"machine.block.listener":      "execai serve starten",

	"machine.footer":   "Quellen erneut prüfen: /machine check · Einführung: /start",
	"machine.checking": "Prüfe die Quellen…",

	// === /phone ===
	"phone.title":     "Diese Maschine vom Handy aus rufen",
	"phone.scan":      "Richte die Handykamera auf den Code oder öffne den Link:",
	"phone.what":      "Die Seite öffnet den Web-Chat, in dem diese Maschine schon für Aufgaben gewählt ist.",
	"phone.needLogin": "Der Link fürs Handy erscheint nach der Anmeldung: /login",
	"phone.listener":  "Damit diese Maschine Aufgaben annimmt, muss execai serve laufen (in einem eigenen Fenster).",
	"phone.plan":      "Aufgaben aus dem Web und vom Handy gibt es in den Tarifen «Work» und «Max»: %s",
	"phone.qrFailed":  "Der QR-Code ließ sich nicht zeichnen: %v",
}

func init() { i18n.Register("de", deMachineMessages) }
