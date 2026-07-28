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

// mockBackend — fake /auth-vbai/agent-link/start + /poll. Скрипт ответов —
// последовательность статусов, возвращается по одному на каждый poll.
type mockBackend struct {
	startCalls  int32
	pollCalls   int32
	pollStatuses []string // по одному на вызов; после конца — цикл на последнем
	linkedJWT   string
	failEvery   int32 // если >0 — каждый N-й poll возвращает 502 (для теста устойчивости)
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
			"poll_interval": 1, // 1 сек — быстрее чем прод-3, чтобы тесты не тормозили
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

// tempConfigDir — изолируем credentials.json от юзерского.
func tempConfigDir(t *testing.T) func() {
	dir := t.TempDir()
	old := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	// Убедимся что папка execai существует.
	_ = os.MkdirAll(filepath.Join(dir, "execai"), 0o700)
	return func() { _ = os.Setenv("XDG_CONFIG_HOME", old) }
}

// Happy path: 2 pending → linked. Клиент должен получить credentials.
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

// Регрессия: транзиентная ошибка (502) НЕ должна ронять flow.
// Раньше первая же ошибка выкидывала из polling'а с "device-flow упал".
func TestWaitLinkUntilLinked_ResilientToTransientErrors(t *testing.T) {
	defer tempConfigDir(t)()
	backend := newMockBackend([]string{"pending", "pending", "pending", "pending", "linked"}, "jwt-2")
	backend.failEvery = 3 // каждый 3-й poll — 502
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

// Регрессия: ExpiresIn=0 от сервера — не должно молча зафейлить в первую же
// итерацию. WaitLinkUntilLinked теперь использует 15 мин дефолт если deadline=0.
func TestWaitLinkUntilLinked_ZeroExpiresIn(t *testing.T) {
	defer tempConfigDir(t)()
	backend := newMockBackend([]string{"pending", "linked"}, "jwt-3")
	backend.forceExpiresIn = 0 // сервер не прислал
	defer backend.Close()

	cfg := &config.Config{APIBase: backend.server.URL}
	start, _ := StartAgentLink(context.Background(), cfg.APIBase)
	// Симулируем что вызывающая сторона передаёт time.Duration(0)*time.Second.
	cr, err := WaitLinkUntilLinked(context.Background(), cfg, start, 0, nil)
	if err != nil {
		t.Fatalf("polling с ExpiresIn=0 упал: %v", err)
	}
	if cr.Token != "jwt-3" {
		t.Fatalf("токен: %q", cr.Token)
	}
}

// Sanity: expired → отдаёт ошибку.
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

// Ctx-cancel через 1 сек — polling должен завершиться быстро.
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

// onTick — должен вызваться на каждый poll (юзер видит счётчик).
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
	// pending, pending → 2 тика (linked не вызывает onTick, там возврат).
	got := atomic.LoadInt32(&ticks)
	if got != 2 {
		t.Fatalf("ждали 2 тика, имеем %d", got)
	}
}
