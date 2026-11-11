package ide

import (
	"context"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// SyncNotices carries what the background sync says to the person — a
// copy kept next to a file, a file left out, the server's version taken —
// to the panel, as notice events without a chat. In execai ide the stdout is
// the protocol: sync must not print there itself.
//
// Sync says these lines from its own goroutine, at any time, and its first
// exchange runs before the protocol is up. So the lines wait in a queue:
//   - nothing goes out before ready, and the lines said before the panel
//     came up go out after the answer to its first command (resume_last,
//     load_chat, new_chat) — the panel takes a notice without a chat that
//     comes while it waits for resume_last for the answer to it — or
//     syncNoticeDelay after ready if no such command came;
//   - after that each line goes out as it comes;
//   - Notify never blocks: a panel that does not read stops only the
//     goroutine that writes; past the limit the oldest lines go, and the
//     panel is told how many (sync.notices.dropped);
//   - where the panel may be absent (execai ui: the page is opened later,
//     or closed for a while) the lines also wait until somebody listens
//     (WhileListening): an event sent to no page is lost, and a page that
//     connects sends its first command, which lets them out after its
//     answer.
//
// Writing goes through server.emit only (under its mutex).
type SyncNotices struct {
	mu        sync.Mutex
	queue     []string
	dropped   int
	limit     int
	open      bool
	listening func() bool
	wake      chan struct{}
}

// syncNoticeDelay — how long after ready the lines said before it wait for
// the panel's first command.
var syncNoticeDelay = 5 * time.Second

// defaultSyncNoticeLimit — lines kept while nobody takes them.
const defaultSyncNoticeLimit = 50

// NewSyncNotices — limit lines are kept while they wait (0 — the default).
func NewSyncNotices(limit int) *SyncNotices {
	if limit <= 0 {
		limit = defaultSyncNoticeLimit
	}
	return &SyncNotices{limit: limit, wake: make(chan struct{}, 1)}
}

// WhileListening makes the lines wait while listening reports false — no
// page is connected to take them. nil (the default) — somebody always
// listens, as an editor does.
func (n *SyncNotices) WhileListening(listening func() bool) {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.listening = listening
	n.mu.Unlock()
}

// Notify takes one line from sync. It never blocks.
func (n *SyncNotices) Notify(text string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.queue = append(n.queue, text)
	if over := len(n.queue) - n.limit; over > 0 {
		n.queue = n.queue[over:]
		n.dropped += over
	}
	n.mu.Unlock()
	n.signal()
}

func (n *SyncNotices) signal() {
	select {
	case n.wake <- struct{}{}:
	default:
	}
}

// release lets the lines go out from now on (idempotent).
func (n *SyncNotices) release() {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.open = true
	n.mu.Unlock()
	n.signal()
}

// take — what may go out now: the line about dropped ones first.
func (n *SyncNotices) take() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.open || (len(n.queue) == 0 && n.dropped == 0) {
		return nil
	}
	if n.listening != nil && !n.listening() {
		return nil
	}
	var out []string
	if n.dropped > 0 {
		out = append(out, i18n.Tf("sync.notices.dropped", n.dropped))
		n.dropped = 0
	}
	out = append(out, n.queue...)
	n.queue = nil
	return out
}

// pump writes the lines through emit until ctx ends; started by Run.
func (n *SyncNotices) pump(ctx context.Context, emit func(Out)) {
	if n == nil {
		return
	}
	for {
		for _, line := range n.take() {
			emit(Out{Type: "notice", Text: line})
		}
		select {
		case <-ctx.Done():
			return
		case <-n.wake:
		}
	}
}
