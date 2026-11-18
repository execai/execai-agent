// Desktop notifications on behalf of the editor panel (E105).
//
// When the editor window is in the background, a toast inside it is seen
// by nobody. The panel then asks the agent — which runs on the same
// machine — to show a desktop notification:
//
//	{"type":"command","name":"notify","value":"turn_done"}
//	{"type":"command","name":"notify","value":"approval","tool":"Bash"}
//	{"type":"command","name":"notify","value":"ask"}
//
// Only these three kinds exist, and the text is built here from the agent's
// own strings: the working folder's name, the tool's name. Whoever writes to
// stdin cannot put text of their own on the screen, let alone into a
// command line — a tool name that is not a plain name is dropped. The
// command never answers on the protocol (an older agent answers "unknown
// command", which the panel swallows), and a burst of requests is cut.
//
// ready carries features: ["notify"] only when this machine can show one
// (a local desktop session with the program) and the notifications setting
// is not off; otherwise the panel keeps its toasts.
package ide

import (
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/notify"
)

// featureNotify — the ready feature: the agent shows desktop notifications.
const featureNotify = "notify"

// desktopEnv — where the desktop program is looked for; tests replace it.
var desktopEnv = notify.System

// desktopBurst notifications at most within desktopWindow.
const (
	desktopBurst  = 4
	desktopWindow = 10 * time.Second
)

// toolNameRE — what a tool name may look like in a notification.
var toolNameRE = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

// readyFeatures — what the ready event offers on this machine.
func readyFeatures(cfg *config.Config) []string {
	if cfg.GetNotifications() == notify.ModeOff || notify.Desktop(desktopEnv()).Tool == "" {
		return nil
	}
	return []string{featureNotify}
}

// desktop shows the panel's desktop notifications.
type desktop struct {
	n    *notify.Notifier
	mu   sync.Mutex
	sent []time.Time
}

func newDesktop(cfg *config.Config) *desktop {
	if len(readyFeatures(cfg)) == 0 {
		return nil
	}
	return &desktop{n: notify.NewDesktop(desktopEnv())}
}

// allow — false when the burst limit is reached.
func (d *desktop) allow(now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	keep := d.sent[:0]
	for _, t := range d.sent {
		if now.Sub(t) < desktopWindow {
			keep = append(keep, t)
		}
	}
	d.sent = keep
	if len(d.sent) >= desktopBurst {
		return false
	}
	d.sent = append(d.sent, now)
	return true
}

// notifyBody — the text for a kind; "" for an unknown kind.
func notifyBody(kind, tool, cwd string) string {
	switch kind {
	case "turn_done":
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		return i18n.Tf("notify.turnDone", notify.FolderName(cwd))
	case "approval":
		if toolNameRE.MatchString(tool) {
			return i18n.Tf("notify.approve", tool)
		}
		return i18n.T("notify.approveAny")
	case "ask":
		return i18n.T("notify.ask")
	}
	return ""
}

// desktopNotify — the notify command. Silent on the protocol whatever
// happens; the program runs off the command loop.
func (s *server) desktopNotify(in In) {
	d := s.desktop
	if d == nil {
		return
	}
	body := notifyBody(in.Value, in.Tool, s.cwd)
	if body == "" || !d.allow(time.Now()) {
		return
	}
	go d.n.Send(notify.Message{Title: "ExecAI", Body: body})
}
