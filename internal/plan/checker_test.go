package plan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// billing is a fake GET /billing-vbai/my_plan that counts its calls.
type billing struct {
	srv    *httptest.Server
	calls  atomic.Int32
	status atomic.Int32
	body   atomic.Value
}

func newBilling(t *testing.T, status int, body string) *billing {
	b := &billing{}
	b.status.Store(int32(status))
	b.body.Store(body)
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.calls.Add(1)
		if r.URL.Path != "/billing-vbai/my_plan" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(int(b.status.Load()))
		_, _ = w.Write([]byte(b.body.Load().(string)))
	}))
	t.Cleanup(b.srv.Close)
	return b
}

func checkerAt(url, path, token string, now time.Time) *Checker {
	c := NewChecker(url, path, func() string { return token })
	c.now = func() time.Time { return now }
	return c
}

func writeCache(t *testing.T, path string, p Plan) {
	t.Helper()
	data, _ := json.Marshal(p)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestChecker_ActivePlanSavedWith0600(t *testing.T) {
	b := newBilling(t, 200, bodyActive)
	path := filepath.Join(t.TempDir(), "plan.json")
	c := checkerAt(b.srv.URL, path, "tok", t0)

	v := c.Check(context.Background(), Route{})
	if !v.Allowed || v.Reason != ReasonActive {
		t.Fatalf("active plan refused: %+v", v)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("cache not written: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("cache mode %v, want 0600", st.Mode().Perm())
	}
	// A fresh answer is not re-asked on the next call.
	c.Check(context.Background(), Route{})
	if n := b.calls.Load(); n != 1 {
		t.Errorf("billing called %d times, want 1", n)
	}
}

func TestChecker_NoPlanRefused(t *testing.T) {
	b := newBilling(t, 200, bodyNone)
	c := checkerAt(b.srv.URL, "", "tok", t0)
	if v := c.Check(context.Background(), Route{}); v.Allowed || v.Reason != ReasonNoPlan {
		t.Errorf("no subscription: %+v", v)
	}
}

func TestChecker_401MeansSignedOut(t *testing.T) {
	b := newBilling(t, 200, bodyActive)
	c := checkerAt(b.srv.URL, "", "stale-token", t0)
	v := c.Check(context.Background(), Route{})
	if v.Allowed || v.Reason != ReasonNotLoggedIn {
		t.Errorf("401: %+v", v)
	}
	if p := c.Current(); p == nil || !p.LoggedOut {
		t.Errorf("401 not remembered: %+v", p)
	}
}

func TestChecker_NoTokenNoNetwork(t *testing.T) {
	b := newBilling(t, 200, bodyActive)
	c := checkerAt(b.srv.URL, "", "", t0)
	if v := c.Check(context.Background(), Route{}); v.Allowed || v.Reason != ReasonNotLoggedIn {
		t.Errorf("no token: %+v", v)
	}
	if b.calls.Load() != 0 {
		t.Error("billing asked without a token")
	}
}

func TestChecker_OwnSourceNoNetwork(t *testing.T) {
	b := newBilling(t, 200, bodyNone)
	for _, token := range []string{"tok", ""} {
		c := checkerAt(b.srv.URL, "", token, t0)
		if v := c.Check(context.Background(), Route{Own: true}); !v.Allowed || v.Reason != ReasonOwnSource {
			t.Errorf("own source, token %q: %+v", token, v)
		}
	}
	if b.calls.Load() != 0 {
		t.Error("billing asked for an own source")
	}
}

// Billing down (5xx or no connection): the saved answer stands in for up
// to OfflineGrace from when it was given.
func TestChecker_OfflineGrace(t *testing.T) {
	down := newBilling(t, 503, `{"error":"unavailable"}`)
	refused := httptest.NewServer(http.NotFoundHandler())
	refusedURL := refused.URL
	refused.Close() // connection refused from now on

	for _, srv := range []struct{ name, url string }{{"5xx", down.srv.URL}, {"no connection", refusedURL}} {
		for _, age := range []struct {
			days    int
			allowed bool
			reason  Reason
		}{{6, true, ReasonActive}, {8, false, ReasonUnverified}} {
			t.Run(srv.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "plan.json")
				p, _ := Parse([]byte(bodyActive), t0)
				p.TokenFP = fingerprint("tok")
				writeCache(t, path, p)
				c := checkerAt(srv.url, path, "tok", t0.Add(time.Duration(age.days)*24*time.Hour))
				v := c.Check(context.Background(), Route{})
				if v.Allowed != age.allowed || v.Reason != age.reason {
					t.Errorf("cache %d days old: %+v, want allowed=%v reason=%v",
						age.days, v, age.allowed, age.reason)
				}
				if c.LastError() == nil {
					t.Error("failed refresh not reported")
				}
			})
		}
	}
}

