package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"

	// Register i18n catalogs — usage output is localized; tests run under the
	// default locale (en) and assert English strings.
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
)

// TestKimiTierFromModels — plan detection from the model list.
func TestKimiTierFromModels(t *testing.T) {
	cases := []struct {
		name   string
		models []string
		want   string
	}{
		{"empty", nil, ""},
		{"only-k27", []string{"kimi-for-coding"}, "K2.7 Code"},
		{"k27+k3", []string{"kimi-for-coding", "k3"}, "K3"},
		{"k27+k3+highspeed", []string{"kimi-for-coding", "k3", "kimi-for-coding-highspeed"}, "K3 + HighSpeed"},
		{"unknown-only", []string{"foo", "bar"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := kimiTierFromModels(tc.models); got != tc.want {
				t.Errorf("kimiTierFromModels(%v) = %q, want %q", tc.models, got, tc.want)
			}
		})
	}
}

// TestKimiWindowLabel — rolling-window formatting.
func TestKimiWindowLabel(t *testing.T) {
	cases := []struct {
		duration int64
		unit     string
		idx      int
		want     string
	}{
		{300, "MINUTES", 0, "5h"}, // 300 min = 5h
		{45, "MINUTES", 0, "45min"},
		{5, "HOURS", 0, "5h"},
		{1, "DAYS", 0, "1d"},
		{0, "", 3, "window #4"},
	}
	for _, tc := range cases {
		if got := kimiWindowLabel(tc.duration, tc.unit, tc.idx); got != tc.want {
			t.Errorf("kimiWindowLabel(%d, %q, %d) = %q, want %q", tc.duration, tc.unit, tc.idx, got, tc.want)
		}
	}
}

// TestKimiResetHint — formatting ISO time as a localized "resets in X" hint
// (default locale = en).
func TestKimiResetHint(t *testing.T) {
	// Empty = empty.
	if got := kimiResetHint(""); got != "" {
		t.Errorf("empty ISO should give empty, got %q", got)
	}
	// Past = "refreshing".
	past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	if got := kimiResetHint(past); got != "refreshing" {
		t.Errorf("past ISO should give 'refreshing', got %q", got)
	}
	// ~2h in the future should start with "resets".
	future := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	got := kimiResetHint(future)
	if !strings.HasPrefix(got, "resets") {
		t.Errorf("future ISO should start with 'resets', got %q", got)
	}
}

// TestFetchKimiAvailableModels_OK — successful /models response.
func TestFetchKimiAvailableModels_OK(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"k3"},{"id":"kimi-for-coding"},{"id":"kimi-for-coding-highspeed"}]}`))
	}))
	defer srv.Close()

	models := fetchKimiAvailableModels(srv.URL, "sk-test-key")
	if len(models) != 3 {
		t.Fatalf("want 3 models, got %d: %v", len(models), models)
	}
	if models[0] != "k3" || models[1] != "kimi-for-coding" || models[2] != "kimi-for-coding-highspeed" {
		t.Errorf("wrong models: %v", models)
	}
	if gotAuth != "Bearer sk-test-key" {
		t.Errorf("wrong auth header: %q", gotAuth)
	}
}

// TestFetchKimiAvailableModels_Empty — server returned 401/500/empty.
func TestFetchKimiAvailableModels_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	if got := fetchKimiAvailableModels(srv.URL, "sk-x"); len(got) != 0 {
		t.Errorf("401 should give empty, got %v", got)
	}
}

// TestFetchKimiAvailableModels_NoServer — network unavailable.
func TestFetchKimiAvailableModels_NoServer(t *testing.T) {
	// Port 1 is always closed.
	if got := fetchKimiAvailableModels("http://127.0.0.1:1", "sk-x"); len(got) != 0 {
		t.Errorf("closed port should give empty, got %v", got)
	}
}

// TestFetchKimiUsage_FullPayload — full payload with quota and a rolling window.
func TestFetchKimiUsage_FullPayload(t *testing.T) {
	future := time.Now().Add(3 * 24 * time.Hour).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("wrong auth: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"usage":  {"name":"Weekly limit","used":300,"limit":1000,"resetAt":"` + future + `"},
			"limits": [
				{"detail":{"used":40,"limit":100,"name":"5h limit","resetAt":"` + future + `"},
				 "window":{"duration":300,"timeUnit":"MINUTES"}}
			]
		}`))
	}))
	defer srv.Close()

	sub := subscriptions.Subscription{
		Provider:        subscriptions.SourceKimi,
		APIKey:          "sk-test",
		BaseURL:         srv.URL,
		Plan:            "coding",
		AvailableModels: []string{"k3", "kimi-for-coding", "kimi-for-coding-highspeed"},
	}
	text, err := fetchKimiUsage(sub)
	if err != nil {
		t.Fatalf("fetchKimiUsage: %v", err)
	}
	// Check the key pieces.
	// humanInt(1000) → "1.0k" (readability formatting), hence not "300 / 1000".
	// In the rolling window we use Detail.Name if present ("5h limit"),
	// duration/timeUnit is the fallback.
	mustContain := []string{
		"Kimi Code",
		"Plan: K3 + HighSpeed",
		"Weekly limit",
		"30%",        // 300/1000
		"300 / 1.0k", // humanInt formats it
		"(700 left)",
		"5h limit", // priority — Detail.Name from the payload
		"40 / 100",
	}
	for _, s := range mustContain {
		if !strings.Contains(text, s) {
			t.Errorf("missing substring %q in output:\n%s", s, text)
		}
	}
}

