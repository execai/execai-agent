package plan

import (
	"context"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Describe renders the /plan report: state, tariff, valid until, when it
// was checked, what the agent may do, and the feature line.
//
//	p        — the latest answer, nil if there is none;
//	signedIn — an ExecAI token is present;
//	offline  — the latest refresh failed, so p is a saved answer;
//	now      — the moment of the report.
func Describe(p *Plan, signedIn, offline bool, now time.Time) string {
	var b strings.Builder
	line := func(key string, args ...any) {
		b.WriteString(i18n.Tf(key, args...))
		b.WriteByte('\n')
	}
	known := signedIn && p != nil && !p.LoggedOut
	v := Decide(p, signedIn, false, now)

	b.WriteString(i18n.T("plan.show.header") + "\n")
	line("plan.show.state", stateText(p, signedIn, now))
	if known && p.TariffCode != "" {
		name := p.TariffCode
		if p.Title != "" && p.Title != p.TariffCode {
			name = p.Title + " (" + p.TariffCode + ")"
		}
		line("plan.show.tariff", name)
		if !p.ValidUntil.IsZero() && p.TariffCode != TariffFlexible {
			line("plan.show.validUntil", stamp(p.ValidUntil))
		}
	}
	if known && !p.CheckedAt.IsZero() {
		key := "plan.show.checked"
		if offline {
			key = "plan.show.checkedOffline"
		}
		line(key, stamp(p.CheckedAt))
	}
	allowed := "plan.allowed.ownOnly"
	if v.Allowed {
		allowed = "plan.allowed.all"
	}
	line("plan.show.allowed", i18n.T(allowed))
	features := "plan.features.no"
	if known && p.Allows(FeatureSync, now) && p.Allows(FeatureBoard, now) {
		features = "plan.features.yes"
	}
	line("plan.show.features", i18n.T(features))

	if !v.Allowed {
		b.WriteByte('\n')
		if v.Reason == ReasonNotLoggedIn {
			b.WriteString(i18n.T("plan.gate.notLoggedIn"))
		} else {
			b.WriteString(i18n.Tf("plan.show.subscribe", SubscribeURL))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// stateText — the "State" line of /plan.
func stateText(p *Plan, signedIn bool, now time.Time) string {
	switch {
	case !signedIn || (p != nil && p.LoggedOut):
		return i18n.T("plan.state.signedOut")
	case p == nil:
		return i18n.T("plan.state.unknown")
	case p.Granted() && !p.Fresh(now):
		return i18n.T("plan.state.unverified")
	case p.TariffCode == "":
		return i18n.T("plan.state.none")
	case p.TariffCode == TariffFlexible:
		return i18n.T("plan.state.flexible")
	case p.State == StateExpired || !p.Accessible:
		return i18n.T("plan.state.expired")
	case p.State == StateGrace:
		return i18n.Tf("plan.state.grace", stamp(p.GraceUntil))
	case p.Trial():
		return i18n.T("plan.state.trial")
	case p.State == StateCancelled:
		return i18n.T("plan.state.cancelled")
	default:
		return i18n.T("plan.state.active")
	}
}

// stamp — a date for people, in local time; locale-neutral on purpose.
func stamp(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// Report refreshes the answer (bounded by FetchTimeout) and renders it.
func (c *Checker) Report(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, FetchTimeout)
	defer cancel()
	err := c.Refresh(ctx)
	return Describe(c.Current(), c.SignedIn(), err != nil, c.now())
}