func TestChecker_NoCacheOfflineRefused(t *testing.T) {
	b := newBilling(t, 502, "bad gateway")
	c := checkerAt(b.srv.URL, filepath.Join(t.TempDir(), "plan.json"), "tok", t0)
	if v := c.Check(context.Background(), Route{}); v.Allowed || v.Reason != ReasonUnverified {
		t.Errorf("offline, never verified: %+v", v)
	}
}

func TestChecker_StaleOrForeignCacheRefreshed(t *testing.T) {
	cases := []struct {
		name  string
		age   time.Duration
		fp    string
		calls int32
	}{
		{"fresh, same token", time.Hour, fingerprint("tok"), 0},
		{"older than a day", 25 * time.Hour, fingerprint("tok"), 1},
		{"another token", time.Hour, fingerprint("old-token"), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := newBilling(t, 200, bodyNone)
			path := filepath.Join(t.TempDir(), "plan.json")
			p, _ := Parse([]byte(bodyActive), t0)
			p.TokenFP = c.fp
			writeCache(t, path, p)
			ch := checkerAt(b.srv.URL, path, "tok", t0.Add(c.age))
			v := ch.Check(context.Background(), Route{})
			if got := b.calls.Load(); got != c.calls {
				t.Errorf("billing called %d times, want %d", got, c.calls)
			}
			wantAllowed := c.calls == 0 // a refresh brings "no plan"
			if v.Allowed != wantAllowed {
				t.Errorf("verdict %+v, want allowed=%v", v, wantAllowed)
			}
		})
	}
}

// Paid on the site a minute ago: the saved "no plan" must not keep refusing.
func TestChecker_RefusalRecheckedAfterPayment(t *testing.T) {
	b := newBilling(t, 200, bodyActive)
	path := filepath.Join(t.TempDir(), "plan.json")
	p, _ := Parse([]byte(bodyNone), t0)
	p.TokenFP = fingerprint("tok")
	writeCache(t, path, p)
	c := checkerAt(b.srv.URL, path, "tok", t0.Add(time.Minute))
	if v := c.Check(context.Background(), Route{}); !v.Allowed {
		t.Errorf("fresh payment not noticed: %+v", v)
	}
}

// While billing keeps refusing, it is asked at most once per RefusalRecheck.
func TestChecker_RefusalRecheckIsRateLimited(t *testing.T) {
	b := newBilling(t, 200, bodyNone)
	now := t0
	c := NewChecker(b.srv.URL, "", func() string { return "tok" })
	c.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		c.Check(context.Background(), Route{})
	}
	if n := b.calls.Load(); n != 1 {
		t.Errorf("billing called %d times within the gap, want 1", n)
	}
	now = now.Add(RefusalRecheck + time.Second)
	c.Check(context.Background(), Route{})
	if n := b.calls.Load(); n != 2 {
		t.Errorf("billing called %d times after the gap, want 2", n)
	}
}

// The fallback is checked like the ExecAI source it really is.
func TestChecker_FallbackRouteIsChecked(t *testing.T) {
	b := newBilling(t, 200, bodyNone)
	c := checkerAt(b.srv.URL, "", "tok", t0)
	v := c.Check(context.Background(), Route{Fallback: "codex-cli"})
	if v.Allowed || v.Fallback != "codex-cli" || b.calls.Load() != 1 {
		t.Errorf("fallback: %+v, billing calls %d", v, b.calls.Load())
	}
}

// Background refreshes ask billing only while the ExecAI models are in use:
// my_plan may renew (charge) an expired period, so nobody else triggers it.
func TestChecker_StartOnlyWhenExecAI(t *testing.T) {
	b := newBilling(t, 200, bodyActive)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	NewChecker(b.srv.URL, "", func() string { return "tok" }).Start(ctx, func() bool { return false })
	time.Sleep(200 * time.Millisecond)
	if n := b.calls.Load(); n != 0 {
		t.Fatalf("billing asked %d times while an own source is active", n)
	}
	NewChecker(b.srv.URL, "", func() string { return "tok" }).Start(ctx, func() bool { return true })
	deadline := time.Now().Add(3 * time.Second)
	for b.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if b.calls.Load() != 1 {
		t.Errorf("billing asked %d times on the ExecAI source, want 1", b.calls.Load())
	}
}
