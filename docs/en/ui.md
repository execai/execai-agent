# The agent panel in a browser — `execai ui`

- **What it is.** The same chat panel as in VS Code and ExecAI Studio, in a browser tab: chats, permissions, models and sources, tasks from the site. No editor needed — a browser on this computer is enough.
- **Run.** `execai ui` in the project directory (or `execai ui --cwd DIR`): it prints the address and opens the browser; `--no-open` only prints it, `--port N` picks the port. Stop: Ctrl+C — closing the tab does not stop the agent.
- **Security.** It listens on 127.0.0.1 only. The address carries a one-time 32-byte key that the first visit swaps for an HttpOnly SameSite=Strict cookie, so the link from the terminal opens nothing a second time. Requests from other sites and other host names are refused, the page can reach nothing but this agent, and your ExecAI login never reaches the page.
- **Getting started and My machine.** The panel menu has the five getting-started steps with their marks and the machine card (models, sync, taking tasks) with a QR code for the phone: the same as `/start`, `/machine` and `/phone` in the chat — [more](start.md).
- **Files** you attach in the browser are saved in `.execai/uploads/` of the project; project sync does not send them.
- **Over SSH:** `ssh -L 8765:127.0.0.1:8765 host`, run `execai ui --port 8765 --no-open` there and open the printed link here.
