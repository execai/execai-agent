package inbox

import (
	"context"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
)

// PlanURL — where the plans that include running tasks from the site are
// sold: the same page the model gate sends people to.
const PlanURL = plan.SubscribeURL

// FeatureGate is the part of plan.Checker the inbox needs.
type FeatureGate interface {
	Allows(ctx context.Context, f plan.Feature, ask bool) bool
}

// Allowed reports whether the person's plan includes running tasks from the
// site (plan.FeatureBoard: the Work and Max plans). This gate is about the
// channel itself and does not depend on the model source; the model call has
// its own gate (plan.Checker.Check). ask — billing may be asked now (a task
// arrived, the person pressed a button); otherwise the saved answer counts
// (see plan.Checker.Allows). A nil gate allows.
func Allowed(ctx context.Context, g FeatureGate, ask bool) bool {
	if g == nil {
		return true
	}
	return g.Allows(ctx, plan.FeatureBoard, ask)
}

// Background — whether a check nobody asked for may ask billing: only while
// the ExecAI models are the active source, where the agent asks it anyway.
func Background() bool { return plan.OnExecAI() }

// PlanRefusal is the result of a task refused by the plan — what the web chat
// shows instead of an answer.
func PlanRefusal() string {
	return i18n.Tf("inbox.plan.refused", PlanURL)
}
