# Sync between computers

The agent's memory, settings, permissions and provider keys are the same on
all your computers. Save a fact to memory at work — the agent knows it at
home; connect a Z.ai key on the laptop — it is on the work computer too.
Everything is encrypted on the computer with your sync password; the server
keeps only the encrypted copy — we do not see or parse what is inside.

Sync comes with the «Work» and «Max» plans:
https://chat.execai.ru/settings/payment

## 1. The first computer

In the chat:

```
/sync on
```

or in a shell:

```bash
execai sync on
```

The agent asks you to make up a sync password — at least 10 characters.
You will enter it on your other computers. **The password cannot be
recovered**: we do not have it. Keep it in a password manager.

The agent then sends memory and settings right away and shows what syncs:

```
Sync is on.
Syncing: memory (files: 12), settings and provider keys — all encrypted on this computer.
```

## 2. The second computer and the next ones

Install the agent, sign in to the same ExecAI account and type `/sync on`.
The agent sees that sync is already on and asks for the password from the
first computer. Enter it — memory, settings and provider keys arrive. The
config is shared by all computers: the active source comes from the first
one too. Sources only this computer had are not lost — they join the shared
ones and travel to the others.

A wrong password gets «The password did not fit» and another try.

If this computer already had its own encryption key (say, after `/key new`)
and it differs from the key on the server, the agent offers a choice:

1. take the key from the server — your previous key is kept next to it
   (`sync.key.old-<date>`);
2. keep yours and replace the server's — everything on the server is
   overwritten with this computer's data, and the other computers ask for the
   password again.

The first option is the usual one.

## 3. What syncs and what does not

| Synced | Not synced |
|---|---|
| Personal memory: `memory/` — `MEMORY.md` and the fact files, file by file | API address (`api_base`) — each computer keeps its own |
| The config as one encrypted file: `config.json`, `subscriptions.json` (sources with their provider keys), `permissions.json` | ExecAI sign-in (`credentials.json`), machine id |
| A project's whole `.execai/` folder (the project memory `EXECAI.md` and everything there) — for projects bound to the web chat, see section 6 | Conversation history, model cache, plan, project list, `serve.pid` |

The config goes to the server as one encrypted file, whole — we do not see or
parse what is inside. The only thing each computer keeps for itself is the
server address `api_base`.

Where the files are:

| OS | Directory |
|---|---|
| Linux | `~/.config/execai/` |
| macOS | `~/Library/Application Support/execai/` |
| Windows | `%APPDATA%\execai\` |

A file over 256 KB stays on its own computer — `/sync` says which one. The
server holds up to 32 MB and 2000 files in total.

## 4. How it is protected

- Every memory file and the whole config (provider keys included) are
  encrypted on the computer with your key before they are sent. The key is
  in `sync.key` (readable by its owner only).
- The key reaches the server only locked with your password. The server
  never sees the password, so it can neither read nor recover anything.
- Memory file names are hidden too: the server sees a meaningless id, the
  real name is inside the encrypted copy.
- What the server does know: how many files and how large, when they
  changed, and for each computer — the names of its sources (e.g. «ExecAI ·
  Z.ai Coding Plan · Moonshot»), the active one and the agent version. The
  provider keys themselves are only inside the encrypted config.

## 5. Moving keys without sync — the fallback

With sync on, provider keys arrive by themselves, with the config. Without
sync (or without a plan) you can move them directly — no server, no plan.

On the computer that has the keys:

```
/sync keys export
```

The agent shows a transfer code and, separately, a six-word phrase. Carry
them to the other computer yourself (a messenger, a USB stick), ideally by
two different ways: together the code and the phrase open your keys.

On the other computer:

```
/sync keys import
```

Paste the code, then type the phrase. New sources are added; the active
source does not change — to switch: `/source <name>`. If the source was
already there with another key, the one connected later stays.

In a shell: `execai sync keys export`, `execai sync keys import`. Moving keys
needs neither a sign-in nor a plan.

## 6. Projects

The config and memory are shared by all computers; projects sync project
by project. A web chat project is linked to a directory on a computer by a
binding, and each computer may keep its own directory: `~/work/app` on the
laptop, `D:\src\app` at work. The `.execai/` folder of a bound project
travels as one encrypted file.

**Your directory into the web chat.** In the project directory:

```
/project publish [name]
```

The agent creates a web chat project (named after the directory by
default), binds this directory to it and sends `.execai/` right away. In a
shell: `execai project publish [name]`.

**A web chat project into a directory.** On another computer, in the
directory you want (an empty one is fine):

```
/project bind <name>
```

The agent binds the directory and takes `.execai/` from the server at once.

If `.execai/` exists both here and on the server and they differ, the newer
copy wins. The other one is not lost: its files stay next to them —
`EXECAI.md.local-<date>` (it was here) or `EXECAI.md.server-<date>` (it was
on the server) — and the agent says so. These copies never travel.

A file deleted in `.execai/` is deleted on the other computers too.
Unbinding (`/project unbind`) leaves the project's data on the server: other
computers may still have it bound. Project files sync while sync is on;
`/sync` shows «Projects: N (names)». One project may take up to 1 MB on the
server (compressed).

## 7. When it syncs

- when the chat, `execai serve` or the editor panel starts;
- every 10 minutes while the agent runs;
- 10 seconds after memory, settings, sources, permissions or a bound
  project's `.execai/` change;
- right after `/project bind` and `/project publish` — for that project;
- on exit;
- on `/sync now`.

If a file was changed on two computers at once, the later edit wins. The
memory index `MEMORY.md` does not lose lines that way: lines from both
computers are merged. A memory file deleted on one computer is deleted on
the others, but an edit made after the deletion is kept.

## 8. Commands

| In the chat | In a shell | What it does |
|---|---|---|
| `/sync` | `execai sync` | status: on or off, last exchange, what syncs and what does not |
| `/sync on` | `execai sync on` | turn sync on on this computer |
| `/sync now` | `execai sync now` | sync now |
| `/sync password` | `execai sync password` | change the password; nothing to do on the other computers |
| `/sync off` | `execai sync off` | turn sync off on this computer; the server keeps the data |
| `/sync forget` | `execai sync forget` | delete everything from the server (asks for the word «delete») |
| `/sync keys export` | `execai sync keys export` | the fallback without server and plan: key transfer code and its phrase |
| `/sync keys import` | `execai sync keys import` | take keys by code and phrase |
| `/project publish [name]` | `execai project publish [name]` | make a web chat project from this directory and send `.execai/` |
| `/project bind <name>` | — | bind this directory to a web chat project and take `.execai/` |

## 9. Deleting the data from the server

`/sync forget` deletes from the server the encryption key, all synced
memory, the config (provider keys included) and the list of computers. Everything stays on the computers
themselves. The other computers notice at their next check and switch sync
off — they do not send anything back. Deleting works without a plan too.

## 10. If something is off

- **«Sync comes with the «Work» and «Max» plans»** — the current plan has no
  sync. When a plan ends, sync pauses: files on the computer are untouched.
  After renewal it resumes by itself within a day, or at once on `/sync now`.
- **«The password did not fit»** — check the keyboard layout and Caps Lock.
  The password cannot be recovered; if it is lost, delete the data from the
  server (`/sync forget`) and turn sync on again — memory on the computers
  stays.
- **«The sync key was changed on another computer»** — someone chose «keep
  yours» there. Type `/sync on`, enter the password and choose «take the key
  from the server».
- **«Files on the server that do not open here»** — they are encrypted with
  another key; same fix: `/sync on`.
