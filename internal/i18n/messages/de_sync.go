// Package messages/de_sync — German strings for sync between computers
// (/sync, execai sync). Merged into the "de" catalog via i18n.Register.
package messages

import "github.com/velesbsdllc/agent-vbai/internal/i18n"

var deSyncMessages = map[string]string{
	"hint.sync":        "Memory und Einstellungen zwischen deinen Computern synchronisieren",
	"sync.usage":       "/sync — Status · /sync on · off · now · password · forget · keys export · keys import",
	"sync.placeholder": "Antwort — Enter, Abbrechen — Esc",
	"sync.working":     "Einen Moment…",
	"sync.cancelled":   "Abgebrochen.",
	"sync.error":       "Sync: Fehler: %v",
	"sync.failed":      "Sync-Server nicht erreichbar: %v",

	// === gate ===
	"sync.gate.noPlan": "Sync gibt es in den Tarifen «Work» und «Max»: %s",
	"sync.gate.login":  "Für Sync bei ExecAI anmelden: execai starten und im Browser bestätigen (im Chat: /login).",

	// === status ===
	"sync.status.off":         "Sync ist aus. Einschalten: %[1]s on — Memory, Einstellungen und Provider-Schlüssel werden auf allen deinen Computern gleich (Tarife «Work» und «Max»).",
	"sync.status.on":          "Sync ist an · letzter Abgleich: %s",
	"sync.status.onNever":     "Sync ist an · noch kein Abgleich",
	"sync.status.what":        "Synchronisiert: %s — alles auf diesem Computer verschlüsselt.",
	"sync.status.pausedPlan":  "Pausiert: Sync gibt es in den Tarifen «Work» und «Max»: %s",
	"sync.status.pausedLogin": "Pausiert: bei ExecAI anmelden (im Chat: /login).",
	"sync.status.pausedKey": "Pausiert: der Sync-Schlüssel wurde auf einem anderen Computer geändert. " +
		"Sync-Passwort erneut eingeben: %s on",
	"sync.status.unreadable": "Dateien auf dem Server, die sich hier nicht öffnen: %d — sie sind mit einem anderen Schlüssel verschlüsselt. " +
		"Sync-Passwort eingeben: %s on",
	"sync.status.tooBig":    "Zu groß, bleiben nur hier: %s",
	"sync.status.quota":     "Der Sync-Speicher auf dem Server ist voll — neue Dateien werden nicht gesendet.",
	"sync.status.lastError": "Letzter Fehler: %s",
	"sync.status.commands":  "Befehle: %[1]s now · %[1]s off · %[1]s password · %[1]s forget · %[1]s keys export | import",

	"sync.what.memory":   "Memory (Dateien: %d)",
	"sync.what.projects": "Projekte (%d)",

	// === on ===
	"sync.on.newIntro": "Das ist dein erster Computer mit Sync. Memory, Einstellungen und Provider-Schlüssel werden hier verschlüsselt, auf dem Computer: der Server hat nur die verschlüsselte Kopie, und wir sehen nicht, was drin ist.",
	"sync.on.newPassword": "Denk dir ein Sync-Passwort aus (mindestens %d Zeichen). Du gibst es auf deinen " +
		"anderen Computern ein. Es lässt sich nicht wiederherstellen.",
	"sync.on.repeatPassword": "Passwort wiederholen:",
	"sync.on.joinIntro":      "Sync ist bereits auf einem anderen deiner Computer an.",
	"sync.on.enterPassword":  "Sync-Passwort vom ersten Computer eingeben:",
	"sync.on.keyDiffers": "Dieser Computer hat schon einen eigenen Schlüssel, und er weicht vom Schlüssel auf dem Server ab. " +
		"Mit einem Schlüssel verschlüsseltes Memory lässt sich mit dem anderen nicht lesen.",
	"sync.on.chooseKey": "1 — Schlüssel vom Server nehmen (deiner wird daneben aufbewahrt)\n" +
		"2 — deinen behalten und den auf dem Server ersetzen: alles auf dem Server wird von hier überschrieben, " +
		"deine anderen Computer fragen erneut nach dem Passwort",
	"sync.on.oldKeySaved": "Der bisherige Schlüssel ist aufbewahrt: %s",
	"sync.on.raced":       "Inzwischen wurde der Schlüssel auf dem Server von einem anderen Computer geändert. Noch einmal: %s on",
	"sync.on.racedJoin":   "Auf einem anderen Computer wurde gerade ein Sync-Passwort gesetzt — gib es ein.",
	"sync.on.created":     "Sync ist an.",
	"sync.on.joined":      "Schlüssel erhalten, Sync ist an.",
	"sync.on.replaced":    "Sync ist an; auf dem Server liegen jetzt die Daten dieses Computers.",
	"sync.on.done":        "Sync ist an.",
	"sync.on.firstFailed": "Der erste Abgleich ist fehlgeschlagen: %v. Neuer Versuch in 10 Minuten, oder jetzt: %s now",

	// === password ===
	"sync.password.wrong":    "Das Passwort passt nicht.",
	"sync.password.short":    "Zu kurz: mindestens %d Zeichen.",
	"sync.password.mismatch": "Die Passwörter stimmen nicht überein, noch einmal.",
	"sync.password.new":      "Neues Sync-Passwort (mindestens %d Zeichen, nicht wiederherstellbar):",
	"sync.password.changed": "Passwort geändert. Auf den anderen Computern ist nichts zu tun — " +
		"das Passwort braucht man nur, um einen neuen hinzuzufügen.",

	// === now / off ===
	"sync.now.done":    "Fertig: %d empfangen, %d gesendet.",
	"sync.now.off":     "Sync ist aus. Einschalten: %s on",
	"sync.off.already": "Sync ist bereits aus. Einschalten: %s on",
	"sync.off.done": "Sync ist auf diesem Computer aus. Die Daten bleiben auf dem Server; " +
		"löschen: %[1]s forget. Wieder einschalten: %[1]s on",

	// === forget ===
	"sync.forget.warn":      "Vom Server werden der Verschlüsselungsschlüssel, alles synchronisierte Memory, die Einstellungen und Provider-Schlüssel sowie die Computerliste gelöscht. Auf deinen Computern bleibt alles. Das lässt sich nicht rückgängig machen.",
	"sync.forget.confirm":   "Zum Löschen das Wort «%s» eingeben:",
	"sync.forget.word":      "löschen",
	"sync.forget.cancelled": "Nicht gelöscht.",
	"sync.forget.done": "Vom Server gelöscht, Dateien: %d. Auf deinen Computern ist alles noch da; " +
		"die anderen schalten Sync bei der nächsten Prüfung aus. Wieder einschalten: %s on",

	// === background ===
	"sync.bg.paused":  "Sync pausiert — Memory und Einstellungen auf diesem Computer bleiben unverändert.",
	"sync.bg.resumed": "Sync läuft wieder.",
	"sync.bg.wiped": "Sync-Daten wurden vom Server gelöscht (von einem anderen Computer) — hier ist Sync aus. " +
		"Wieder einschalten: %s on",
	"sync.bg.keyChanged": "Sync pausiert: der Schlüssel wurde auf einem anderen Computer geändert. " +
		"Sync-Passwort erneut eingeben: %s on",

	// === keys ===
	"sync.keys.nothing":  "Nichts zu übertragen: auf diesem Computer ist kein Provider-Schlüssel verbunden.",
	"sync.keys.exported": "Provider-Schlüssel: %s.\nÜbertragung ohne Server und ohne Tarif. Hier der Übertragungscode — auf dem anderen Computer einfügen (%[3]s keys import):\n\n%[2]s",
	"sync.keys.phrase": "Die Phrase zum Code — getrennt vom Code und auf anderem Weg senden:\n\n    %s\n\n" +
		"Code und Phrase zusammen öffnen deine Schlüssel: niemandem zeigen.",
	"sync.keys.importIntro": "Provider-Schlüssel von einem anderen Computer übernehmen (dort: keys export).",
	"sync.keys.enterCode":   "Übertragungscode einfügen:",
	"sync.keys.enterPhrase": "Phrase aus %d Wörtern eingeben:",
	"sync.keys.badCode":     "Das sieht nicht nach einem Übertragungscode aus — vollständig kopieren.",
	"sync.keys.badPhrase":   "Die Phrase passt nicht.",
	"sync.keys.added":       "Hinzugefügt: %s",
	"sync.keys.updated":     "Aktualisiert (der Schlüssel dort ist neuer): %s",
	"sync.keys.kept":        "Unverändert: %s",
	"sync.keys.active":      "Die aktive Quelle bleibt: %s. Wechseln: /source <Name>",

	// === execai sync ===
	"sync.cmd.short":            "Memory und Einstellungen zwischen deinen Computern synchronisieren",
	"sync.cmd.long":             "Memory, Einstellungen, Berechtigungen und Provider-Schlüssel des Agenten sind auf allen deinen Computern gleich. Alles wird auf dem Computer mit dem Sync-Passwort verschlüsselt; der Server hat nur die verschlüsselte Kopie, und wir sehen nicht, was drin ist.\nOhne Tarif lassen sich Provider-Schlüssel direkt übertragen: execai sync keys export / import.\n\nSync gibt es in den Tarifen «Work» und «Max».",
	"sync.cmd.status":           "Sync-Status (Standard)",
	"sync.cmd.on":               "Sync auf diesem Computer einschalten",
	"sync.cmd.off":              "Sync auf diesem Computer ausschalten (Daten bleiben auf dem Server)",
	"sync.cmd.now":              "Jetzt synchronisieren",
	"sync.cmd.password":         "Sync-Passwort ändern",
	"sync.cmd.forget":           "Alles vom Server löschen",
	"sync.cmd.keys":             "Provider-Schlüssel ohne Server zwischen Computern übertragen",
	"sync.cmd.keysExport":       "Übertragungscode und Phrase anzeigen",
	"sync.cmd.keysImport":       "Schlüssel von einem anderen Computer per Code und Phrase übernehmen",
	"sync.cmd.needTerminal":     "Dafür ist ein Terminal nötig: die Antworten werden von Hand eingegeben.",
	"sync.what.config":          "Einstellungen und Provider-Schlüssel",
	"sync.status.keysFallback":  "Ohne Sync lassen sich Provider-Schlüssel direkt übertragen — ohne Server und ohne Tarif: %s keys export",
	"sync.pulledSource":         "Die aktive Quelle wurde auf einem anderen Computer gewechselt: %s",
	"sync.status.projects":      "Projekte: %d (%s)",
	"sync.project.off":          "Projektdateien (.execai/) werden synchronisiert, wenn Sync an ist: %s on",
	"sync.project.pulled":       "Projektdateien (.execai/) vom Server empfangen.",
	"sync.project.pushed":       "Projektdateien (.execai/) an den Server gesendet — verschlüsselt.",
	"sync.project.same":         "Projektdateien (.execai/) stimmen mit dem Server überein.",
	"sync.project.empty":        "Das Projekt hat noch keine .execai/-Dateien — sie werden synchronisiert, sobald es sie gibt.",
	"sync.project.backup":       "Das Projekt wurde hier und auf einem anderen Computer geändert — die neuere Kopie gewinnt, die andere liegt daneben: %s",
	"project.published":         "Projekt «%s» im Web-Chat erstellt.",
	"project.publishedNotBound": "Projekt «%s» im Web-Chat erstellt, aber dieses Verzeichnis ließ sich nicht verbinden: %v",
	"hint.projectPublish":       "aus diesem Verzeichnis ein Projekt im Web-Chat machen",
	"project.cmd.short":         "Projekte: Verzeichnis auf dem Computer ↔ Projekt im Web-Chat",
	"project.cmd.publish":       "Aus dem aktuellen Verzeichnis ein Web-Chat-Projekt machen, verbinden und .execai/ senden",
}

func init() { i18n.Register("de", deSyncMessages) }
