package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
)

// Report timing and size, shared with agents-vbai: the server keeps a report
// for five minutes (two missed reports), and refuses bodies over 64 KB.
const (
	ReportInterval = 30 * time.Second
	MaxReportBody  = 64 << 10
	sendTimeout    = 15 * time.Second
	finalTimeout   = 3 * time.Second
)

// SendFunc delivers one report body (JSON {agents, pid, cwd}).
type SendFunc func(ctx context.Context, body []byte) error

// ErrNoToken — the process is not signed in to ExecAI; there is nobody to
// report to, and that is not an error worth showing.
var ErrNoToken = errors.New("fleet: not signed in")

// HTTPSender sends reports to agents-vbai (PUT /agents-vbai/presence/fleet)
// with the agent's JWT. token is asked on every send: the person may sign in
// or out while the process runs.
func HTTPSender(apiBase string, token func() string) SendFunc {
	return func(ctx context.Context, body []byte) error {
		tok := ""
		if token != nil {
			tok = token()
		}
		if tok == "" {
			return ErrNoToken
		}
		return inbox.NewClient(apiBase, tok).PutFleet(ctx, body)
	}
}

// SavedToken reads the ExecAI token from credentials.json on every call: the
// reporter runs beside the UI, and a sign-in or sign-out there must reach it
// without sharing the UI's state across goroutines.
func SavedToken() string {
	cr, err := config.LoadCredentials()
	if err != nil || cr == nil {
		return ""
	}
	return cr.Token
}

// ReportPayload is the body of PUT /presence/fleet.
type ReportPayload struct {
	Agents []Record `json:"agents"`
	PID    int      `json:"pid"`
	Cwd    string   `json:"cwd"`
}

// ForReport prepares records for the server: no results (they stay on this
// machine), previews within the limit.
func ForReport(recs []Record) []Record {
	out := make([]Record, len(recs))
	for i, r := range recs {
		r.Result = ""
		r.Preview = Tail(r.Preview, PreviewLimit)
		out[i] = r
	}
	return out
}

// BuildReport marshals the payload, dropping the oldest finished records
// (then the oldest of any) until it fits into MaxReportBody.
func BuildReport(recs []Record, pid int, cwd string) ([]byte, error) {
	agents := ForReport(recs)
	envelope, err := json.Marshal(ReportPayload{Agents: []Record{}, PID: pid, Cwd: cwd})
	if err != nil {
		return nil, err
	}
	sizes := make([]int, len(agents))
	total := len(envelope)
	for i, r := range agents {
		b, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		sizes[i] = len(b) + 1 // and a comma
		total += sizes[i]
	}
	keep := make([]bool, len(agents))
	for i := range keep {
		keep[i] = true
	}
	drop := func(active bool) {
		for i, r := range agents {
			if total <= MaxReportBody {
				return
			}
			if keep[i] && r.State.Active() == active {
				keep[i] = false
				total -= sizes[i]
			}
		}
	}
	drop(false)
	drop(true)
	kept := make([]Record, 0, len(agents))
	for i, r := range agents {
		if keep[i] {
			kept = append(kept, r)
		}
	}
	return json.Marshal(ReportPayload{Agents: kept, PID: pid, Cwd: cwd})
}

// Reporter tells the server what this process's subagents are doing: once
// when work starts, every Interval while any subagent is active, once when
// the last one finishes, and once more on Close if anything changed since.
// A process without subagents sends nothing at all.
type Reporter struct {
	fleet    *Fleet
	send     SendFunc
	cwd      string
	pid      int
	interval time.Duration

	changes chan struct{}
	done    chan struct{}
	stopped chan struct{}
	once    sync.Once

	mu       sync.Mutex
	sentVer  uint64
	sentAny  bool
	lastErr  error
	inflight sync.Mutex // one send at a time
}

// StartReporter starts reporting f. interval 0 means ReportInterval.
func StartReporter(f *Fleet, send SendFunc, cwd string, interval time.Duration) *Reporter {
	if interval <= 0 {
		interval = ReportInterval
	}
	r := &Reporter{
		fleet: f, send: send, cwd: cwd, pid: os.Getpid(), interval: interval,
		changes: make(chan struct{}, 1),
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	unsub := f.Subscribe(func([]Record) {
		select {
		case r.changes <- struct{}{}:
		default:
		}
	})
	go func() {
		defer close(r.stopped)
		defer unsub()
		r.loop()
	}()
	return r
}

func (r *Reporter) loop() {
	var tick <-chan time.Time
	var ticker *time.Ticker
	stopTicker := func() {
		if ticker != nil {
			ticker.Stop()
			ticker, tick = nil, nil
		}
	}
	defer stopTicker()
	wasActive := false
	for {
		select {
		case <-r.done:
			return
		case <-r.changes:
			active := r.fleet.HasActive()
			switch {
			case active && !wasActive:
				r.sendNow(sendTimeout)
				ticker = time.NewTicker(r.interval)
				tick = ticker.C
			case !active && (wasActive || r.unsent()):
				// The final state of this batch — also for a subagent that
				// started and finished between two deliveries.
				r.sendNow(sendTimeout)
				stopTicker()
			}
			wasActive = active
		case <-tick:
			r.sendNow(sendTimeout)
		}
	}
}

// sendNow sends the current snapshot.
func (r *Reporter) sendNow(timeout time.Duration) {
	r.inflight.Lock()
	defer r.inflight.Unlock()
	ver := r.fleet.Version()
	recs := r.fleet.Snapshot()
	if len(recs) == 0 {
		return
	}
	body, err := BuildReport(recs, r.pid, r.cwd)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		err = r.send(ctx, body)
		cancel()
	}
	r.mu.Lock()
	r.lastErr = err
	if err == nil {
		r.sentVer, r.sentAny = ver, true
	}
	r.mu.Unlock()
}

// unsent reports whether the registry changed since the last report.
func (r *Reporter) unsent() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.sentAny || r.sentVer != r.fleet.Version()
}

// LastError — the outcome of the latest send (nil after a success).
func (r *Reporter) LastError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErr
}

// Close stops reporting. If the registry changed since the last report
// (subagents were marked cancelled on exit), it sends one last report first,
// bounded by a short timeout: the process is on its way out.
func (r *Reporter) Close() {
	r.once.Do(func() {
		close(r.done)
		<-r.stopped
		if len(r.fleet.Snapshot()) == 0 {
			return
		}
		if r.unsent() {
			r.sendNow(finalTimeout)
		}
	})
}
