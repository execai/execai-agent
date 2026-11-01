// Package fleet is the registry of "who is doing what": the subagents this
// process runs, their state, last step, tokens and result.
//
// Every surface reads the same records. The terminal draws them above the
// input and lists them with /agents; the editor panel (and `execai ui`) gets
// them as the `agents` event; the server gets them through report.go and
// shows them on the machine's card in the web panel. One shape everywhere —
// the JSON field names are the contract between the agent, the panel, the
// server and the web (see TASKS E101 "Контракт fleet").
//
// Changes are delivered to subscribers as full snapshots, coalesced to at
// most one delivery per MinGap (two a second by default): a subagent
// streaming text changes its preview many times a second, and nobody needs
// to redraw that often.
package fleet

import (
	"sync"
	"time"
)

// State of a subagent.
type State string

const (
	Queued    State = "queued"    // waiting for a free subagent slot
	Running   State = "running"   // working
	Waiting   State = "waiting"   // waiting for the person to answer a permission question
	Done      State = "done"      // finished with a result
	Failed    State = "failed"    // finished with an error (or timed out)
	Cancelled State = "cancelled" // stopped: by the person, with its parent, or on exit
)

// Active reports whether the subagent has not finished yet.
func (s State) Active() bool { return s == Queued || s == Running || s == Waiting }

// PreviewLimit — how many characters of the latest text a record carries.
const PreviewLimit = 200

