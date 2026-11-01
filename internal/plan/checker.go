package plan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/version"
)

const (
	// FetchTimeout bounds one request to billing.
	FetchTimeout = 10 * time.Second
	// CheckTimeout bounds how long a model call may wait for the gate when
	// the saved answer has to be re-asked. A TUI turn runs this in its own
	// goroutine, so the interface never freezes on it.
	CheckTimeout = 5 * time.Second
	// MinRefreshGap — a stale answer is re-asked at most this often, so an
	// outage does not add a network timeout to every model call.
	MinRefreshGap = 10 * time.Minute
	// RefusalRecheck — the gap for a saved refusal: someone who has just paid
	// on the site should not wait ten minutes for the agent to notice.
	RefusalRecheck = 30 * time.Second
)

// StatusError — billing answered with something other than 200 or 401.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("my_plan: HTTP %d: %s", e.Code, e.Body)
}

// Checker keeps the latest answer about the plan and refreshes it. Safe for
// concurrent use. The zero value is not usable — build it with New.
type Checker struct {
	apiBase string
	// token returns the current ExecAI JWT ("" = not signed in). Read on every
	// check: a login in the TUI replaces credentials.json mid-process.
	token func() string
	// path — the cache file; "" keeps the answer in memory only.
	path string
	http *http.Client
	now  func() time.Time

	mu            sync.Mutex
	loaded        bool
	cur           *Plan
	lastAttempt   time.Time
	lastAttemptFP string
	lastErr       error
	inflight      chan struct{}
}

// New builds a Checker for the API base from config, with the token from
// credentials.json and the cache in the config directory.
func New(apiBase string) *Checker {
	path := ""
	if dir, err := config.Dir(); err == nil {
		path = filepath.Join(dir, "plan.json")
	}
	return NewChecker(apiBase, path, func() string {
		cr, err := config.LoadCredentials()
		if err != nil || cr == nil {
			return ""
		}
		return cr.Token
	})
}

// NewChecker builds a Checker with an explicit cache path ("" = memory
// only) and token source. New is the usual constructor; this one exists for
// tests and for callers that hold the token themselves.
func NewChecker(apiBase, path string, token func() string) *Checker {
	return &Checker{
		apiBase: strings.TrimRight(apiBase, "/"),
		token:   token,
		path:    path,
		http:    &http.Client{Timeout: FetchTimeout},
		now:     time.Now,
	}
}

// fingerprint identifies a token without storing it.
func fingerprint(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:8])
}

// load reads the cache once. Caller holds c.mu.
func (c *Checker) load() {
	if c.loaded {
		return
	}
	c.loaded = true
	if c.path == "" {
		return
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil || p.CheckedAt.IsZero() {
		return // a broken cache is the same as no cache
	}
	c.cur = &p
}

// save writes the cache atomically, mode 0600.
func (c *Checker) save(p Plan) {
	if c.path == "" {
		return
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, c.path)
}

// Current returns the latest known answer (from billing or the cache), nil
// when there is none.
func (c *Checker) Current() *Plan {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	if c.cur == nil {
		return nil
	}
	p := *c.cur
	return &p
}

// LastError — the error of the latest refresh, nil when it succeeded.
func (c *Checker) LastError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

// SignedIn reports whether an ExecAI token is present.
func (c *Checker) SignedIn() bool { return c.token() != "" }

// Refresh asks billing now. A 401 is an answer (signed out), not an error.
// Network errors and other statuses keep the previous answer and are
// returned. Concurrent calls share one request.
func (c *Checker) Refresh(ctx context.Context) error {
	token := c.token()
	if token == "" {
		return nil // nothing to ask with; the gate reports "not signed in"
	}
	c.mu.Lock()
	c.load()
	if ch := c.inflight; ch != nil {
		c.mu.Unlock()
		select {
		case <-ch:
			c.mu.Lock()
			defer c.mu.Unlock()
			return c.lastErr
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ch := make(chan struct{})
	c.inflight = ch
	c.lastAttempt = c.now()
	c.lastAttemptFP = fingerprint(token)
	c.mu.Unlock()

	p, err := c.fetch(ctx, token)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.inflight = nil
	close(ch)
	c.lastErr = err
	if err != nil {
		return err
	}
	p.TokenFP = fingerprint(token)
	c.cur = &p
	c.save(p)
	return nil
}

// RefreshAsync re-asks billing in the background — after a 402 from the
// ExecAI backend, when the plan has most likely just ended.
func (c *Checker) RefreshAsync() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), FetchTimeout)
		defer cancel()
		_ = c.Refresh(ctx)
	}()
}

// Start refreshes in the background now and then every RefreshEvery until
// ctx ends, but only while when() says the ExecAI models are in use: my_plan
// may renew (charge) an expired period, so a person on their own keys never
// triggers it. With an own source the plan is asked lazily — on a switch to
// the ExecAI models, on a turn there (Ensure) and on /plan. The caller never
// waits for it: the cache answers meanwhile.
func (c *Checker) Start(ctx context.Context, when func() bool) {
	go func() {
		for {
			if when == nil || when() {
				rctx, cancel := context.WithTimeout(ctx, FetchTimeout)
				_ = c.Refresh(rctx)
				cancel()
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(RefreshEvery):
			}
		}
	}()
}

// Ensure returns the latest answer, first re-asking billing when there is
// none, it belongs to another token, or it is stale — but for the same token
// not more often than MinRefreshGap (RefusalRecheck for a refusal). A
// refresh already in flight (the one started at launch) is waited for.
func (c *Checker) Ensure(ctx context.Context) *Plan {
	token := c.token()
	if token == "" {
		return c.Current()
	}
	fp := fingerprint(token)
	now := c.now()
	c.mu.Lock()
	c.load()
	busy := c.inflight != nil
	need := c.cur == nil || c.cur.TokenFP != fp || c.cur.Stale(now)
	gap := MinRefreshGap
	if c.cur == nil || !c.cur.Granted() {
		gap = RefusalRecheck
	}
	gapOK := c.lastAttemptFP != fp || now.Sub(c.lastAttempt) >= gap
	c.mu.Unlock()
	if busy || (need && gapOK) {
		_ = c.Refresh(ctx)
	}
	return c.Current()
}

// Check is the gate for one model call (r — see RouteOf). An own source is
// let through without touching the network; only a call to the ExecAI models
// (including a CLI fallback) is checked against the plan.
func (c *Checker) Check(ctx context.Context, r Route) Verdict {
	if r.Own {
		return Decide(nil, false, true, c.now())
	}
	var v Verdict
	if !c.SignedIn() {
		v = Decide(nil, false, false, c.now())
	} else {
		ctx, cancel := context.WithTimeout(ctx, CheckTimeout)
		defer cancel()
		v = Decide(c.Ensure(ctx), true, false, c.now())
	}
	v.Fallback = r.Fallback
	return v
}

// fetch performs GET /billing-vbai/my_plan.
func (c *Checker) fetch(ctx context.Context, token string) (Plan, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+"/billing-vbai/my_plan", nil)
	if err != nil {
		return Plan{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", version.UserAgent())
	resp, err := c.http.Do(req)
	if err != nil {
		return Plan{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	now := c.now()
	switch {
	case resp.StatusCode == http.StatusOK:
		return Parse(body, now)
	case resp.StatusCode == http.StatusUnauthorized:
		return Plan{LoggedOut: true, CheckedAt: now}, nil
	default:
		b := strings.TrimSpace(string(body))
		if len(b) > 200 {
			b = b[:200]
		}
		return Plan{}, &StatusError{Code: resp.StatusCode, Body: b}
	}
}