// TestFetchKimiUsage_HTTPError — server returned 500.
func TestFetchKimiUsage_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	sub := subscriptions.Subscription{
		Provider: subscriptions.SourceKimi,
		APIKey:   "sk",
		BaseURL:  srv.URL,
		Plan:     "coding",
	}
	_, err := fetchKimiUsage(sub)
	if err == nil {
		t.Fatal("want error from 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention 500, got: %v", err)
	}
}

// TestFetchKimiUsage_StringNumbers — server sends used/limit as strings.
// Regression: prod returned "cannot unmarshal string into Go struct field .usage.limit"
// because the struct expected int64.
func TestFetchKimiUsage_StringNumbers(t *testing.T) {
	future := time.Now().Add(3 * 24 * time.Hour).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// All numbers are strings. Exactly what api.kimi.com returned in prod.
		w.Write([]byte(`{
			"usage":  {"name":"Weekly limit","used":"250","limit":"1000","resetAt":"` + future + `"},
			"limits": [
				{"detail":{"used":"5","limit":"100","name":"5h limit","resetAt":"` + future + `"},
				 "window":{"duration":"300","timeUnit":"MINUTES"}}
			]
		}`))
	}))
	defer srv.Close()

	sub := subscriptions.Subscription{
		Provider: subscriptions.SourceKimi,
		APIKey:   "sk",
		BaseURL:  srv.URL,
		Plan:     "coding",
	}
	text, err := fetchKimiUsage(sub)
	if err != nil {
		t.Fatalf("fetchKimiUsage with string numbers: %v", err)
	}
	mustContain := []string{"25%", "250 / 1.0k", "5 / 100"}
	for _, s := range mustContain {
		if !strings.Contains(text, s) {
			t.Errorf("string-numbers case: missing %q in:\n%s", s, text)
		}
	}
}

// TestParseFlexInt — universal parser: number, number-in-string, float, float-in-string.
func TestParseFlexInt(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"42", 42},
		{`"42"`, 42},
		{"3.14", 3},
		{`"3.14"`, 3},
		{"0", 0},
		{`"0"`, 0},
		{`""`, 0},
		{"null", 0},
		{`"abc"`, 0},
	}
	for _, tc := range cases {
		got := parseFlexInt([]byte(tc.in))
		if got != tc.want {
			t.Errorf("parseFlexInt(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestFetchKimiUsage_BaseURLNormalization — base with and without the /v1 suffix.
func TestFetchKimiUsage_BaseURLNormalization(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Write([]byte(`{"usage":{"used":0,"limit":100}}`))
	}))
	defer srv.Close()

	// base without /v1
	sub := subscriptions.Subscription{Provider: subscriptions.SourceKimi, APIKey: "x", BaseURL: srv.URL, Plan: "coding"}
	_, _ = fetchKimiUsage(sub)
	if seenPath != "/v1/usages" {
		t.Errorf("base without /v1: want path /v1/usages, got %s", seenPath)
	}
	// base with /v1
	sub2 := subscriptions.Subscription{Provider: subscriptions.SourceKimi, APIKey: "x", BaseURL: srv.URL + "/v1", Plan: "coding"}
	_, _ = fetchKimiUsage(sub2)
	if seenPath != "/v1/usages" {
		t.Errorf("base with /v1: want path /v1/usages, got %s", seenPath)
	}
	// base with trailing slash
	sub3 := subscriptions.Subscription{Provider: subscriptions.SourceKimi, APIKey: "x", BaseURL: srv.URL + "/", Plan: "coding"}
	_, _ = fetchKimiUsage(sub3)
	if seenPath != "/v1/usages" {
		t.Errorf("base with trailing /: want /v1/usages, got %s", seenPath)
	}
}
