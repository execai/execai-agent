package uiserve

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
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
	// busy — the chats whose turn runs; asks — the permission requests and
	// questions that wait for an answer. Read off the agent's events, for
	// the one case where the server speaks to the agent by itself: an alarm
	// stops the turns and refuses the requests (Server.halt).
	busy map[string]bool
	asks map[string]bool
}

type client struct {
	ch chan []byte // closed by the hub when the client is dropped
}

func newHub() *hub {
	return &hub{snaps: map[string][]byte{}, clients: map[*client]struct{}{},
		busy: map[string]bool{}, asks: map[string]bool{}}
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
		Type  string `json:"type"`
		Chat  string `json:"chat"`
		ID    string `json:"id"`
		AskID string `json:"ask_id"`
	}
	if json.Unmarshal(line, &head) == nil {
		for _, t := range snapshotTypes {
			if head.Type == t {
				h.snaps[t] = line
				break
			}
		}
		h.noteLocked(head.Type, head.Chat, head.ID, head.AskID, line)
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

// noteLocked keeps busy and asks current; under h.mu.
func (h *hub) noteLocked(typ, chat, id, askID string, line []byte) {
	switch typ {
	case "turn_start", "queued":
		// queued — the turn waits for its place among the running ones: it
		// has begun, and a stop ends it there.
		h.busy[chat] = true
	case "done":
		delete(h.busy, chat)
	case "chats":
		// The list of chats says which of them are busy — a turn that began
		// before this process had anybody to tell (a wake-up, a task from
		// the site).
		var list struct {
			Chats []struct {
				ID   string `json:"id"`
				Busy bool   `json:"busy"`
			} `json:"chats"`
		}
		if json.Unmarshal(line, &list) == nil {
			for _, c := range list.Chats {
				if c.Busy {
					h.busy[c.ID] = true
				} else {
					delete(h.busy, c.ID)
				}
			}
		}
	case "ask", "ask_user":
		if id != "" {
			h.asks[id] = true
		}
	case "ask_resolved":
		delete(h.asks, askID)
	}
}

// answered — the page answered a request: it waits no more.
func (h *hub) answered(id string) {
	h.mu.Lock()
	delete(h.asks, id)
	h.mu.Unlock()
}

// pending — the chats whose turn runs and the requests that wait, sorted.
func (h *hub) pending() (chats, asks []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.busy {
		chats = append(chats, c)
	}
	for a := range h.asks {
		asks = append(asks, a)
	}
	sort.Strings(chats)
	sort.Strings(asks)
	return chats, asks
}

// dropAll ends every stream that is open now; new ones may come.
func (h *hub) dropAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		h.dropLocked(c)
	}
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

// Listening — a tab is connected, so an event written now reaches somebody.
// Events that are steps rather than snapshots are not replayed: what is
// written while nobody listens is lost, and the agent may ask first (the
// sync lines for the page do, ide.SyncNotices).
func (h *hub) Listening() bool { return h.count() > 0 }

// count — connected tabs.
func (h *hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}
