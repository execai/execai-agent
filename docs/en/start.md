# Getting started, "My machine" and calls from the phone

Three chat commands show what is already set up on this computer and what
is left: `/start` — the five getting-started steps, `/machine` — the
"My machine" card, `/phone` — a QR code to call this machine from your
phone.

## `/start` — five steps

On the first launch the chat shows these steps instead of the greeting
(before the sign-in link when you are not signed in yet). Later `/start`
shows them again. Each step is marked by the real state of the machine:
✓ — done, ○ — not yet; on top — "N of 5 done".

| Step | What to type | Marked done when |
|---|---|---|
| 1. Sign in to ExecAI | `/login`, then confirm in the browser | the agent is signed in |
| 2. Connect models | `/connect <provider>` — your own key, `/plan` — the ExecAI plan | at least one model source answers |
| 3. Turn on sync | `/sync on` («Work» and «Max» plans) | sync is on and an exchange has happened |
| 4. Bind a folder to a web project | `/project publish` — make a project from this folder, `/project bind <name>` — bind to an existing one | the machine has a bound folder |
| 5. Call the agent from your phone | `/phone`; to take tasks — `execai serve` | `/phone` was shown or the first task from the web chat arrived |

The sync step says right away what travels (the config with the provider
keys, your personal memory, the files of bound projects — all encrypted on
this computer with your sync password), what stays on the machine
(`api_base` and the sign-in), and that only you know the sync password: it
cannot be recovered. More — [sync](sync.md).

## `/machine` — the "My machine" card

- **Machine:** name, OS, agent version.
- **Account:** e-mail (partly masked) and the ExecAI plan — or "not signed
  in — /login".
- **Models:** every connected source — answers, did not answer, or not
  checked yet, with the time of the check; the active one is marked.
- **Sync:** off or on, the time of the last upload, what is inside (config
  and provider keys, personal memory — number of files, projects — number)
  and the mode.
- **Reachable:** whether the machine takes tasks (`execai serve` or an open
  editor window) and whether it can be called from the web chat and the
  phone. If not — why: not signed in, needs the «Work» or «Max» plan,
  `execai serve` is not running.

`/machine check` asks every source for its model list again (as `/connect`
does) and refreshes the plan — so you see which source really answers.

## `/phone` — call the machine from your phone

The agent draws a QR code (white on black, so it scans on light terminal
themes too) and prints the same link as text:
`<web chat address>/m/<machine id>`, for example
`https://chat.execai.ru/m/...`. Point the phone camera at it — the web chat
opens with this machine chosen for tasks.

For a task to arrive:

- sign in to ExecAI — without it there is no link, the agent asks for
  `/login`;
- the «Work» or «Max» plan;
- `execai serve` running on the machine (in a separate window) or an open
  editor window with the project.

Tasks from the phone work on your own provider keys too.

## In the editor panel and in the browser

In the panel (VS Code, Cursor, ExecAI Studio) and in `execai ui` the same
steps and card are in the menu: **Getting started** and **My machine**; the
QR code is behind a button.

For your own integrations: `execai ide` answers the command
`{"type":"command","name":"machine_state"}` with the `machine_state` event
carrying `machine` (the card), `tutorial` (the steps with their marks) and
`phone_url`. The agent also sends this event by itself after sign-in and
sign-out, connecting and disconnecting a source, a change of folder
bindings, and when sync changes or `execai serve` starts or stops.

## Where this is kept

The step 5 marks (`onboarding.json`) and the `/machine check` results
(`source_checks.json`) live in the agent's config directory and are not
sent by sync.
