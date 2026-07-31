package auth

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

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// mockBackend — fake /auth-vbai/agent-link/start + /poll. The response script
// is a sequence of statuses, returned one per poll.
type mockBackend struct {
	startCalls  int32
	pollCalls   int32
	pollStatuses []string // one per call; after the end — stick to the last one
	linkedJWT   string
	failEvery   int32 // if >0 — every Nth poll returns 502 (for the resilience test)
	forceExpiresIn int
	server      *httptest.Server
}

func newMockBackend(pollScript []string, linkedJWT string) *mockBackend {
	m := &mockBackend{pollStatuses: pollScript, linkedJWT: linkedJWT}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth-vbai/agent-link/start", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&m.startCalls, 1)
		exp := m.forceExpiresIn
		if exp == 0 {
			exp = 900
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_code":    "TESTCODE",
			"link_token":   "test-link-token",
			"verify_uri":   "https://mock/agents/connect/TESTCODE",
			"expires_in":   exp,
			"poll_interval": 1, // 1 sec — faster than prod's 3, so tests do not stall
		})
	})
	mux.HandleFunc("/auth-vbai/agent-link/poll", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&m.pollCalls, 1)
		if m.failEvery > 0 && n%m.failEvery == 0 {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		idx := int(n-1)
		if idx >= len(m.pollStatuses) {
			idx = len(m.pollStatuses) - 1
		}
		status := m.pollStatuses[idx]
		resp := map[string]any{"status": status}
		if status == "linked" {
			resp["jwt"] = m.linkedJWT
			resp["agent_id"] = "agent-42"
			resp["alias"] = "TestBot"
			resp["user_email"] = "test@example.com"
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	m.server = httptest.NewServer(mux)
	return m
}

func (m *mockBackend) Close() { m.server.Close() }

// tempConfigDir isolates credentials.json from the user's one.
func tempConfigDir(t *testing.T) func() {
	dir := t.TempDir()
	old := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	// Make sure the execai directory exists.
	_ = os.MkdirAll(filepath.Join(dir, "execai"), 0o700)
	return func() { _ = os.Setenv("XDG_CONFIG_HOME", old) }
}

// Happy path: 2 pending → linked. The client must receive credentials.
func TestWaitLinkUntilLinked_HappyPath(t *testing.T) {
	defer tempConfigDir(t)()
	backend := newMockBackend([]string{"pending", "pending", "linked"}, "test-jwt-value")
	defer backend.Close()

	cfg := &config.Config{APIBase: backend.server.URL}
	start, err := StartAgentLink(context.Background(), cfg.APIBase)
	if err != nil {
		t.Fatal(err)
	}
	if start.UserCode != "TESTCODE" {
		t.Fatalf("ждали TESTCODE, имеем %q", start.UserCode)
	}

	cr, err := WaitLinkUntilLinked(context.Background(), cfg, start, 10*time.Second, nil)
	if err != nil {
		t.Fatalf("polling упал: %v", err)
	}
	if cr.Token != "test-jwt-value" {
		t.Fatalf("токен не тот: %q", cr.Token)
	}
	if cr.Email != "test@example.com" {
		t.Fatalf("email не тот: %q", cr.Email)
	}
	if atomic.LoadInt32(&backend.pollCalls) != 3 {
		t.Fatalf("ждали 3 poll'а, было %d", backend.pollCalls)
	}
}

// Regression: a transient error (502) must NOT break the flow.
// Previously the very first error kicked out of polling with "device-flow failed".
func TestWaitLinkUntilLinked_ResilientToTransientErrors(t *testing.T) {
	defer tempConfigDir(t)()
	backend := newMockBackend([]string{"pending", "pending", "pending", "pending", "linked"}, "jwt-2")
	backend.failEvery = 3 // every 3rd poll — 502
	defer backend.Close()

	cfg := &config.Config{APIBase: backend.server.URL}
	start, err := StartAgentLink(context.Background(), cfg.APIBase)
	if err != nil {
		t.Fatal(err)
	}
	cr, err := WaitLinkUntilLinked(context.Background(), cfg, start, 15*time.Second, nil)
	if err != nil {
		t.Fatalf("устойчивый polling упал: %v", err)
	}
	if cr.Token != "jwt-2" {
		t.Fatalf("токен: %q", cr.Token)
	}
}

// Regression: ExpiresIn=0 from the server must not silently fail on the very
// first iteration. WaitLinkUntilLinked now uses a 15 min default when deadline=0.
func TestWaitLinkUntilLinked_ZeroExpiresIn(t *testing.T) {
	defer tempConfigDir(t)()
	backend := newMockBackend([]string{"pending", "linked"}, "jwt-3")
	backend.forceExpiresIn = 0 // server did not send it
	defer backend.Close()

	cfg := &config.Config{APIBase: backend.server.URL}
	start, _ := StartAgentLink(context.Background(), cfg.APIBase)
	// Simulate the caller passing time.Duration(0)*time.Second.
	cr, err := WaitLinkUntilLinked(context.Background(), cfg, start, 0, nil)
	if err != nil {
		t.Fatalf("polling с ExpiresIn=0 упал: %v", err)
	}
	if cr.Token != "jwt-3" {
		t.Fatalf("токен: %q", cr.Token)
	}
}

// Sanity: expired → returns an error.
func TestWaitLinkUntilLinked_Expired(t *testing.T) {
	defer tempConfigDir(t)()
	backend := newMockBackend([]string{"expired"}, "")
	defer backend.Close()
	cfg := &config.Config{APIBase: backend.server.URL}
	start, _ := StartAgentLink(context.Background(), cfg.APIBase)
	_, err := WaitLinkUntilLinked(context.Background(), cfg, start, 5*time.Second, nil)
	if err == nil {
		t.Fatal("ждали error на 'expired' статус")
	}
}

// Ctx cancel after 1 sec — polling must finish quickly.
func TestWaitLinkUntilLinked_ContextCancelled(t *testing.T) {
	defer tempConfigDir(t)()
	backend := newMockBackend([]string{"pending", "pending", "pending", "pending", "pending", "pending"}, "")
	defer backend.Close()

	cfg := &config.Config{APIBase: backend.server.URL}
	start, _ := StartAgentLink(context.Background(), cfg.APIBase)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	deadline := 30 * time.Second
	done := make(chan error, 1)
	go func() {
		_, err := WaitLinkUntilLinked(ctx, cfg, start, deadline, nil)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil || err != context.DeadlineExceeded {
			t.Fatalf("ждали context.DeadlineExceeded, имеем %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("polling не отреагировал на context.cancel за 5 сек")
	}
}

// onTick — must be called on every poll (the user sees a counter).
func TestWaitLinkUntilLinked_TickCallback(t *testing.T) {
	defer tempConfigDir(t)()
	backend := newMockBackend([]string{"pending", "pending", "linked"}, "jwt-ticks")
	defer backend.Close()

	cfg := &config.Config{APIBase: backend.server.URL}
	start, _ := StartAgentLink(context.Background(), cfg.APIBase)

	ticks := int32(0)
	_, err := WaitLinkUntilLinked(context.Background(), cfg, start, 10*time.Second,
		func() { atomic.AddInt32(&ticks, 1) })
	if err != nil {
		t.Fatal(err)
	}
	// pending, pending → 2 ticks (linked does not call onTick, it returns there).
	got := atomic.LoadInt32(&ticks)
	if got != 2 {
		t.Fatalf("ждали 2 тика, имеем %d", got)
	}
}