// Record is one subagent. Field names are the shared contract.
type Record struct {
	ID             string     `json:"id"`
	ParentChat     string     `json:"parent_chat"`
	ParentToolCall string     `json:"parent_tool_call"`
	Title          string     `json:"title"`
	State          State      `json:"state"`
	Mode           string     `json:"mode"` // read | write
	Iter           int        `json:"iter"`
	MaxIter        int        `json:"max_iter"`
	LastTool       string     `json:"last_tool"`
	StartedAt      time.Time  `json:"started_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	Preview        string     `json:"preview"`
	// Result — the subagent's final answer. Local only: the report to the
	// server leaves it out (see ForReport).
	Result       string `json:"result,omitempty"`
	Session      string `json:"session"`
	Source       string `json:"source"`
	Model        string `json:"model"`
	PromptTokens int    `json:"prompt_tokens"`
	Error        string `json:"error,omitempty"`
}

// Elapsed — how long the subagent has been (or was) at work.
func (r Record) Elapsed(now time.Time) time.Duration {
	end := now
	if r.FinishedAt != nil {
		end = *r.FinishedAt
	}
	if r.StartedAt.IsZero() || end.Before(r.StartedAt) {
		return 0
	}
	return end.Sub(r.StartedAt)
}

// Tail returns the last n characters of s (whole runes), with a leading
// ellipsis when something was cut.
func Tail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "…" + string(r[len(r)-n+1:])
}

// DefaultMinGap — the minimum time between two deliveries to subscribers.
const DefaultMinGap = 500 * time.Millisecond

// DefaultKeep — how many records the registry keeps; the oldest finished
// ones go first when there are more.
const DefaultKeep = 100

// Fleet is the registry. The zero value is not usable; build it with New.
type Fleet struct {
	// MinGap between deliveries; set before the first Subscribe.
	MinGap time.Duration
	// Keep — the most records kept.
	Keep int

	mu      sync.Mutex
	recs    []*Record
	byID    map[string]*Record
	version uint64

	subMu    sync.Mutex
	subs     map[int]func([]Record)
	nextSub  int
	kick     chan struct{}
	closed   chan struct{}
	loopOnce sync.Once
	stopOnce sync.Once
}

// New returns an empty registry.
func New() *Fleet {
	return &Fleet{
		MinGap: DefaultMinGap,
		Keep:   DefaultKeep,
		byID:   map[string]*Record{},
		subs:   map[int]func([]Record){},
		kick:   make(chan struct{}, 1),
		closed: make(chan struct{}),
	}
}

// Add registers a subagent. StartedAt/UpdatedAt default to now.
func (f *Fleet) Add(r Record) {
	now := time.Now().UTC()
	if r.StartedAt.IsZero() {
		r.StartedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = now
	}
	r.Preview = Tail(r.Preview, PreviewLimit)
	f.mu.Lock()
	if old := f.byID[r.ID]; old != nil {
		*old = r
	} else {
		cp := r
		f.recs = append(f.recs, &cp)
		f.byID[r.ID] = &cp
		f.trimLocked()
	}
	f.version++
	f.mu.Unlock()
	f.notify()
}

// trimLocked drops the oldest finished records beyond Keep.
func (f *Fleet) trimLocked() {
	keep := f.Keep
	if keep <= 0 {
		keep = DefaultKeep
	}
	for len(f.recs) > keep {
		drop := -1
		for i, r := range f.recs {
			if !r.State.Active() {
				drop = i
				break
			}
		}
		if drop < 0 {
			return // everything is still running: keep it all
		}
		delete(f.byID, f.recs[drop].ID)
		f.recs = append(f.recs[:drop], f.recs[drop+1:]...)
	}
}

// Update changes a record in place and stamps UpdatedAt. It reports
// whether the record exists.
func (f *Fleet) Update(id string, fn func(*Record)) bool {
	f.mu.Lock()
	r := f.byID[id]
	if r == nil {
		f.mu.Unlock()
		return false
	}
	fn(r)
	r.Preview = Tail(r.Preview, PreviewLimit)
	r.UpdatedAt = time.Now().UTC()
	f.version++
	f.mu.Unlock()
	f.notify()
	return true
}

// Get returns a copy of the record.
func (f *Fleet) Get(id string) (Record, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.byID[id]
	if r == nil {
		return Record{}, false
	}
	return *r, true
}

// Snapshot returns copies of all records in start order.
func (f *Fleet) Snapshot() []Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Record, 0, len(f.recs))
	for _, r := range f.recs {
		out = append(out, *r)
	}
	return out
}

// Version grows with every change; equal versions mean equal contents.
func (f *Fleet) Version() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.version
}

// HasActive reports whether any subagent is still at work.
func (f *Fleet) HasActive() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.recs {
		if r.State.Active() {
			return true
		}
	}
	return false
}

// Subscribe calls fn with a full snapshot after changes, at most once per
// MinGap. fn runs on the registry's delivery goroutine, one call at a time;
// it must not block for long. The returned func unsubscribes.
func (f *Fleet) Subscribe(fn func([]Record)) func() {
	f.subMu.Lock()
	id := f.nextSub
	f.nextSub++
	f.subs[id] = fn
	f.subMu.Unlock()
	f.loopOnce.Do(func() { go f.deliver() })
	return func() {
		f.subMu.Lock()
		delete(f.subs, id)
		f.subMu.Unlock()
	}
}

// Close stops deliveries. Records stay readable.
func (f *Fleet) Close() {
	f.stopOnce.Do(func() { close(f.closed) })
}

func (f *Fleet) notify() {
	select {
	case f.kick <- struct{}{}:
	default: // a delivery is already due and will see this change
	}
}

func (f *Fleet) deliver() {
	var last time.Time
	for {
		select {
		case <-f.closed:
			return
		case <-f.kick:
		}
		if wait := f.MinGap - time.Since(last); wait > 0 && !last.IsZero() {
			select {
			case <-time.After(wait):
			case <-f.closed:
				return
			}
		}
		// Changes made while waiting are in the snapshot below.
		select {
		case <-f.kick:
		default:
		}
		snap := f.Snapshot()
		last = time.Now()
		f.subMu.Lock()
		fns := make([]func([]Record), 0, len(f.subs))
		for _, fn := range f.subs {
			fns = append(fns, fn)
		}
		f.subMu.Unlock()
		for _, fn := range fns {
			fn(snap)
		}
	}
}

// Counts returns how many of recs are active, done, and failed or cancelled.
func Counts(recs []Record) (active, done, failed int) {
	for _, r := range recs {
		switch {
		case r.State.Active():
			active++
		case r.State == Done:
			done++
		default:
			failed++
		}
	}
	return
}
