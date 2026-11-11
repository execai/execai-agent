package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Side finding of the second loss probe: any 404 on the identity — another
// service on the port, a proxy, a gateway's page — switched sync off here
// and reset its state («the sync data were deleted from the server»). Only
// our server's own answer («none», a JSON object with an error text) means
// there is no key; any other 404 is an error to try again later, and the
// state stays as it was.
func TestNotFound_OnlyTheServersOwnAnswer(t *testing.T) {
	i18n.SetLocale("en")
	stub := newStub(t)
	a := newMachine(t, stub, "a")
	a.converse(a.svc.OnFlow(), password, password)
	a.use()
	a.writeFile("memory/fact.md", "a fact\n")
	now(a)
	before, err := LoadState(a.svc.Paths)
	if err != nil || !before.Enabled || len(before.Blobs) == 0 {
		t.Fatalf("setup: %+v %v", before, err)
	}
	for name, h := range map[string]http.HandlerFunc{
		"plain text (a router's default)": func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) },
		"html page": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(404)
			_, _ = w.Write([]byte("<html><body>Not Found</body></html>"))
		},
		"empty": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) },
		"json, no error": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"detail":"Not Found"}`))
		},
		"json, error not a text": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":{"code":404}}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			foreign := httptest.NewServer(h)
			defer foreign.Close()
			a.use()
			a.svc.Client.Base = foreign.URL
			defer func() { a.svc.Client.Base = stub.URL }()
			_, err := a.svc.exchange(context.Background(), Options{}, time.Second)
			if err == nil || strings.Contains(err.Error(), "no sync data") {
				t.Errorf("exchange: %v", err)
			}
			if err != nil && (strings.Contains(err.Error(), "<") || strings.Contains(err.Error(), "Not Found")) {
				t.Errorf("the raw answer is shown: %v", err)
			}
			after, _ := LoadState(a.svc.Paths)
			if !after.Enabled || len(after.Blobs) != len(before.Blobs) || after.Paused != "" {
				t.Errorf("the state changed: enabled %v, paused %q, blobs %d → %d", after.Enabled, after.Paused, len(before.Blobs), len(after.Blobs))
			}
		})
	}
	// Our server's own «none» — or a gateway's that looks like it: a pause,
	// nothing reset, nothing switched off; the person decides.
	stub.mu.Lock()
	stub.identity = nil
	stub.mu.Unlock()
	a.use()
	if _, err := a.svc.exchange(context.Background(), Options{}, time.Second); err != errNoServerKey {
		t.Errorf("the server's own 404: %v", err)
	}
	after, _ := LoadState(a.svc.Paths)
	if !after.Enabled || after.Paused != PausedNoKey || len(after.Blobs) != len(before.Blobs) {
		t.Errorf("the state after «no key»: enabled %v, paused %q, blobs %d → %d", after.Enabled, after.Paused, len(before.Blobs), len(after.Blobs))
	}
}
