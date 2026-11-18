package machine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	gosync "sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/connect"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// ChecksFile — the latest /machine check of each own source. Local to this
// machine: whether a provider answers here says nothing about another one.
const ChecksFile = "source_checks.json"

// Check — how a source answered the latest check.
type Check struct {
	OK bool `json:"ok"`
	// Status — the HTTP status of the model-list request (0 = no answer).
	Status int       `json:"status"`
	At     time.Time `json:"at"`
}

// Checks by source id.
type Checks map[string]Check

var checksMu gosync.Mutex

func checksPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ChecksFile), nil
}

// LoadChecks reads the saved checks; a missing or broken file means none.
func LoadChecks() Checks {
	out := Checks{}
	p, err := checksPath()
	if err != nil {
		return out
	}
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	if out == nil {
		out = Checks{}
	}
	return out
}

// CheckSources asks every own source for its model list — the request
// /connect makes — all at once, and saves the answers. A source that needs
// a key and has none is not asked and counts as not answering.
func CheckSources(ctx context.Context, subs *subscriptions.Store) Checks {
	checksMu.Lock()
	defer checksMu.Unlock()
	out := LoadChecks()
	var mu gosync.Mutex
	var wg gosync.WaitGroup
	for _, sub := range subs.List() {
		if sub.APIKey == "" && connect.NeedsKey(sub) {
			mu.Lock()
			out[sub.Provider] = Check{At: time.Now().UTC()}
			mu.Unlock()
			continue
		}
		if sub.BaseURL == "" {
			sub.BaseURL = connect.DefaultBaseURL(sub.Provider)
		}
		wg.Add(1)
		go func(sub subscriptions.Subscription) {
			defer wg.Done()
			_, status := connect.Fetch(sub)
			mu.Lock()
			out[sub.Provider] = Check{OK: status == 200, Status: status, At: time.Now().UTC()}
			mu.Unlock()
		}(sub)
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-ctx.Done():
		// The requests carry their own timeouts; whatever answered is kept.
	}
	mu.Lock()
	snapshot := Checks{}
	for k, v := range out {
		snapshot[k] = v
	}
	mu.Unlock()
	if p, err := checksPath(); err == nil {
		if data, err := json.MarshalIndent(snapshot, "", "  "); err == nil {
			_ = os.WriteFile(p, data, 0o600)
		}
	}
	return snapshot
}
