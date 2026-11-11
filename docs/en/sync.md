# Sync between computers

The agent's memory, settings, permissions and provider keys are the same on
all your computers. Save a fact to memory at work — the agent knows it at
home; connect a Z.ai key on the laptop — it is on the work computer too.
Everything is encrypted on the computer with your sync password; the server
keeps only the encrypted copy — we do not see or parse what is inside.

Sync comes with the "Work" and "Max" plans:
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
2. keep yours and replace the server's — the server gets this computer's
   memory, settings and projects; what the server holds now is encrypted
   with the other key and is deleted from it (the copies on the other
   computers stay, and they ask for the password again). The agent names
   beforehand the projects whose memory is on the server but not on this
   computer — it is deleted from the server along with the rest; their
   `.execai/` stays in the folders where they are bound and goes back to the
   server from there, with this key, when those computers sync again (the
   agent there says the project was sent again with the current key). Only
   what the question named is deleted, and only at the version the agent
   showed: a project, a memory file or the settings changed on another
   computer after the question stay on the server under the earlier key,
   the agent says so, and every `/sync on` asks about them again — delete
   them or leave them — until they are decided.

The first option is the usual one.

## 3. What syncs and what does not

| Synced | Not synced |
|---|---|
| Personal memory: `memory/` — `MEMORY.md` and the fact files, file by file | API address (`api_base`), the notifications setting (`notifications`, [more](notifications.md)) and the browser of the `execai ui` window (`ui_browser`) — each computer keeps its own |
| The config as one encrypted file: `config.json`, `subscriptions.json` (sources with their provider keys), `permissions.json` | ExecAI sign-in (`credentials.json`), machine id |
| A project's whole `.execai/` folder (the project memory `EXECAI.md` and everything there) — for projects bound to the web chat, see section 6 | Conversation history, model cache, plan, project list, `serve.pid` |

The config goes to the server as one encrypted file — we do not see or parse
what is inside. Each computer keeps for itself the server address `api_base`,
the `notifications` setting and the window browser `ui_browser`; the last two
are not even sent.

Where the files are:

