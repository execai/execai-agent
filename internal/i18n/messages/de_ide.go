// Package messages/de_ide — German strings the agent sends to the editor
// panel (execai ide): the permission buttons and the list of sources to
// connect.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deIDEMessages = map[string]string{
	// The descriptions under the permission buttons; the buttons themselves
	// are approve.* (shared with the terminal chat).
	"ide.approve.onceDesc":    "nur diesen Aufruf erlauben",
	"ide.approve.sessionDesc": "bis zum Ende dieses Chats",
	"ide.approve.exactDesc":   "genau diesen Aufruf bis zum Ende des Chats",
	"ide.approve.foreverDesc": "diese Erlaubnis merken — sie wird in permissions.json gespeichert",
	"ide.approve.denyDesc":    "nicht ausführen; der Agent erhält eine Ablehnung",

	"ide.connect.withKey":    "%s (Key)",
	"ide.connect.noKey":      "%s (ohne Key)",
	"ide.connect.zaiAPI":     "Z.ai-Plattform, Pay-per-Token (Key)",
	"ide.connect.openrouter": "OpenRouter — alle Anbieter mit einem Key (Key)",
	"ide.connect.ollama":     "Ollama — lokal oder Cloud (Key für die Cloud)",
	"ide.connect.custom":     "Eigener Anbieter — jede OpenAI- oder Anthropic-kompatible Adresse",

	// Notices and errors the panel shows (execai ide), the security levels
	// and the errors of the sign-in and connect paths from other packages.
	"ide.err.effort":            "effort: off|low|medium|high|max oder eine Zahl",
	"ide.err.positive":          "eine positive Zahl ist nötig",
	"ide.maxIter.set":           "Tool-Iterationen pro Zug: %d",
	"ide.login.startFailed":     "die Anmeldung ließ sich nicht starten",
	"ide.login.saveFailed":      "das Token ließ sich nicht speichern",
	"ide.login.expired":         "der Bestätigungscode ist abgelaufen — versuch es noch einmal",
	"ide.login.timeout":         "die Anmeldung wurde nicht rechtzeitig bestätigt",
	"ide.connect.keyRejectedBy": "%s hat den Key abgelehnt (HTTP %d)",
	"ide.connect.noModels":      "verbunden: %s (%s) — der Server lieferte keine Modellliste (HTTP %d); neu verbinden und die Modell-IDs angeben",
	"ide.connect.ollamaKey":     "ollama.com hat den Key nicht angenommen (HTTP %d)",
	"ide.connect.ollamaDown":    "Ollama unter %s hat nicht geantwortet (HTTP %d) — läuft sie?",
	"ide.connect.unknown":       "unbekannte Quelle: %s",
	"ide.connect.needKey":       "der Key des Anbieters ist nötig",
	"ide.connect.keyRejected":   "Key abgelehnt (HTTP %d) — prüfe, dass es ein Key von %s ist",
	"ide.connect.done":          "verbunden: %s",
	"ide.connect.models":        " — verfügbare Modelle: %d",
	"ide.connect.unverified":    " (Key gespeichert; er wird bei der ersten Anfrage geprüft)",
	"ide.turn.queued":           "Zug in der Warteschlange: andere Chats antworten noch",
	"ide.err.chdir":             "kann nicht in den Ordner %s wechseln",
	"ide.err.noModel":           "kein Modell auswählbar — prüfe /model in execai",
	"ide.err.noModelShort":      "kein Modell",
	"ide.err.badMessage":        "Nachricht nicht lesbar: %s",
	"ide.err.noChat":            "kein solcher Chat: %s",
	"ide.subs.unreadable":       "subscriptions.json ist nicht lesbar (%s) — es läuft die Standardquelle; die Abos sind nicht verloren, aber die Datei muss repariert werden",
	"ide.err.noSuchModel":       "die aktuelle Quelle hat kein solches Modell: %s",
	"ide.err.unknownLevel":      "unbekannte Stufe: %s",
	"ide.security.set":          "Sicherheitsstufe: %s",
	"ide.logout.done":           "von ExecAI abgemeldet (die Abos bleiben)",
	"ide.disconnect.done":       "getrennt: %s",
	"ide.resume.none":           "nichts fortzusetzen — das ist der erste Chat im Projekt",
	"ide.resume.failed":         "der letzte Chat lässt sich nicht öffnen: %s",
	"ide.chat.openFailed":       "der Chat lässt sich nicht öffnen: %s",
	"ide.wake.none":             "es war kein Aufwecken geplant",
	"ide.err.unknownCommand":    "unbekannter Befehl: %s",
	"ide.err.unknownType":       "unbekannter Typ: %s",
	"ide.chat.busy":             "dieser Chat antwortet noch — warte das Ende des Zugs ab oder stoppe ihn",
	"security.level.light":      "leicht — nicht stören",
	"security.level.deep":       "normal — im Projekt still, außerhalb mit Nachfrage",
	"security.level.paranoid":   "streng — alles nachfragen",
	"connect.err.api":           "Schnittstellentyp %q wird nicht unterstützt: openai (chat/completions) oder anthropic (messages)",
	"connect.err.noURL":         "eine Serveradresse ist nötig, z. B. http://localhost:1234/v1",
	"connect.err.badURL":        "Adresse %q sieht nicht wie eine http(s)-URL aus",
	"subs.err.nameLatin":        "der Name braucht lateinische Buchstaben oder Ziffern (daraus entsteht die ID für /source)",
	"subs.err.nameLong":         "der Name ist länger als 40 Zeichen",
	"subs.err.builtin":          "%q ist eine eingebaute Quelle — wähle einen anderen Namen",
	"subs.err.notLoaded":        "die Abos sind nicht geladen",
	"subs.err.notConnected":     "das Abo %q ist nicht verbunden — zuerst /connect %s",
	"auth.err.linkStart":        "agent-link/start lieferte %d: %s",
	"auth.err.linkPoll":         "agent-link/poll lieferte %d: %s",
	"auth.err.pollFailed":       "Polling ist 10-mal hintereinander gescheitert",
	"auth.err.linkExpired":      "Link abgelaufen — /login erneut ausführen",
}

func init() { i18n.Register("de", deIDEMessages) }
