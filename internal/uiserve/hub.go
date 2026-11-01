package uiserve

import (
	"bytes"
	"encoding/json"
	"errors"
	"sync"
)

// snapshotTypes — events that describe the whole current state rather than a
// step of a turn. The last one of each is replayed to a tab that connects (or
// reconnects), in this order, so a fresh tab does not start blank.
var snapshotTypes = []string{"ready", "state", "inbox", "chats", "agents"}

// clientBuffer — how many events a tab may lag behind before it is dropped.
// A dropped tab's EventSource reconnects and gets the snapshots again; a turn
// in progress is replayed by load_chat, as in the editor.
const clientBuffer = 1024

// maxClients — open event streams at once; a limit, not a feature.
const maxClients = 32

// maxPartial — a line without its newline is kept up to this size; the
// encoder always ends a line, so more means garbage.
const maxPartial = 16 << 20

var (
	errHubClosed  = errors.New("uiserve: shutting down")
	errTooManyTab = errors.New("uiserve: too many open tabs")
)

// hub is the io.Writer the ide server writes its protocol to: it splits the
// stream into lines (one Out event each), remembers the last snapshot of each
// kind and fans every line out to the connected tabs. Writes never block the
// agent: a tab that cannot keep up is disconnected instead.
type hub struct {
	mu      sync.Mutex
	partial []byte
	snaps   map[string][]byte
	clients map[*client]struct{}
	closed  bool
}

type client struct {
	ch chan []byte // closed by the hub when the client is dropped
}

func newHub() *hub {
	return &hub{snaps: map[string][]byte{}, clients: map[*client]struct{}{}}
}

// Write implements io.Writer for json.Encoder: complete lines are published,
// a trailing partial line waits for its newline.
func (h *hub) Write(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	data := p
	if len(h.partial) > 0 {
		data = append(h.partial, p...)
		h.partial = nil
	}
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		h.publishLocked(data[:i])
		data = data[i+1:]
	}
	if len(data) > 0 && len(data) <= maxPartial {
		h.partial = append([]byte(nil), data...)
	}
	return len(p), nil
}

// publishLocked — under h.mu. The line is copied once and shared read-only by
// all clients.
func (h *hub) publishLocked(raw []byte) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return
	}
	line := append([]byte(nil), raw...)
	var head struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(line, &head) == nil {
		for _, t := range snapshotTypes {
			if head.Type == t {
				h.snaps[t] = line
				break
			}
		}
	}
	if h.closed {
		return
	}
	for c := range h.clients {
		select {
		case c.ch <- line:
		default:
			h.dropLocked(c)
		}
	}
}

func (h *hub) dropLocked(c *client) {
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.ch)
	}
}

// subscribe registers a tab and returns the snapshots to send first. Both
// happen under one lock, so no event falls between the replay and the live
// stream, and none comes twice.
func (h *hub) subscribe() (*client, [][]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, nil, errHubClosed
	}
	if len(h.clients) >= maxClients {
		return nil, nil, errTooManyTab
	}
	var replay [][]byte
	for _, t := range snapshotTypes {
		if l := h.snaps[t]; l != nil {
			replay = append(replay, l)
		}
	}
	c := &client{ch: make(chan []byte, clientBuffer)}
	h.clients[c] = struct{}{}
	return c, replay, nil
}

func (h *hub) unsubscribe(c *client) {
	h.mu.Lock()
	h.dropLocked(c)
	h.mu.Unlock()
}

// close ends every stream; later writes are still accepted and dropped, so
// the agent shutting down never blocks on its output.
func (h *hub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for c := range h.clients {
		h.dropLocked(c)
	}
}

// count — connected tabs (tests).
func (h *hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}
