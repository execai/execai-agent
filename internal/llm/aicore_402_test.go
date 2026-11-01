package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A 402 from the ExecAI backend is recognisable (the plan gate re-checks the
// plan on it) and keeps its original text.
func TestAICore_402IsPaymentRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":"subscription_required"}`))
	}))
	defer srv.Close()
	c := NewAICoreClient(srv.URL, "tok", "m", "p")
	_, err := c.Stream(context.Background(), []AIMessage{{Role: "user", Content: "hi"}}, nil, StreamCallbacks{})
	if !errors.Is(err, ErrPaymentRequired) {
		t.Fatalf("402 not marked: %v", err)
	}
	if !strings.Contains(err.Error(), "402") || !strings.Contains(err.Error(), "subscription_required") {
		t.Errorf("original text lost: %q", err.Error())
	}

	srv500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv500.Close()
	_, err = NewAICoreClient(srv500.URL, "tok", "m", "p").Stream(context.Background(),
		[]AIMessage{{Role: "user", Content: "hi"}}, nil, StreamCallbacks{})
	if errors.Is(err, ErrPaymentRequired) {
		t.Error("500 marked as payment required")
	}
}
