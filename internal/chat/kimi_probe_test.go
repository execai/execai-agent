package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProbeKimiOne_Anthropic — emulation of api.kimi.com/coding/v1/messages.
func TestProbeKimiOne_Anthropic(t *testing.T) {
	var gotKey, gotVer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotVer = r.Header.Get("anthropic-version")
		// 400 = "model unknown" — the probe accepts it as valid.
		w.WriteHeader(400)
	}))
	defer srv.Close()

	code, err := probeKimiOne(&http.Client{}, probeAttempt{
		url:    srv.URL + "/v1/messages",
		plan:   "coding",
		base:   srv.URL,
		authAn: true,
	}, "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	if code != 400 {
		t.Errorf("want 400, got %d", code)
	}
	if gotKey != "sk-test" {
		t.Errorf("x-api-key wrong: %q", gotKey)
	}
	if gotVer == "" {
		t.Errorf("anthropic-version missing")
	}
}

// TestProbeKimiOne_OpenAI — emulation of /chat/completions with Bearer auth.
func TestProbeKimiOne_OpenAI(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	code, err := probeKimiOne(&http.Client{}, probeAttempt{
		url:    srv.URL + "/v1/chat/completions",
		plan:   "api",
		base:   srv.URL + "/v1",
		authAn: false,
	}, "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	if code != 200 {
		t.Errorf("want 200, got %d", code)
	}
	if !strings.HasPrefix(gotAuth, "Bearer sk-test") {
		t.Errorf("Authorization wrong: %q", gotAuth)
	}
}

// TestProbeKimiOne_NetworkError — unreachable port.
func TestProbeKimiOne_NetworkError(t *testing.T) {
	code, err := probeKimiOne(&http.Client{}, probeAttempt{
		url: "http://127.0.0.1:1/x",
	}, "sk-x")
	if err == nil {
		t.Fatal("want network error, got nil")
	}
	if code != 0 {
		t.Errorf("code should be 0 on network error, got %d", code)
	}
}