| OS | Directory |
|---|---|
| Linux | `~/.config/execai/` |
| macOS | `~/Library/Application Support/execai/` |
| Windows | `%APPDATA%\execai\` |

A memory or settings file over 256 KB, a project's `.execai/` over 1 MB
(compressed), anything that cannot be read, and symbolic links (a `memory/`
or `.execai/` folder that is a link, a link inside them, a settings file
that is a link) are left out of the exchange — in both directions: they are
not sent, nothing from the server is written over them, they are not
removed, and no deletion of them goes to the other computers. A link or a
file that cannot be read inside a project's `.execai/` is left out alone:
for that name the server keeps its version and this computer keeps its
own, and the rest of the project syncs; the name keeps its agreed version,
so once it can be read again it is decided as if it never left: an edit
made here goes up, a newer version comes here, a deletion made elsewhere
removes it here. The agent says once what is left
out and why; `/sync` shows the list. The server holds up to 32 MB and 2000
files in all.

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
already there with another key, the one connected later stays — the one
place where a time decides which version stays; the other key is kept.

In a shell: `execai sync keys export`, `execai sync keys import`. Moving keys
needs neither a sign-in nor a plan.

## 6. Projects

The config and memory are shared by all computers; projects sync project
by project. A web chat project is linked to a directory on a computer by a
binding, and each computer may keep its own directory: `~/work/app` on the
laptop, `D:\src\app` at work. The `.execai/` folder of a bound project
travels as one encrypted file.

The agent keeps the project memory in `.execai/EXECAI.md` — that is why it
travels to your other computers. `EXECAI.md` at the project root is the
project's instructions (often kept in git): the agent reads it every turn
but does not write it unless you ask, and it does not travel between
computers. Older versions kept the project memory in the root file; if it is
there, the chat says so once, and `/project memory` copies it to
`.execai/EXECAI.md` (the root file stays; the same text is read once).

**The `.execai/` folder and git.** A `.execai/` folder appears in the
project. It holds the project memory `EXECAI.md` and whatever you or the
agent put there; `uploads/` — files attached in the browser (`execai ui`);
copies of versions that were never sent, `*.local-<time>` (section 7; older
agents also left `*.server-<time>`). Sync carries everything except
`uploads/`, those copies and editors' temporary files. Whether `.execai/`
goes into git is up to you: in a private repository it is a way to share
the project memory with the team, in a public one probably not — it may
hold internal notes. The agent does not
edit `.gitignore`; to keep git away from it, add a `.execai/` line to your
`.gitignore` yourself.

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

If `.execai/` exists both here and on the server and they differ, the
agent brings them together file by file, as in section 7 — a project is
never replaced by one side as a whole, and what goes up is the result: a
file changed here and another changed on the other computer end up on both.

**One folder holds one project.** A folder this computer already bound to
another project is not bound again: `/project bind`, `/project publish` and
the editor panel say which project has it. If two projects reach one folder
anyway (bindings made elsewhere), the exchange keeps the folder to the
project whose files were agreed in it and leaves the other out, saying so.

**The agent never deletes the memory of a whole project.** An empty folder,
a folder made again, a removed `.execai/`, one with only placeholder files
left (`.gitkeep`, `.DS_Store` and the like), a binding moved to another
folder — none of it is a deletion: the agent takes the server's version into
the folder, as on a new computer, and says so in one line. If the bound
folder is not on this computer at all (removed, disk not mounted), the agent
says so and writes nothing there. A bound folder that is there but empty —
the mount point of a disk that is not mounted — is filled from the server
like any empty folder; once the disk is mounted again, its folder is
another one, and what differs there is kept next to the files.

If the server marks the project memory deleted (agents before 7.21 do that
when their folder moves), the files here are not erased, and there is no
agreed version any more. If they are exactly the files the mark deleted,
they stay here, are not sent, and the agent says so once; `/sync` shows how
many such projects there are. If anything here is new to the server — a
note written after the mark, an edit not sent yet, the `.execai/` of a
computer that joins — the project comes back: everything here goes up,
and the agent says so. To bring back the very files that were deleted,
change any of them; to delete them here too, remove `.execai/` by hand. A
version that comes later over the mark is merged as with nothing to
compare: what differs here is kept next to the file. Unbinding
(`/project unbind`) leaves the project's data on the server: other
computers may still have it bound.

`/sync` shows «Projects bound to this computer: N (names)», and in the line
«Syncing: … projects (M)» how many of them match the server. One project may
take up to 1 MB on the server (compressed).

## 7. When it syncs

- when the chat, `execai serve` or the editor panel starts;
- every 10 minutes while the agent runs;
- 10 seconds after memory, settings, sources, permissions or a bound
  project's `.execai/` change;
- right after `/project bind` and `/project publish` — for that project;
- on exit;
- on `/sync now`.

### Which version stays

Every file is decided on its own — a memory note, each settings file, each
file of a project's `.execai/`. The agent compares three versions by their
content: the one here, the one on the server, and the one this computer
and the server last agreed on (the agreed version: always one the server
took or gave; for projects it is kept file by file in `sync-bases.json`,
next to `sync.json`). **The time of a file plays no part**: a clone, an
unpacked archive, a restored backup or a wrong clock give a file any time
at all. A file here is replaced without a word only when it is proven out
of date; everything else is said.

| Here | On the server | What happens | What is kept of this computer's version |
|---|---|---|---|
| the same as on the server | | nothing | — |
| as agreed | changed | the server's version comes here | the previous one, in the history |
| as agreed | changed back to an earlier version | the server's version comes here; the agent says so | the newer one, next to the file for good |
| as agreed | deleted | removed here; the agent says so | the last version, in the history |
| changed or deleted, in the same folder | as agreed | the change goes to the server, a deletion too | — |
| changed to an earlier version, in the same folder | as agreed | goes to the server as a return; the agent says so | — |
| changed, in a folder that is not the same one | as agreed | the server's version stays | next to the file for good; the agent says so |
| missing, and that is not taken for a deletion (below) | as agreed | the server's version comes back | — |
| changed | changed otherwise | the server's version stays | next to the file for good; the agent says so |
| changed | deleted | this version stays and goes up again | — (the agent says so) |
| deleted | changed | the server's version comes here; the agent says so | — |
| nothing agreed, the file only here | | goes up | — |
| nothing agreed, the file only on the server | | comes here | — |
| nothing agreed, both differ | | the server's version stays | next to the file for good; the agent says so |
| nothing agreed, deleted on the server | | this version stays and goes up again | — (the agent says so) |

**The same folder.** The agent knows a folder by its identity: the number
and creation time of its `.execai/` (of `memory/` for the personal memory),
recorded at every exchange in `sync-bases.json`. The device number is not
part of it — on some systems it changes across reboots. A folder moved with
`mv` keeps its identity and stays the same folder at its new path. Another
folder at the same path (the repository cloned again, the folder made
again, a copy) is not the same one. Where the file system cannot tell
folders apart (network drives, sshfs; `/mnt/c` in WSL was not checked) the
path is trusted for edits, but a missing file there is not taken for a
deletion and comes back, and an older version put back is not taken for an
edit.

**An older version put back.** If the content here is one of the file's
earlier agreed versions and the folder is not known for sure to be the same
one, it is an old copy (a backup, an old checkout), not an edit: the
server's version stays, the old one goes to the history, the agent says so
once. In the same folder, going back to an older text is a change like any
other: it goes up, and the agent says it went as a return; your other
computers keep the newer text next to the file and say so. The same happens
when an older agent writes an old version of the whole project. A settings
file that comes back to an earlier version is the exception: the version
here goes to the history, not next to the file — it may hold a provider key
that was removed on purpose. The agent says so for permissions and
prohibitions (check that no prohibition you need was lifted) and for
provider keys; any other settings file is replaced without a word.

A project file missing from a version that came from the server may have
been deleted on another computer, or that version may be an older one: the
agent cannot tell, so it says the file is not in that version and is
removed here, with its last version in the history.

**An older snapshot of a project.** A version of a project that came from
the server with at least one file in an earlier version is taken for an
older copy of the whole project (an older agent sent an old checkout, a
version was restored on the server). The files it lacks were not deleted by
anyone: they are removed here, as that version has it, but kept for good
next to their place, like the newer versions of the files that went back;
the agent says it in one line per project. A version with no earlier file
in it removes a file it lacks as a deletion made elsewhere (with the
history).

A return is recognized within the last 64 agreed versions of a file; going
back to an older one is taken for an ordinary edit, and the newer text stays
in the history of the other computers for 30 days.

**A missing file is a deletion** only in the same folder (by its identity)
that still holds the rest of its content: for a project, a `.execai/` with
other files in it; for the memory, an index `MEMORY.md` with entries. So
one note, several or all of them can be deleted — as long as the index
stays. A folder emptied or made again, a removed `.execai/`, an index gone
or left without a single entry, a folder trusted by its path only — missing
files come back from the server. An exchange that only sends (on a change,
on exit) sends no deletions then, and takes nothing from the server at all.

**The memory index** is merged entry by entry, not as one file: every note
that exists keeps its line, and a line goes only together with its note. An
index the model wrote again with fewer lines gets the lines of the
existing notes back; where both computers changed it, the lines of both
stay. A note without a line gets one, made from the description in its
header or from its name (not in the very exchange that first sends the
note: the model may be writing its line). A line whose note was deleted
goes; a line whose note no server has seen stays — it may be a note another
computer cannot send. The text you write around the lines (headings,
paragraphs, rules) is decided like any file, by the table above, compared
without the note lines: changed on one side, that side's text stays;
changed on both, the server's, and this computer's index is kept for good
and said. Whatever of the index here a merge leaves out is kept: in the
history when the server had it all, for good otherwise. An index without a
single entry (the template) never goes up; when all notes are deleted at
once, their lines still go from the index here. With notes and no index at
all (a new account), the index is made with their lines after the first
full exchange. When the text around the lines changed on both sides, the
line about the copy asks to move what you need by hand: copying the old
index back would undo the other computer's changes.

**The settings** are decided file by file: a provider key added on one
computer and a model changed on another both stay. They are compared
without what is each computer's own (the server address, notifications)
and without what the agent writes into them by itself: a provider's model
list, when it was fetched, when a source was connected, a provider's
default address. Such a change alone is not sent and makes no conflict.
A model the agent picks by itself on one computer (the chosen one is not
in its list, say on a start without the network) stays on that computer:
the settings travel with the model you chose. A model you choose (`/model`,
the panel) goes to all of them.
If `permissions.json` was changed here and on the server, the agent says
plainly that this computer's permissions and prohibitions were not
applied, and where its file is kept.

**Two kinds of copies.**

- *Kept for good* — content the server never had and that the server's
  version replaced here, the newer version of a file when the server sends
  an earlier one, and what differs here when there is nothing to compare
  with.
  It may be the only copy there is, so it stays next to the file in a
  project (`EXECAI.md.local-<time>`) and in `sync-unsent/<date>/` of the
  config directory for memory and settings (for example
  `~/.config/execai/sync-unsent/2026-11-11/fact.local-121003.md`). The
  agent says in one line, per project or area, how many there are and
  where — also when the same copy is there already; to use one, copy it
  over the file and it goes out as an ordinary change. The agent never
  removes these copies — only you do; `/sync` shows how many there are and
  where.
- *History* — versions the server had that were replaced by a newer one or
  removed because another computer deleted the file. They go to
  `sync-history/` in the config directory, not to the project, and stay
  for 30 days however many versions a file gets (the server keeps no
  versions of its own, so this may be the last copy). The line about a
  file removed here names its copy in the history; for several files, the
  folder that holds their copies. Up to 256 MB per project (and as much
  for the memory and for the settings): over that the oldest go first, and
  the agent says so if any younger than 30 days had to go. Earlier versions
  of the settings, provider keys included, are there too
  (`sync-history/settings`), only on this computer; to remove them at once,
  delete that folder.

No copy is made when nothing of the old content is lost: the same bytes,
every entry of the index, or, for the settings, everything but what is each
computer's own (the server address, notifications, a model the agent picked
itself, a source's model lists). Copies of memory and settings are mode
0600, in folders only you can open (0700) — they may hold provider keys —
and a line never shows what is inside. Copies never travel. Copies left by earlier agents
(`*.local-*`, `*.server-*`, `sync-conflicts/`) are never removed; `/sync`
counts them too, those in `sync-conflicts/` on a line of their own. Some
earlier builds removed the day folders of `sync-conflicts/` older than 30
days, which is why this agent keeps its copies elsewhere.

When a second computer joins, the settings of the first arrive on it; if its
own held something the arriving ones lack, the agent says just that: the
settings came from your other computer, the previous ones are kept.

**While the agent works.** A replacement goes in this order: the copy is
written and flushed to disk; the new content is written into a temporary
file next to the target and flushed; then the target is read again and,
only if it is still what the decision was taken on, the temporary file is
renamed over it. So a half-written file is never there, and a copy is on
disk before what it saves is replaced, also after a power cut. If the file
changed meanwhile, it is left alone and the next exchange decides again.
Some writers take a short lock for their write, and the agent takes the
same lock for that last comparison and rename, so none of their writes can
fall between the two: the model's Write and Edit tools (for any file in a
`.execai/` folder or the personal memory, notes and `MEMORY.md` alike), the
memory index template written at start, `/project memory`, and a memory
import. Everything else does not take it: commands the model runs in the
shell (`>>`, `sed -i` and the like), an external source that writes files
by itself (claude-cli, Codex), the agent's own settings writes (`/connect`,
a model list refresh, `/model`), your editor, a script. If one of them saves
a file in exactly the moment between the last comparison and the rename
(well under a millisecond), that one save is replaced by the server's
version and is lost unless it is that version. An exchange that stops anywhere — a crash,
a full disk, a file that cannot be written — comes together on the next
one: the agreed versions are recorded only once the files are in place and
the server took what was sent, so nothing is sent half-written, twice, or
taken for a deletion. Names that differ only in case are left as they are,
here and on the server, where the folder does not tell them apart (the
folder is asked, not guessed from the system); a name that is a file on
one side and a folder on the other is left as it is too; the agent says
so once, and `/sync` lists it while it lasts. A `sync-bases.json` written
by a newer agent is read and not changed; the agent asks to update.

Where these lines appear: in the terminal chat, in the output of `execai
serve`, and in the editor panel (the plugin, Studio) as a service line in
the chat; the sync commands are typed in a terminal there: `execai sync`.
With several editor windows open, one of them makes the exchange, and that
one shows the line.

## 8. Commands

| In the chat | In a shell | What it does |
|---|---|---|
| `/sync` | `execai sync` | status: on or off, last exchange, what syncs and what does not |
| `/sync on` | `execai sync on` | turn sync on on this computer |
| `/sync now` | `execai sync now` | sync now |
| `/sync password` | `execai sync password` | change the password; nothing to do on the other computers |
| `/sync off` | `execai sync off` | turn sync off on this computer; the server keeps the data |
| `/sync forget` | `execai sync forget` | delete everything from the server (asks for the word "delete") |
| `/sync keys export` | `execai sync keys export` | the fallback without server and plan: key transfer code and its phrase |
| `/sync keys import` | `execai sync keys import` | take keys by code and phrase |
| `/project publish [name]` | `execai project publish [name]` | make a web chat project from this directory and send `.execai/` |
| `/project bind <name>` | — | bind this directory to a web chat project and take `.execai/` |
| `/project memory` | — | copy the root `EXECAI.md` into `.execai/EXECAI.md`, so the project memory travels with the project |

## 9. Deleting the data from the server

`/sync forget` deletes from the server the encryption key, all synced
memory, the config (provider keys included) and the list of computers. Everything stays on the computers
themselves. The other computers notice at their next check and pause sync
— they do not send anything back; on each of them you then turn it off
(`/sync off`) or on again (`/sync on`). Deleting works without a plan too.

## 10. If something is off

- **«Sync comes with the "Work" and "Max" plans»** — the current plan has no
  sync. When a plan ends, sync pauses: files on the computer are untouched.
  After renewal it resumes by itself within a day, or at once on `/sync now`.
- **«The password did not fit»** — check the keyboard layout and Caps Lock.
  The password cannot be recovered; if it is lost, delete the data from the
  server (`/sync forget`) and turn sync on again — memory on the computers
  stays.
- **«The sync key was changed on another computer»** — someone chose «keep
  yours» there. Type `/sync on`, enter the password and choose «take the key
  from the server».
- **«On the server, encrypted with an earlier sync key»** — after «keep
  yours» some projects, memory files or the settings stayed on the server
  under the earlier key, and changes to them from here cannot go up. `/sync
  on` asks whether to delete them from the server; then this computer's
  versions go up again.
- **«Paused: the server says it has no sync key»** — sync may have been
  deleted from another computer (`/sync forget`), or something in the way
  answered for the server. Nothing is reset or switched off here: check
  with `/sync`, make a new key with `/sync on`, or stop with `/sync off`.
