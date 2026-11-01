package plan

import (
	"context"
	"testing"
	"time"
)

// Checker.Allows gates features that come with the plan (the web console,
// sync) regardless of the model source: week_work/week_max unlock them,
// week_start does not, and without a login nothing is asked at all.
func TestChecker_AllowsFeature(t *testing.T) {
	work := `{"user_id":"u1","tariff_code":"week_work","state":"active",
		"valid_until":"2026-11-05T22:22:28Z","is_accessible":true}`
	cases := []struct {
		name  string
		body  string
		token string
		want  bool
	}{
		{"work plan", work, "tok", true},
		{"start plan", bodyActive, "tok", false},
		{"no plan", bodyNone, "tok", false},
		{"grace keeps work", bodyGrace, "tok", true},
		{"rejected token", work, "stale", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := newBilling(t, 200, c.body)
			ch := checkerAt(b.srv.URL, "", c.token, t0)
			if got := ch.Allows(context.Background(), FeatureBoard, true); got != c.want {
				t.Errorf("Allows(board) = %v, want %v", got, c.want)
			}
		})
	}

	b := newBilling(t, 200, work)
	ch := checkerAt(b.srv.URL, "", "", t0)
	if ch.Allows(context.Background(), FeatureBoard, true) {
		t.Error("allowed without a login")
	}
	if b.calls.Load() != 0 {
		t.Errorf("billing asked %d times without a token", b.calls.Load())
	}
}

// A check nobody asked for uses the saved answer: my_plan may renew (charge)
// an expired period. Only with no answer at all is billing asked once.
func TestChecker_AllowsWithoutAskUsesSavedAnswer(t *testing.T) {
	b := newBilling(t, 200, bodyNone)
	ch := checkerAt(b.srv.URL, "", "tok", t0)
	if ch.Allows(context.Background(), FeatureBoard, false) {
		t.Fatal("no plan allowed")
	}
	if b.calls.Load() != 1 {
		t.Fatalf("first check without an answer: %d billing calls, want 1", b.calls.Load())
	}
	ch.now = func() time.Time { return t0.Add(time.Hour) } // a refusal is stale by now
	ch.Allows(context.Background(), FeatureBoard, false)
	if b.calls.Load() != 1 {
		t.Errorf("a background check asked billing again: %d calls", b.calls.Load())
	}
	ch.Allows(context.Background(), FeatureBoard, true)
	if b.calls.Load() != 2 {
		t.Errorf("an asked check did not re-ask billing: %d calls", b.calls.Load())
	}
}
