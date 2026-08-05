[English](README.md) | [Русский](README.ru.md) | [Español](README.es.md) | **Deutsch** | [中文](README.zh.md)

🌐 **Website:** [execai.ru](https://execai.ru) · 💬 Web-Chat: [chat.execai.ru](https://chat.execai.ru)

---

# execai — ein Terminal-KI-Agent

execai ist ein Terminal-first KI-Coding-Agent, der mit deinen Lieblings-LLM-Anbietern zusammenarbeitet (Claude, GPT-5, Kimi K3, GLM-5.2 und mehr) — bring dein eigenes Abo mit oder nutze unseres. Ein CLI-Agent im Geist von Claude Code, der wirklich auf deiner Maschine läuft: er liest Dateien, führt Befehle aus, redet mit Kubernetes und antwortet dir im Chat. Er unterstützt **9 Sources** — unser ExecAI-Backend + deine Abonnements bei Z.ai / Kimi Code / Anthropic / OpenAI + lokale Claude Code / OpenAI Codex CLI / Ollama (Cloud oder lokal). Du kannst spontan zwischen ihnen wechseln, mit gemeinsam genutztem Conversation-History.

```
execai R5.136 · openai/gpt-5 · it@execai.ru · ~
Type a task. /model — models, /help — commands, /quit — exit.

› check free disk space on server dev01
```

![demo](scripts/demo.gif)

> **ℹ Ein ExecAI-Konto ist optional.** Die Standard-Source `execai` nutzt unser Backend (Registrierung auf [execai.ru](https://execai.ru)), aber die CLI funktioniert vollständig **ohne ExecAI-Login**: verbinde dein eigenes Abo direkt vom Login-Screen — `/connect kimi <key>`, `/connect zai <key>`, `/connect openai <key>`, `/connect anthropic <key>` oder lokal `claude-cli` / `codex-cli` / `ollama` — dann `/source <provider>` und loslegen.

**Ink-style rendering** (Standard seit R5.107): der Message-History wird in den Terminal-Scrollback committet (native Textauswahl funktioniert und wird bei Updates nicht zurückgesetzt). Live-Streaming und Eingabe leben in einem dynamischen Bereich am unteren Rand. Genau wie Claude Code.

---

## Inhalt

1. [Installation](#installation)
2. [Erster Start](#erster-start)
3. [Mit dem Agent sprechen](#mit-dem-agent-sprechen)
4. [Sources — ExecAI und Abonnements](#sources--execai-und-abonnements)
5. [Modelle](#modelle)
6. [Bilder und Dateien](#bilder-und-dateien)
7. [Effort — Reasoning-Level](#effort--reasoning-level)
8. [Loop und Autoloop — wiederholen und warten](#loop-und-autoloop--wiederholen-und-warten)
9. [Agent-Memory (EXECAI/MEMORY.md)](#agent-memory)
10. [Projekte und Hintergrundmodus — Web → Agent](#projekte-und-hintergrundmodus--web--agent)
11. [Ausgaben / `/usage`](#ausgaben--usage)
12. [Befehle und Hotkeys](#befehle-und-hotkeys)
13. [Sessions und Verlauf](#sessions-und-verlauf)
14. [Wo die Dateien liegen](#wo-die-dateien-liegen)
15. [Troubleshooting](#troubleshooting)

---

## Installation

### Linux / macOS (Intel und Apple Silicon)

```bash
curl -fsSL https://raw.githubusercontent.com/execai/execai-agent/main/install.sh | bash
```

Die Binaries kommen standardmäßig von **GitHub Releases**. In Russland/GUS ist der Yandex-Mirror meist deutlich schneller — das Skript wählt ihn für `ru`-Locales automatisch, oder erzwinge ihn mit `MIRROR=yandex`.

Das Skript wird:
- deine Architektur erkennen (`linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`),
- die SHA256 verifizieren,
- die Binary nach `/usr/local/bin/execai` (mit passwortlosem sudo) oder `~/.local/bin/execai` legen,
- veraltete execai-Kopien aus dem PATH und tote/`/tmp/execai*`-Einträge aus deinen rc-Dateien entfernen,
- unter macOS automatisch `com.apple.quarantine` entfernen (Gatekeeper).

Nach der Installation — in einem **neuen Terminal** einfach `execai` eintippen. Im aktuellen Terminal — falls du `execai` schon hattest und bash den alten Pfad gecacht hat, hilft `hash -r`.

### Windows 10/11 (amd64 und ARM64)

```powershell
iwr -useb https://storage.yandexcloud.net/execai-agent-prod/execai/stable/install.ps1 | iex
```

Erkennt die Architektur automatisch (amd64 / arm64 für Copilot+ PCs), fragt einmalig nach **UAC**, um den Ordner zu Defenders Ausnahmen hinzuzufügen (sonst kann das Antivirus die Binary fressen), lädt `execai.exe` nach `%LOCALAPPDATA%\execai\` herunter und registriert es im User-PATH.

> Starte es aus **Windows Terminal** (nicht dem alten `cmd.exe`/conhost) — die TUI (bubbletea) rendert dort korrekt.

Nach der Installation prüfen:
```bash
execai version
# execai R5.NN
```

### Eine bestimmte Version pinnen

```bash
curl -fsSL https://raw.githubusercontent.com/execai/execai-agent/main/install.sh | VERSION=5.136 bash
```

### Manueller Download

Wenn du das Binary lieber selbst holst statt den Installer laufen zu lassen — alle Releases mit Checksums liegen auf GitHub:

**https://github.com/execai/execai-agent/releases/latest**

Wähle das Archiv für deine Plattform (`execai-{linux,darwin,windows}-{amd64,arm64}.{tar.gz,zip}`), prüfe gegen `SHA256SUMS`, entpacke es und lege es in den PATH.

---

## Erster Start

```bash
execai
```

Ohne Argumente öffnet sich die Chat-TUI.

**Falls du noch nicht eingeloggt bist** — der Agent zeigt dir einen Bestätigungslink für den Browser und öffnet ihn automatisch:

```
👉 Open in your browser and confirm this is your agent:
   https://chat.execai.ru/agents/connect/U7XQ9F4P

(waiting, polling every 3s… once you confirm, we continue)
```

Im Browser:
1. Falls du nicht bei ExecAI eingeloggt bist — melde dich an (Yandex / VK / E-Mail).
2. Auf der Seite "Connect agent" ist das Alias-Feld mit deinem Hostname vorausgefüllt. Übernimm es oder überschreib es (z. B. "yz-laptop"). Klick auf "Confirm".

Der Agent bekommt ein JWT und macht sofort weiter. Das Token lebt 90 Tage und erneuert sich bei jedem Connect automatisch.

### Was ein persistenter Agent ist

Jeder Agent ist an eine **stabile host-id** auf deiner Maschine gebunden:
- Linux — `/etc/machine-id`
- macOS — `IOPlatformUUID`
- Windows — `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid`
- Fallback — `~/.config/execai/host_id` (UUID, beim ersten Install generiert)

Das heißt: wenn du execai neu installierst, `credentials.json` löschst und dich erneut einloggst — **verwendet das Backend die bestehende Session weiter**, die agent_id bleibt gleich. Keine Duplikate in deiner Geräteliste.

Du kannst alle deine Agenten anzeigen und einzelne entkoppeln unter `chat.execai.ru/settings/agents` (im UserMenu unten links → "Agents", Terminal-Icon).

---

## Mit dem Agent sprechen

Beschreib deine Aufgabe einfach in normaler Sprache:

```
› check how much space /var/log takes on server dev01
```

Der Agent wird:
- rausfinden, was du brauchst,
- entscheiden, welches Tool er nutzt (Bash, Read, Grep, …),
- dir den Befehl vor der Ausführung zeigen und um Bestätigung bitten, falls er gefährlich ist,
- ihn ausführen, das Ergebnis lesen und antworten.

### Gefährliche Befehle bestätigen

Read-only-Sachen (`ls`, `cat`, `git status`, `kubectl get`, `grep`) laufen automatisch. Befehle, die den Zustand ändern (`rm`, `>`, `chmod`, `kubectl delete`, `git push`), fragen nach:

```
⚠  Confirm Bash execution
   kubectl delete pod foo-xxx
   
   [y] Once  [a] All Bash this session  [s] This command this session  [f] FOREVER  [n] Deny
```

| Taste | Bedeutung |
|---|---|
| **y** | EINMAL erlauben |
| **a** | ALLE Bash-Aufrufe erlauben, bis execai neu startet |
| **s** | Genau diesen Befehl (mitsamt Argumenten) bis zum Neustart erlauben |
| **f** | FÜR IMMER erlauben (persistiert in `~/.config/execai/permissions.json`) |
| **n** oder Esc | Ablehnen — der Agent bekommt "user refused" zurück |

Navigation: ← → oder Tab zum Fokus wechseln, Enter zum Bestätigen, oder die Shortcuts `y/a/s/f/n`.

### Eingebaute Tools

| Tool | Was es macht |
|---|---|
| **Bash** | Führt einen Shell-Befehl mit Streaming-Ausgabe aus (Zeilen erscheinen, sobald sie produziert werden) |
| **Read** | Liest eine Datei mit offset/limit, erkennt Binärdateien automatisch |
| **Write** | Erstellt/überschreibt eine Datei (mit Bestätigung) |
| **Edit** | Präzises String-Replacement in einer Datei (mit Bestätigung) |
| **Grep** | Regexp-Suche über einen Datei-Baum |
| **Glob** | Dateien nach Pattern wie `**/*.go` finden |
| **LS** | Verzeichnis-Listing |
| **Tree** | Datei-Baum bis Tiefe N |
| **WebFetch** | Öffnet eine Seite: HTML → lesbarer Text + Links zum Weitergehen (kein JS) |
| **WebSearch** | Websuche mit Antwort und Quellenangaben (ExecAI-Konto nötig) |
| **AskUser** | Stellt eine Frage mit 2–4 Optionen, wenn die Entscheidung bei dir liegt |
| **Task** | Übergibt eine abgeschlossene Recherche an einen Nur-Lese-Subagenten |
| **TodoWrite** | Interner Aufgaben-Planner des Agenten |
| **schedule_wakeup** | Die KI plant ihr eigenes Aufwachen in N Sekunden (siehe [Autoloop](#loop-und-autoloop--wiederholen-und-warten)) |

---

## Sources — ExecAI und Abonnements

Der Agent kann mit **9 verschiedenen Sources** reden. Der Conversation-History wird geteilt — spontan wechseln, ohne Kontext zu verlieren.

| Source | Was das ist | Abrechnung |
|---|---|---|
| **execai** (Standard) | Unser Gateway → eines von ~34 Modellen | ExecAI-Plan |
| **zai** | Z.ai GLM Coding Plan direkt | Dein Z.ai-Abo |
| **zai-api** | Z.ai open platform, Pay-per-Token — **derselbe Schlüssel wie beim Abo**, aber ohne die geschlossene Tool-Liste des Coding Plan | Pay-per-Token Z.ai |
| **kimi** | Kimi Code Coding Plan (kimi.com/code) | Dein Kimi-Code-Abo |
| **kimi-api** | Moonshot Platform pay-per-token | Pay-per-token Moonshot |
| **anthropic** | Direkte Claude-API (sk-ant-...) | Pay-per-token Anthropic |
| **openai** | Direkte OpenAI-API (sk-proj-...) | Pay-per-token OpenAI |
| **claude-cli** | Delegiert an dein lokales `claude`-CLI | Dein Claude Pro/Max/Team OAuth |
| **codex-cli** | Delegiert an dein lokales `codex`-CLI | Dein ChatGPT Plus/Pro/Team OAuth |
| **ollama** | Cloud (ollama.com) oder lokal (`ollama serve`) | Dein Abo / 0 ₽ lokal |

### 1. ExecAI — Standard

Nach dem Login nutzt der Agent unsere Abrechnung. Kein zusätzliches Setup.

```
src : ExecAI   provider : anthropic   model : claude-sonnet-4-6
```

### 2. Z.ai Coding Plan

Dein eigenes Abo für $3-60/Monat → du greifst direkt auf GLM-5.2 zu, wir stellen nichts in Rechnung.

1. Hol dir einen **Coding-Plan**-Key (nicht den normalen API-Key!):
   - https://z.ai/manage-apikey/apikey-list → **Individual Coding Plan > Plan Overview**
   - Team: https://z.ai/manage-apikey/coding-plan/team/my-plan

   > ⚠️ Ein normaler API-Key wird pay-per-token abgerechnet, NICHT aus deinem Abo.

2. `/connect zai sk-zai-XXXXX`
3. `/source zai` → primäres Modell **GLM-5.2**

### 3. Kimi Code Coding Plan

Ein separates Produkt von Moonshot AI — ihr Claude-Code-Äquivalent mit eigenem Abo. Ihr Flaggschiff **K3** ist in jedem Plan verfügbar außer den untersten Stufen.

1. Hol dir einen Key unter https://www.kimi.com/code/console (Bereich API keys).
2. ```
   /connect kimi sk-XXXXX
   /source kimi
   ```
3. Dein Plan wird automatisch über `/coding/v1/models` erkannt — die Statuszeile zeigt `kimi (K3)`, `kimi (K3 + HighSpeed)` oder `kimi (K2.7 Code)`, je nachdem, was tatsächlich verfügbar ist.

Modelle: `k3` (primär), `kimi-for-coding` (K2.7 Code), `kimi-for-coding-highspeed`. `/usage` zeigt dein **echtes Kontingent**: das Wochenlimit plus rolling windows (5h) mit Fortschrittsbalken.

### 4. Moonshot Platform (pay-per-token API)

Ein normaler API-Key für Per-Token-Abrechnung (kein Abo). Wenn du Moonshot-Modelle außerhalb von Kimi Code nutzen willst:

1. Key unter https://platform.moonshot.ai/console/api-keys holen.
2. ```
   /connect kimi-api sk-XXXXX
   /source kimi-api
   ```

Der Modell-Katalog wird dynamisch aus dem `/v1/models` deines Accounts gezogen. Primär → `kimi-latest` (Auto-Alias auf das aktuelle Flagship). Der Key wird beim `/connect` validiert — bei 401/403 gibt es sofort eine Ablehnung mit einem Hinweis, welcher Key wo hingehört.

### 5. Anthropic API

Direkter Key von https://console.anthropic.com/settings/keys — pay-per-token.

```
/connect anthropic sk-ant-XXXXX
/source anthropic
```
Modelle: claude-sonnet-4-6, claude-opus-4-8, claude-haiku-4-5.

### 6. OpenAI Platform (pay-per-token API)

Direkter Key von https://platform.openai.com/api-keys — pay-per-token.

```
/connect openai sk-proj-XXXXX
/source openai
```

Der Katalog wird aus dem `/v1/models` deines Accounts gezogen. Primär → **gpt-5** → o3 → gpt-4.1 → gpt-4o (Prioritätsreihenfolge).

### 7. Claude Code CLI (Claude Pro/Max/Team OAuth)

Wenn du bereits `claude` (Claude Code) installiert hast und ein **Pro/Max/Team**-Abo besitzt — nutze dessen Kontingent über die lokale OAuth-Session, ohne separaten Key.

```
/connect claude-cli    # prüft, ob `claude` im PATH ist
/source claude-cli
```

Modelle (Aliase): sonnet / opus / haiku. Die Modellauswahl wird zusätzlich extern über `claude config set defaultModel <id>` verwaltet.

⚠️ **Einschränkungen:**
- execai-Tools (Bash/Read/Write) funktionieren NICHT über claude-cli — es startet seine eigenen Tools mit eigenen Berechtigungen
- Der History wird als Plaintext-Prompt übergeben (keine session-id)

### 8. OpenAI Codex CLI (ChatGPT Plus/Pro/Team OAuth)

Das `claude-cli`-Äquivalent für OpenAI. Nutzt dein ChatGPT-Abo-Kontingent (Plus/Pro/Team/Enterprise) über die lokale `codex`-Binary.

**codex installieren** (Linux/macOS):
```bash
curl -fsSL https://chatgpt.com/codex/install.sh | sh
codex login    # OAuth im Browser mit deinem ChatGPT-Account
```
Windows:
```powershell
powershell -ExecutionPolicy ByPass -c "irm https://chatgpt.com/codex/install.ps1 | iex"
```

**In execai:**
```
/connect codex-cli
/source codex-cli
```

Modelle: gpt-5, o3, o4-mini. Gleiche Einschränkungen wie bei claude-cli — execai-Tools funktionieren nicht, codex startet seine eigenen.

### 9. Ollama — Cloud oder lokal

Zwei Modi unter einem Befehl:

**Cloud (ollama.com):**
```
/connect ollama <api-key>          # Key von https://ollama.com/settings/keys
/source ollama
```
Modelle auf ihren Servern: **glm-5.2** (primär), qwen3-coder:480b, kimi-k2:1t, deepseek-v3.1:671b, gpt-oss:120b. Anthropic-kompatibler Endpoint, Effort wird unterstützt.

**Lokal (dein eigenes Ollama):**
```
ollama pull llama3.2               # extern, was auch immer du laufen lassen willst
/connect ollama local              # localhost:11434
/connect ollama local http://192.168.1.10:11434   # deine eigene URL
/source ollama
```
Der Katalog wird dynamisch über `/api/tags` gezogen. 0 ₽. OpenAI-kompatibler Endpoint.

### Zum Wechseln

- **Der History wird geteilt.** `/source zai` → `/source execai` → `/source ollama` — der Kontext bleibt erhalten.
- **Die Abrechnung ist isoliert.** Auf einem externen Abo stellt dir die ExecAI-Abrechnung NICHTS in Rechnung.
- **`/subscriptions`** — was verbunden ist, `/disconnect <provider>` — entfernen.
- **`/source` ohne Argument** — Quick-Picker.

---

## Modelle

`/model` ohne Argument — Liste der verfügbaren. `/model <id>` oder `/model <num>` — wechseln.

Autovervollständigung ist bequemer:
```
/model<Tab>   →  Menü aller Modelle mit Filter
/model deeps  →  Filter nach Teilstring
```

### Was im Katalog steht (nach Source)

| Source | Modelle |
|---|---|
| **execai** | ~34 Modelle — Claude/GPT/DeepSeek/Kimi/MiniMax/Qwen/GLM über unser Backend |
| **zai** | glm-5.2 (primär), glm-5.2[1m], glm-4.7 |
| **kimi** | k3 (primär), kimi-for-coding (K2.7), kimi-for-coding-highspeed |
| **kimi-api** | kimi-latest (primär), kimi-k2-turbo-preview, moonshot-v1-* (dynamisch aus /v1/models) |
| **anthropic** | claude-sonnet-4-6, claude-opus-4-8, claude-haiku-4-5 |
| **openai** | gpt-5 (primär), o3, gpt-4.1, gpt-4o, o4-mini (dynamisch aus /v1/models) |
| **claude-cli** | sonnet / opus / haiku (Aliase + gepinnte Versionen) |
| **codex-cli** | gpt-5, o3, o4-mini |
| **ollama cloud** | glm-5.2, qwen3-coder:480b, kimi-k2:1t, deepseek-v3.1:671b, gpt-oss:120b |
| **ollama local** | was auch immer du per `ollama pull` installiert hast (dynamisch aus /api/tags) |

Das primäre Modell wechselt automatisch, wenn du die Source änderst. Preise und Beschreibungen leben in `/model`.

---

## Bilder und Dateien

### Bilder

Zieh einfach ein PNG/JPG/GIF/WEBP per **Drag & Drop** in dein Terminal-Fenster oder **tippe den Pfad in Anführungszeichen**:

```
› '/home/user/Pictures/Screenshot.png' what's in this image?
```

Der Agent wird:
- den Pfad in deiner Nachricht finden (Leerzeichen und Nicht-ASCII-Zeichen im Dateinamen sind okay, wenn die Datei gequoted ist),
- ihn base64-encoden,
- ihn als `image_url` an ein Vision-Modell senden,
- eine Beschreibung zurückbekommen.

**Vision-Modelle** (die sehen können): claude-sonnet-4-6, gpt-5.5, glm-5v-turbo, glm-4.5v. Wenn du gerade ein Nicht-Vision-Modell ausgewählt hast — per `/model` umschalten.

### Dateien (Text/Code)

Sag einfach den Pfad — der Agent liest sie über das `Read`-Tool:

```
› take a look at internal/chat/tui.go and suggest a refactor
```

Große Dateien werden in Chunks gelesen. Wenn die KI etwas Wichtiges übersieht, frag konkreter ("read lines 100-200" oder "find function X").

### Eine Datei aus der Zwischenablage anhängen

Terminals können keine Bilder aus der Zwischenablage übergeben (Ctrl+V) — das ist eine Einschränkung aller TUIs. Drag & Drop von Dateien (das Terminal fügt den Pfad ein) funktioniert.

---

## Effort — Reasoning-Level

Unterstützt auf Anthropic-kompatiblen Sources: **zai, kimi, anthropic, ollama-cloud, claude-cli**. Das Modell "denkt" zuerst intern nach und antwortet dann. Bei schweren Aufgaben liefert das drastisch bessere Ergebnisse, ist aber langsamer und frisst mehr Kontingent.

### Level

```
/effort                      # Picker öffnen
```

Sechs Positionen: **off** (0) · **low** (1024) · **medium** (4096) · **high** (8192) · **xhigh** (16384) · **max** (32000) Token.

Steuerung im Picker: **← →** zum Auswählen, **Enter** zum Anwenden, **Esc** zum Abbrechen.

**Shift+Tab** schaltet Effort-Level durch, ohne den Picker zu öffnen (funktioniert in manchen Terminals nicht — dann auf `/effort` zurückfallen).

Wird in `~/.config/execai/config.json` persistiert, sichtbar in der Statuszeile als `effort : high`.

---

## Loop und Autoloop — wiederholen und warten

### `/loop` — ein einfacher Repeater

Einen Prompt auf einem Timer laufen lassen:

```
/loop 5m check the CI build status
```

Alle 5 Minuten sendet der Agent den Prompt erneut. Nützlich zum Pollen — "warte, bis etwas passiert".

```
/loop          # aktuellen Status zeigen
/loop stop     # stoppen
```

Die Statuszeile zeigt `🔁 loop: 5m`. Funktioniert nur, solange die TUI offen ist.

### Autoloop — die KI entscheidet, wann sie aufwacht

Smarter: der KI wird ein `schedule_wakeup`-Tool gegeben, und sie entscheidet selbst, wann sie zu einer Aufgabe zurückkehrt.

Beispiel:
```
› run `npm install`, wait for it to finish, then check the logs for errors
```

Die KI:
1. Startet `npm install` (Streaming in den Chat)
2. Sieht, dass die Installation ~5 Minuten dauern wird
3. Ruft `schedule_wakeup(delay_seconds=300, reason="waiting for npm install", prompt_on_wake="check logs and errors")` auf
4. Beendet die aktuelle Antwort
5. **5 Minuten später weckt die TUI den Agenten** mit dem Prompt "check logs and errors" auf
6. Die KI macht dort weiter, wo sie aufgehört hat — sie sieht den kompletten Conversation-History

Im UI sieht das so aus:
```
🌙 autoloop: wake in 5m (waiting for npm install) → prompt: "check logs and errors"
... 5 minutes later ...
› 🌙 [autoloop] check logs and errors
● ...AI continues...
```

Wenn die KI entscheidet, dass die Aufgabe erledigt ist — ruft sie einfach kein `schedule_wakeup` mehr auf, und der Autoloop stoppt von allein.

**Wann was nutzen:**
- `/loop` — ein festes Intervall, du weißt schon, wie lange du warten musst
- Autoloop — die KI findet es raus, du beschreibst nur die Aufgabe

---

## Agent-Memory

Der Agent merkt sich Fakten über Sessions hinweg — wie Claude Code, nur einfacher. Sie werden automatisch in den System-Prompt geladen (nur der Index, nicht alles auf einmal) — der Kontext bleibt leichtgewichtig.

### Wo was hingeht

**User-Memory** (projektübergreifende, persönliche Fakten über dich):
```
~/.config/execai/memory/
├── MEMORY.md              ← Index (immer im System-Prompt)
├── user_role.md           ← 1 Datei pro Fakt
├── feedback_style.md
├── project_alpha.md
└── reference_jenkins.md
```

Jede Datei — Frontmatter + kurzer Body:
```markdown
---
name: user-role
description: senior Go engineer, live in EU
metadata:
  type: user
---

- Write in Go, less often Python
- Prefer bubbletea for TUI
- Don't use sudo unless absolutely necessary
```

In `MEMORY.md` — eine Zeile, die auf die Datei verweist:
```
- [🎯 My stack](user_role.md) — Go, bubbletea, EU timezone
```

**Project-Memory** (spezifisch für dieses Repo):
```
<CWD>/EXECAI.md            ← eine einzige Datei
```

### 4 Eintragstypen

| Typ | Was | Beispiel |
|---|---|---|
| **user** | Rolle, Skills, Präferenzen | "Write in Go, tech lead" |
| **feedback** | Wie du sein Verhalten willst | "Don't do long summaries. Why: I don't read them" |
| **project** | Initiativen, Deadlines, Entscheidungen | "R5 — new branch for subscriptions, deadline Q3" |
| **reference** | Zeiger auf externe Systeme | "Jenkins: jenkins.mycompany.com, Grafana dashboards" |

### Wie du es nutzt

Sag dem Chat einfach, was er sich merken soll:
```
› remember that I write in Go and prefer bubbletea
```
Das Modell erstellt `user_role.md` (oder aktualisiert die vorhandene) + fügt eine Zeile zu `MEMORY.md` hinzu.

```
› project R5 — new branch for subscriptions/GUI, don't merge into R4
```
Erstellt `project_r5.md` im User-Memory. Wenn der Fakt zu DIESEM Repo gehört — aktualisiert `EXECAI.md` im CWD.

Das Modell füllt das Memory NICHT mit episodischen Fakten zu ("today I fixed a bug" — das gehört ins git log). Es schreibt nur Dauerhaftes rein.

Funktioniert über **alle Sources** — der System-Prompt ist derselbe.

---

## Projekte und Hintergrundmodus — Web → Agent

*Ab v6.17. Die Web-Seite braucht das Tool «Agenten» im Projektkatalog auf execai.ru — es wird schrittweise freigeschaltet.*

Dein Rechner kann **ein Tool innerhalb eines Web-Chat-Projekts** sein: Verzeichnis einmal an ein Projekt binden, den Hintergrund-Listener starten — und im Browser das Modell bitten, Dinge auf deinem Rechner zu erledigen. Das Modell ruft den Agenten wie jedes andere Projekt-Tool (ssh, git) auf, die Aufgabe läuft **im Projektverzeichnis**, die Antwort kommt zurück in den Chat.

### Verzeichnis binden

```
/project              # deine Projekte; ● — an dieses Verzeichnis gebunden
/project bind <Name>  # aktuelles Verzeichnis binden + diesen Rechner zum Projekt hinzufügen
/project on|off       # derselbe Schalter wie in der Projektkarte im Web
/project unbind       # Bindung lösen und den Rechner aus dem Projekt entfernen
```

Die Bindung tut zwei Dinge: merkt sich „dieses Verzeichnis auf diesem Rechner = jenes Projekt" und legt im Projekt einen normalen Tool-Eintrag an — wie ein ssh-Profil —, sodass der Rechner in der Projektkarte mit An/Aus-Schalter erscheint. Die Oberfläche zeigt Aliasse; die stabile Rechner-id überlebt ein erneutes Login.

### `execai serve` — der Hintergrund-Listener

```
execai serve
```

Lauscht auf Aufgaben aus dem Web-Chat und führt sie aus. Was zählt:

- Eine Aufgabe läuft **im Verzeichnis, das an ihr Projekt gebunden ist**, nicht dort, wo `serve` gestartet wurde. Verzeichnis weg → klarer Fehler statt Ausführung am falschen Ort.
- **Erlaubt ist, was in deiner `permissions.json` steht** (was du in der TUI mit „FÜR IMMER" bestätigt hast). Leere Datei → alles erlaubt, mit deutlicher Warnung beim Start.
- **Alles andere erfragt der Agent direkt im Web-Chat** *(ab v6.18)*: während der Aufgabe erscheint eine Frage mit genau dem Kommando, das der Agent ausführen will, und denselben Optionen wie in der TUI — „Einmal", „Das ganze Tool in dieser Aufgabe", „Dieses Kommando in dieser Aufgabe", „FÜR IMMER", „Ablehnen". „FÜR IMMER" wird in die `permissions.json` der Maschine geschrieben. Keine Antwort in ~2 Minuten (oder Tab zu) → der Agent bekommt eine Ablehnung: Schweigen erweitert niemals Rechte.
- `--read-only` — Nur-Lesen-Modus: keine Dateiänderungen, keine Kommandos.
- Jeder Tool-Aufruf landet im **Audit-Log** `~/.config/execai/serve-audit.log` (Rotation bei 8 MB) — man sieht, was der Agent nachts getan hat.
- Ein Daemon pro Rechner (pid-lock). `execai serve --status` zeigt pid/Laufzeit/Endpoint; `--stop` stoppt sanft (lässt die aktuelle Aufgabe fertig werden); `--stop --force` killt nach 5 s — das Ergebnis der aktuellen Aufgabe geht verloren, der Chat sieht einen Timeout.
- Agent offline → der Chat bekommt in ~12 s ein ehrliches „Agent antwortet nicht"; die Aufgabe bleibt in der Queue und läuft, sobald `serve` startet. Zustellung wird bestätigt (ack) — eine in eine tote Verbindung gereichte Aufgabe geht nicht verloren.
- Terminal zu — Prozess tot. Damit er überlebt:
  `setsid nohup execai serve > ~/.execai-serve.log 2>&1 &`

In diesem Modus ist AskUser deaktiviert (die Rückfragen des Modells; Berechtigungsfragen — siehe oben — kommen in den Web-Chat), das Iterationslimit ist niedriger (30 pro Aufgabe).

---

## Ausgaben / `/usage`

```
/usage
```

Zeigt Verschiedenes, je nach aktiver `/source`:

**ExecAI (Standard):** dein Plan + Guthaben + 4 Rate-Limit-Fenster (5h/Tag/Woche/Monat) mit Fortschrittsbalken + die letzten 14 Iterationen (Modell, Token, Preis in ₽).

**Kimi Code (`/source kimi`):** echtes Kontingent aus `api.kimi.com/coding/v1/usages` — Plan nach verfügbaren Modellen, Wochenlimit mit Reset-Zeit, rolling windows (5h etc.).

**Moonshot Platform (`/source kimi-api`), OpenAI (`/source openai`), Anthropic, Z.ai:** lokale Token-Zähler + Link zum Dashboard des Providers (die echte Abrechnung lebt dort).

---

## Befehle und Hotkeys

### Slash-Befehle

| Befehl | Was er macht |
|---|---|
| `/help` | Alle Befehle |
| `/model [id]` | Modell wechseln |
| `/source [name]` | Source wechseln (execai/zai/kimi/kimi-api/anthropic/openai/claude-cli/codex-cli/ollama) |
| `/connect <provider> [args...]` | Ein Abo verbinden. Ohne Args — Hilfe |
| `/disconnect <provider>` | Ein Abo trennen |
| `/subscriptions` (oder `/subs`) | Verbindungen auflisten |
| `/usage` | Plan + Ausgaben (Source-spezifisch) |
| `/effort` | Picker für Reasoning-Level (für Anthropic-kompatible Sources) |
| `/max-iterations [N]` | Limit für tool-use-Iterationen pro Turn. Ohne Arg wird der aktuelle Wert gezeigt (Standard 40). Bei Überschreitung — sanfter Stopp |
| `/paste` | Liste der Pastes (Ctrl+V großer Blöcke) — [Pasted #N — L lines, C chars] |
| `/paste show <N>` | Inhalt von Paste #N |
| `/project [bind <Name>\|on\|off\|unbind]` | Verzeichnis an ein Web-Chat-Projekt binden; on/off — Projektschalter (ab v6.17) |
| `/loop <interval> <prompt>` | Loop mit festem Timer |
| `/loop stop` | Stoppen |
| `/log` | Letzte 20 LLM-Requests (welches Modell tatsächlich geantwortet hat) |
| `/new` | Neue Conversation (die aktuelle wird gespeichert) |
| `/sessions` | Liste der Conversations |
| `/resume <num\|id>` | Eine gespeicherte Conversation öffnen |
| `/compact` | History per LLM in eine Zusammenfassung verdichten (für lange Conversations) |
| `/cd <path>` | Working Directory wechseln |
| `/clear` | History leeren (Ctrl+L) |
| `/whoami` | Wer eingeloggt ist |
| `/config` | Konfiguration zeigen |
| `/permissions` | Persistente Tool-Berechtigungen |
| `/classic on\|off` | Klassische TUI (alt-screen+Maus) statt Ink-style umschalten. Erfordert Neustart |
| `/mouse on\|off` | Maus-Capture (nur in klassischer TUI relevant) |
| `/login` | Neu einloggen |
| `/logout` | Ausloggen |
| `/quit` | Beenden (Ctrl+D) |

> Tipp: tippe `/` und das Autovervollständigungs-Menü zeigt jeden Befehl. Bei Befehlen **ohne Argumente** führt die Auswahl aus dem Menü sofort aus (ein Enter). Befehle, die ein Space-getrenntes Argument nehmen (`/model `, `/source `, `/cd `), warten auf Eingabe.

### Hotkeys

| Taste | Aktion |
|---|---|
| **Enter** | Senden |
| **Shift+Enter** | Neue Zeile (mehrzeilig) |
| **↑ / ↓** | Verlauf deiner vorherigen Eingaben (wie in bash) |
| **Tab** | Autovervollständigungs-Vorschlag annehmen |
| **Shift+Tab** | Effort-Level durchschalten (oder den Picker öffnen) |
| **PgUp / PgDn** | Chat scrollen |
| **Ctrl+R** | Fuzzy-Suche über Sessions |
| **Ctrl+C** | Stream abbrechen (oder zweimal zum Beenden) |
| **Ctrl+D** | Beenden (zweimal zum Bestätigen) |
| **Ctrl+L** | History leeren |
| **Shift+drag** | Text auswählen (wenn die Maus gecaptured ist — `/mouse off`, um normal auszuwählen) |
| **Mausrad** | Viewport scrollen |

---

## Sessions und Verlauf

Jede Conversation wird nach jedem Austausch automatisch nach `~/.config/execai/sessions/<uuid>.json` gespeichert. Beim Neustart:
- Wenn im selben Ordner eine Conversation jünger als 24h war — wird sie fortgesetzt
- Ansonsten startet eine neue

`/sessions` — Liste aller Conversations. `/resume 3` — die dritte öffnen. `/resume <id>` — per ID.

`Ctrl+R` — Fuzzy-Suche: tipp irgendein Wort → sie sucht sowohl in Titeln ALS AUCH in den Inhalten jeder Conversation.

`/compact` — wenn eine Conversation zu lang wird und der Kontext überläuft, komprimiert der Agent den älteren Teil in eine einzige Zusammenfassung und behält die letzten 6 Turns.

Der Eingabe-Verlauf (`↑/↓`) wird ebenfalls in `~/.config/execai/input_history` gespeichert — er überlebt Neustarts.

---

## Wo die Dateien liegen

| OS | Pfad |
|---|---|
| Linux   | `~/.config/execai/` |
| macOS   | `~/Library/Application Support/execai/` |
| Windows | `%APPDATA%\execai\` |

| Datei | Was |
|---|---|
| `config.json` | api_base, selected_model_id, thinking_budget (effort) |
| `credentials.json` | JWT (Modus 0600) |
| `host_id` | Stabile machine-id (nur wenn /etc/machine-id etc. nicht verfügbar sind — Fallback) |
| `subscriptions.json` | Verbundene Abos (Keys im Klartext — sicher aufbewahren) |
| `permissions.json` | Persistente Tool-Allow-List |
| `sessions/<uuid>.json` | History jeder Conversation |
| `memory/MEMORY.md` | Index des User-Memory (siehe [Memory](#agent-memory)) |
| `memory/*.md` | Einzelne Fakten (user/feedback/project/reference) |
| `input_history` | Letzte 200 deiner Eingaben |
| `requests.log` | LLM-Request-Log (für `/log`) |
| `auth-poll.log` | Device-Flow-Login-Diagnostik (zum Debuggen) |
| `models_cache.json` | Cache des Modell-Katalogs — wird genutzt, wenn das Netzwerk down ist, damit execai trotzdem offline startet |
| `installed_arch_sha` | Architektur-SHA für den Auto-Update-Check |
| `last_remote_sha` | Stempel für Versionsvergleich |

### Memory — siehe eigener Abschnitt

[Agent-Memory](#agent-memory) — User- + Project-Struktur, wird bei jeder Session automatisch in den System-Prompt geladen.

---

## Troubleshooting

**`execai: command not found`** nach der Installation:
- In der aktuellen Session: `exec bash -l` oder ein neues Terminal öffnen
- Oder manuell: `export PATH="$HOME/.local/bin:$PATH"`

**"zai subscription is not connected"** bei `/source zai`:
- Zuerst `/connect zai <key>`. Wo du den Key herbekommst — siehe Abschnitt [Sources](#sources--execai-und-abonnements)

**429 "Insufficient balance"** bei Z.ai:
- Du nutzt einen normalen API-Key, keinen Coding-Plan-Key. Hol dir einen echten Coding-Plan: https://z.ai/manage-apikey/apikey-list (Bereich Individual Coding Plan)

**404 "Not found"** während des Chats (ExecAI):
- Das Gateway kennt den Endpoint eventuell nicht. Melde es an die Entwickler — aicore-vbai muss möglicherweise neu deployed werden.

**TUI öffnet sich nicht, leerer Bildschirm**:
- Unter Windows das **Windows Terminal** nutzen, nicht den alten conhost. Unter Linux — irgendein modernes (gnome-terminal, kitty, alacritty).
- Defender: `Add-MpPreference -ExclusionPath "$env:LOCALAPPDATA\execai"` und neu installieren.

**Zeichencodierung kaputt** (`Ð�escribe` statt `Describe`):
- Alter Bug in der bubbles-Version. execai updaten: `curl -fsSL .../install.sh | bash`.

**Auto-Update kommt nie an**:
- Beim Start zeigt die Statuszeile unten `✓ execai R5.X — latest version` oder `🔔 New version available`.
- Wenn es nicht funktioniert — `EXECAI_UPDATE_CHANNEL=R5` überschreibt den Channel.

**Bilder werden nicht gesendet**, obwohl der Pfad da ist:
- Prüfe, ob der Pfad in Anführungszeichen steht (einfach oder doppelt), falls er Leerzeichen oder Nicht-ASCII enthält: `'/path/to/Screenshot.png'`
- Oder unquoted, wenn der Pfad keine Leerzeichen hat: `/path/to/foo.png`
- Wenn die KI weiterhin "no image" sagt — prüfe das Modell, es muss ein Vision-Modell sein (sonnet/gpt-5/glm-5v etc.)

**Request hängt**:
- `Ctrl+C` bricht den aktuellen Request ab. Der History bleibt erhalten.

**"Reached N iterations limit"** im Chat:
- Das ist KEIN Fehler — es ist ein sanfter Stopp nach Erreichen des tool-use-Iterationslimits (Standard 40).
- Sag einfach "continue" — der Agent nimmt eine weitere Charge derselben Größe und macht dort weiter, wo er aufgehört hat.
- Wenn die Aufgabe autonom und groß ist — heb das Limit an: `/max-iterations 100` (Bereich 1-500).
- Wird in `~/.config/execai/config.json` gespeichert, gilt für nachfolgende Turns.

**"context deadline exceeded" / Modelle laden nicht**:
- Seit R5.67+ startet execai IMMER, auch ohne Netzwerk.
- Beim ersten Start holt es sich den Katalog von der API und cached ihn in `~/.config/execai/models_cache.json`.
- Fällt das Netzwerk aus — nutzt es den Cache und zeigt `ℹ Using cached catalog`.
- Wenn auch kein Cache da ist — greift ein eingebauter Fallback (Claude Sonnet 4.6), die TUI öffnet sich, echte Requests können scheitern, aber das Interface lebt.

---

## Support

- Bugs/Feature-Requests: [github.com/execai/execai-agent/issues](https://github.com/execai/execai-agent/issues)
- ExecAI-Docs: https://chat.execai.ru/

---

**execai** — von ExecAI/VBAI. MIT-artige Lizenz.
