// Package plan knows whether the person has an active ExecAI plan and
// decides whether a model call may go ahead (the subscription gate).
//
// The plan is needed for the ExecAI models only: the person's own sources
// (provider keys, local CLIs, custom endpoints, Ollama) work without a plan
// and without a login. Sync and the web console come with the Work and Max
// plans (Allows). The answer comes from GET /billing-vbai/my_plan with
// the same JWT the agent already holds, and is cached in
// ~/.config/execai/plan.json so that a start never waits on the network and
// a short outage does not lock anyone out (see OfflineGrace).
package plan

import (
	"encoding/json"
	"fmt"
	"time"
)

// Tariff codes the gate has to tell apart. The full catalog lives in
// billing-vbai (billing/internal/catalog/data/tariffs.go); the agent never
// keeps prices or limits, only these few names.
const (
	TariffFlexible     = "flexible" // pay-as-you-go balance, NOT a subscription
	TariffTrial        = "trial"    // the first 3 days for 1 ₽
	TariffWeekStart    = "week_start"
	TariffWeekWork     = "week_work"
	TariffWeekMax      = "week_max"
	TariffMonthStart   = "month_start"
	TariffOwnerService = "owner_service"
)

// Subscription states as billing-vbai reports them.
const (
	StateActive    = "active"
	StateCancelled = "cancelled" // promo period without auto-renewal; access until valid_until
	StateGrace     = "grace"     // the card was declined, access lives until grace_until
	StateExpired   = "expired"
)

// OfflineGrace — how long a saved answer stands in when billing cannot be
// reached (network error, 5xx). Older than that, the plan counts as inactive.
const OfflineGrace = 7 * 24 * time.Hour

// RefreshEvery — how often a long-running process re-asks billing.
const RefreshEvery = 24 * time.Hour

// Plan is what the agent knows about the person's ExecAI plan.
type Plan struct {
	TariffCode string    `json:"tariff_code,omitempty"`
	Title      string    `json:"title,omitempty"` // display name from the billing catalog
	State      string    `json:"state,omitempty"`
	ValidUntil time.Time `json:"valid_until,omitempty"`
	GraceUntil time.Time `json:"grace_until,omitempty"`
	// Accessible — billing's own verdict (is_accessible) at CheckedAt: it
	// already accounts for valid_until, grace_until and cancelled periods.
	Accessible bool `json:"is_accessible"`
	// LoggedOut — billing answered 401: the token is not accepted.
	LoggedOut bool `json:"logged_out,omitempty"`
	// CheckedAt — when billing gave this answer.
	CheckedAt time.Time `json:"checked_at"`
	// TokenFP — fingerprint of the token the answer was given for. A
	// different token (re-login, another account) means the answer is stale.
	TokenFP string `json:"token_fp,omitempty"`
}

// myPlanDTO mirrors GET /billing-vbai/my_plan. A subscriber gets the fields at
// the top level; a person without a subscription gets
// {"user_id": "...", "subscription": null, "is_accessible": false}.
// A nested "subscription" object is read too, in case the shape converges.
type myPlanDTO struct {
	TariffCode   string          `json:"tariff_code"`
	Title        string          `json:"title"`
	State        string          `json:"state"`
	ValidUntil   string          `json:"valid_until"`
	GraceUntil   string          `json:"grace_until"`
	IsAccessible *bool           `json:"is_accessible"`
	Subscription json.RawMessage `json:"subscription"`
}

// Parse decodes a my_plan response body. checkedAt stamps the answer.
func Parse(body []byte, checkedAt time.Time) (Plan, error) {
	var top myPlanDTO
	if err := json.Unmarshal(body, &top); err != nil {
		return Plan{}, fmt.Errorf("my_plan: %w", err)
	}
	d := top
	if top.TariffCode == "" && len(top.Subscription) > 0 && string(top.Subscription) != "null" {
		var nested myPlanDTO
		if err := json.Unmarshal(top.Subscription, &nested); err != nil {
			return Plan{}, fmt.Errorf("my_plan subscription: %w", err)
		}
		if nested.IsAccessible == nil {
			nested.IsAccessible = top.IsAccessible
		}
		d = nested
	}
	p := Plan{
		TariffCode: d.TariffCode,
		Title:      d.Title,
		State:      d.State,
		ValidUntil: parseTime(d.ValidUntil),
		GraceUntil: parseTime(d.GraceUntil),
		Accessible: d.IsAccessible != nil && *d.IsAccessible,
		CheckedAt:  checkedAt,
	}
	return p, nil
}

// parseTime reads billing's RFC 3339 timestamps; anything else is "unknown".
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Granted — billing's verdict at check time: a subscription that is not the
// pay-as-you-go balance, not expired, and accessible (active, trial, promo
// period, or grace). It says nothing about how old the answer is.
func (p Plan) Granted() bool {
	if p.LoggedOut || p.TariffCode == "" || p.TariffCode == TariffFlexible {
		return false
	}
	return p.State != StateExpired && p.Accessible
}

// Trial reports whether the plan is the 3-day trial.
func (p Plan) Trial() bool { return p.TariffCode == TariffTrial }

// Active — the plan lets the agent work at now: granted, and the answer is
// not older than OfflineGrace.
func (p Plan) Active(now time.Time) bool {
	return p.Granted() && p.Fresh(now)
}

// Fresh reports whether the answer is young enough to stand in for billing
// (OfflineGrace).
func (p Plan) Fresh(now time.Time) bool {
	return !p.CheckedAt.IsZero() && now.Sub(p.CheckedAt) <= OfflineGrace
}

// End — when the current access ends by the saved answer: grace_until in
// grace, valid_until otherwise. Zero when unknown.
func (p Plan) End() time.Time {
	if p.State == StateGrace && !p.GraceUntil.IsZero() {
		return p.GraceUntil
	}
	return p.ValidUntil
}

// Stale reports whether the answer should be re-asked: older than
// RefreshEvery, past the end of the paid period, or a refusal of any kind
// (no plan, expired, 401). A refusal is always worth asking again — the
// person may have just paid or logged in on the site; Checker limits how
// often (RefusalRecheck).
func (p Plan) Stale(now time.Time) bool {
	if !p.Granted() || now.Sub(p.CheckedAt) >= RefreshEvery {
		return true
	}
	end := p.End()
	return !end.IsZero() && now.After(end)
}

// Feature — something only some plans unlock.
type Feature string

const (
	// FeatureSync — encrypted sync of agent memory and sessions between machines.
	FeatureSync Feature = "sync"
	// FeatureBoard — the web console (board) for the person's agents.
	FeatureBoard Feature = "board"
)

// featureTariffs — which plans unlock each feature. Kept in ONE place on
// purpose: a monthly "Work" plan is expected to join week_work/week_max here
// once billing ships it.
var featureTariffs = map[Feature][]string{
	// owner_service is the in-house service plan (no price, not on the
	// storefront): it unlocks everything the Max plan does.
	FeatureSync:  {TariffWeekWork, TariffWeekMax, TariffOwnerService},
	FeatureBoard: {TariffWeekWork, TariffWeekMax, TariffOwnerService},
}

// Allows reports whether the plan unlocks f at now: the plan is active and
// its tariff is one of those listed for f.
func (p Plan) Allows(f Feature, now time.Time) bool {
	if !p.Active(now) {
		return false
	}
	for _, code := range featureTariffs[f] {
		if p.TariffCode == code {
			return true
		}
	}
	return false
}
