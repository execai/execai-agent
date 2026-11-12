# Notifications

Gave the agent a task and switched to the browser? The terminal chat tells
you when it needs you:

- a turn has finished — `ExecAI: turn finished — my-project`;
- the agent waits for your approval of a tool — `ExecAI is waiting for approval: Bash`;
- the agent asks you a question — `ExecAI has a question`.

The text names the project folder or the tool and nothing else: no prompt,
no answer, no command arguments, no paths.

## Modes

| Mode | What you get |
|---|---|
| `system` (default) | a desktop notification and the terminal bell |
| `bell` | only what goes through the terminal: its own notification and the bell. Starts no program, works over SSH |
| `off` | nothing |

`/notifications` shows the mode, what this computer uses and whether the
terminal reports focus. Change it with `/notifications system`,
`/notifications bell` or `/notifications off`; `/notifications test` sends
one right away. Outside the chat: `execai config set notifications=bell`,
or `"notifications": "bell"` in `config.json`.

## How it is delivered

- **Terminals with notifications of their own** (they understand OSC 9 or
  OSC 777) show it themselves — a click brings the right window forward.
  Nothing else pops up, so you never get two.
- **Other terminals:** Linux and BSD — `notify-send` in a desktop session;
  macOS — `osascript`; Windows — a toast through the built-in Windows
  PowerShell. If that fails, the terminal sequence is sent instead.
- **Over SSH** nothing is started on the remote machine: the notification
  travels through the terminal to your own computer.
- **tmux and screen** pass only the bell (tmux marks the window). For focus
  reports in tmux: `set -g focus-events on`.

The bell rings in every mode but `off`: most terminals mark the tab or the
taskbar entry with it.

## When exactly

- **The terminal reports focus** (most modern terminals do): you are notified
  only while its window is in the background, and at once.
- **It never reported focus** (and on Windows, where the console does not
  pass focus on): a turn that ran 30 s or more since your last keystroke, and
  a question left unanswered for 15 s, are announced anyway.
- One question — one notification. A turn you stopped yourself is not
  announced.
- **Background work** — `/loop` and the wake-ups the agent schedules itself —
  is not announced turn by turn: you put it there. You hear about a question
  inside it, an error or the step limit (once, until a turn goes through
  cleanly), and its end: a wake-up that schedules no next one, or a loop the
  plan check stopped.

## Where

The terminal chat (`execai`, `execai chat`). The editor panel notifies by
itself (setting `execai.notifications`): inside the editor while its window
is in front, and with a desktop notification shown by the agent while the
window is in the background — on your own computer; in a remote session
(SSH, WSL, a container) the panel keeps its in-editor notification.
`execai ui` notifies through the browser (the 🔔 button in the page header
asks for permission). `execai run` and `execai serve` stay silent.

## This computer only

The setting lives in `config.json`, but sync leaves it out: every computer
keeps its own — a laptop over SSH and a desktop with a big screen want
different things.

Something does not show up? `EXECAI_NOTIFY_LOG=/tmp/execai-notify.log execai`
writes what was sent and why a desktop notification failed (without the
texts).
