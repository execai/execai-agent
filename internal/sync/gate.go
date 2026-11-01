package sync

import (
	"context"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
)

// SubscribeURL — where a refusal sends people to pick a plan.
const SubscribeURL = "https://chat.execai.ru/settings/payment?utm_source=agent&utm_medium=cli&utm_campaign=sync"

// Verdict — may sync run now.
type Verdict int

const (
	// Allowed — the plan includes sync.
	Allowed Verdict = iota
	// NotSignedIn — no ExecAI token, or billing rejected it.
	NotSignedIn
	// NoPlan — signed in, but the plan does not include sync (none, another
	// plan, ended, or unverified for too long).
	NoPlan
)

// Gate asks plan.Allows(FeatureSync). The answer comes from the cached
// plan.json; billing is re-asked only as the plan package allows: for a
// command the person typed — like any explicit check (Ensure); in the
// background — at most once a day (EnsureDaily), and only while sync is
// switched on (the runner does not ask otherwise).
type Gate struct {
	Checker *plan.Checker
	Now     func() time.Time
}

// Check returns the verdict. explicit: the person typed a command.
func (g *Gate) Check(ctx context.Context, explicit bool) Verdict {
	if g == nil || g.Checker == nil || !g.Checker.SignedIn() {
		return NotSignedIn
	}
	ctx, cancel := context.WithTimeout(ctx, plan.FetchTimeout)
	defer cancel()
	var p *plan.Plan
	if explicit {
		p = g.Checker.Ensure(ctx)
	} else {
		p = g.Checker.EnsureDaily(ctx)
	}
	now := time.Now()
	if g.Now != nil {
		now = g.Now()
	}
	switch {
	case p != nil && p.LoggedOut:
		return NotSignedIn
	case p != nil && p.Allows(plan.FeatureSync, now):
		return Allowed
	}
	return NoPlan
}

// Message — the refusal, in the person's language. "" for Allowed.
func (v Verdict) Message() string {
	switch v {
	case NotSignedIn:
		return i18n.T("sync.gate.login")
	case NoPlan:
		return i18n.Tf("sync.gate.noPlan", SubscribeURL)
	}
	return ""
}

// pauseReason — what State.Paused says for a refusal.
func (v Verdict) pauseReason() string {
	if v == NotSignedIn {
		return PausedLogin
	}
	return PausedPlan
}
