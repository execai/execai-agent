package ide

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// Run speaks over the transport it is given (execai ui) instead of
// stdin/stdout, and an input that merely stays quiet — a browser tab closed,
// nobody typing — does not end it: only the context does.
func TestRun_CustomTransportEndsOnContextOnly(t *testing.T) {
	dir := isolateIDE(t)
	t.Chdir(dir)
	t.Setenv("EXECAI_IDE_INBOX", "off")
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/billing-vbai/models_public":
			_, _ = w.Write([]byte(`[{"id":"m1","provider":"p","name":"M1","is_primary":true}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()
	cfg := config.Defaults()
	cfg.APIBase = api.URL
	cfg.Locale = "en"

	inR, inW := io.Pipe()
	defer func() { _ = inW.Close() }()
	out := &syncBuf{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, Options{Cwd: dir, In: inR, Out: out}) }()

	out.waitEvent(t, func(o Out) bool { return o.Type == "ready" })
	if _, err := inW.Write([]byte(`{"type":"ping"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	out.waitEvent(t, func(o Out) bool { return o.Type == "pong" })

	select {
	case err := <-done:
		t.Fatalf("Run returned while the input was open: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return on a cancelled context")
	}
}
