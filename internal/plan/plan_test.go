package plan

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// Bodies as GET /billing-vbai/my_plan returns them (shape from
// billing/internal/transport/http/subscriptions_handler.go: myPlanDTO, and
// the bare object for a person without a subscription).
const (
	bodyActive = `{"user_id":"u1","tariff_code":"week_start","title":"Старт","state":"active",
		"valid_until":"2026-11-05T22:22:28Z","auto_renew":true,"trial_used":true,"is_accessible":true,"has_card":true}`
	bodyTrial = `{"user_id":"u1","tariff_code":"trial","title":"Пробный","state":"active",
		"valid_until":"2026-11-03T10:00:00Z","auto_renew":true,"trial_used":true,"is_accessible":true}`
	bodyGrace = `{"user_id":"u1","tariff_code":"week_work","state":"grace",
		"valid_until":"2026-10-31T10:00:00Z","grace_until":"2026-11-02T10:00:00Z","grace_attempts":1,
		"is_accessible":true,"renewal_failed_reason":"insufficient_funds"}`
	bodyExpired = `{"user_id":"u1","tariff_code":"week_start","state":"expired",
		"valid_until":"2026-10-20T10:00:00Z","grace_until":"2026-10-22T10:00:00Z","is_accessible":false}`
	bodyFlexible = `{"user_id":"u1","tariff_code":"flexible","title":"Гибкий","state":"active",
		"valid_until":"2099-01-01T00:00:00Z","is_accessible":true}`
	bodyNone      = `{"user_id":"u1","subscription":null,"is_accessible":false}`
	bodyCancelled = `{"user_id":"u1","tariff_code":"week_max","state":"cancelled",
		"valid_until":"2026-11-10T00:00:00Z","auto_renew":false,"is_accessible":true}`
	bodyNested = `{"user_id":"u1","is_accessible":true,"subscription":{"tariff_code":"week_work",
		"state":"active","valid_until":"2026-11-07T00:00:00Z","is_accessible":true}}`
	// Active row whose period has passed but is not renewed yet: billing
	// itself says no access.
	bodyActiveNotAccessible = `{"user_id":"u1","tariff_code":"week_start","state":"active",
		"valid_until":"2026-10-31T00:00:00Z","is_accessible":false}`
)

func TestParse(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		granted bool
		tariff  string
		state   string
	}{
		{"active", bodyActive, true, "week_start", "active"},
		{"trial", bodyTrial, true, "trial", "active"},
		{"grace", bodyGrace, true, "week_work", "grace"},
		{"expired", bodyExpired, false, "week_start", "expired"},
		{"flexible is not a plan", bodyFlexible, false, "flexible", "active"},
		{"no subscription", bodyNone, false, "", ""},
		{"cancelled promo period", bodyCancelled, true, "week_max", "cancelled"},
		{"nested subscription object", bodyNested, true, "week_work", "active"},
		{"active but not accessible", bodyActiveNotAccessible, false, "week_start", "active"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Parse([]byte(c.body), t0)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if p.Granted() != c.granted {
				t.Errorf("Granted() = %v, want %v (%+v)", p.Granted(), c.granted, p)
			}
			if p.TariffCode != c.tariff || p.State != c.state {
				t.Errorf("tariff/state = %q/%q, want %q/%q", p.TariffCode, p.State, c.tariff, c.state)
			}
			if !p.CheckedAt.Equal(t0) {
				t.Errorf("CheckedAt = %v, want %v", p.CheckedAt, t0)
			}
		})
	}
}

func TestParse_Dates(t *testing.T) {
	p, err := Parse([]byte(bodyGrace), t0)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC); !p.ValidUntil.Equal(want) {
		t.Errorf("ValidUntil = %v, want %v", p.ValidUntil, want)
	}
	// In grace the access ends at grace_until, not at valid_until.
	if want := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC); !p.End().Equal(want) {
		t.Errorf("End() = %v, want %v", p.End(), want)
	}
	tr, _ := Parse([]byte(bodyTrial), t0)
	if !tr.Trial() {
		t.Error("trial tariff not recognised as trial")
	}
}

func TestParse_Garbage(t *testing.T) {
	if _, err := Parse([]byte(`<html>bad gateway</html>`), t0); err == nil {
		t.Error("non-JSON body parsed without error")
	}
}

// A saved answer stands in for up to 7 days when billing is unreachable.
func TestActive_OfflineGrace(t *testing.T) {
	p, _ := Parse([]byte(bodyActive), t0)
	if !p.Active(t0.Add(6 * 24 * time.Hour)) {
		t.Error("6-day-old answer must still count")
	}
	if p.Active(t0.Add(8 * 24 * time.Hour)) {
		t.Error("8-day-old answer must not count")
	}
	none, _ := Parse([]byte(bodyNone), t0)
	if none.Active(t0) {
		t.Error("no subscription counted as active")
	}
	if (Plan{LoggedOut: true, CheckedAt: t0}).Active(t0) {
		t.Error("401 answer counted as active")
	}
}

func TestStale(t *testing.T) {
	p, _ := Parse([]byte(bodyActive), t0) // valid until 2026-11-05
	if p.Stale(t0.Add(time.Hour)) {
		t.Error("1-hour-old answer is not stale")
	}
	if !p.Stale(t0.Add(25 * time.Hour)) {
		t.Error("answer older than a day must be re-asked")
	}
	g, _ := Parse([]byte(bodyGrace), t0) // grace until 2026-11-02 10:00
	if !g.Stale(time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC)) {
		t.Error("grace past grace_until must be re-asked")
	}
	if !(Plan{LoggedOut: true, CheckedAt: t0}).Stale(t0) {
		t.Error("a 401 answer must be re-asked (the person may have logged in)")
	}
	none, _ := Parse([]byte(bodyNone), t0)
	if !none.Stale(t0.Add(time.Minute)) {
		t.Error("a refusal must be re-asked (the person may have just paid)")
	}
}

func TestAllows(t *testing.T) {
	mk := func(tariff, state string, accessible bool) Plan {
		return Plan{TariffCode: tariff, State: state, Accessible: accessible, CheckedAt: t0}
	}
	cases := []struct {
		p    Plan
		want bool
	}{
		{mk(TariffWeekWork, StateActive, true), true},
		{mk(TariffWeekMax, StateActive, true), true},
		{mk(TariffWeekMax, StateGrace, true), true},
		{mk(TariffWeekStart, StateActive, true), false},
		{mk(TariffMonthStart, StateActive, true), false},
		{mk(TariffTrial, StateActive, true), false},
		{mk(TariffWeekWork, StateExpired, false), false},
	}
	for _, c := range cases {
		for _, f := range []Feature{FeatureSync, FeatureBoard} {
			if got := c.p.Allows(f, t0); got != c.want {
				t.Errorf("%s/%s Allows(%s) = %v, want %v", c.p.TariffCode, c.p.State, f, got, c.want)
			}
		}
	}
	old := mk(TariffWeekWork, StateActive, true)
	if old.Allows(FeatureSync, t0.Add(8*24*time.Hour)) {
		t.Error("an answer older than the offline grace must not unlock features")
	}
}
