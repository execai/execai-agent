package llmpick

import (
	"github.com/velesbsdllc/agent-vbai/internal/catalog"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// Model picks the model for the active source by the same rules as the
// terminal and the editor panel (the source-switch invariants): the selected
// model when the active source offers it, otherwise that source's primary
// model, and nothing at all when its catalog is empty — never a model of
// another source. An ExecAI model ID sent to the person's own subscription is
// a request the provider rejects (or, worse, one it bills differently).
//
// execai is the ExecAI catalog (sorted primary-first), used when no
// subscription is active; provider is the ExecAI provider of the chosen model
// and empty on a subscription.
func Model(selected string, subs *subscriptions.Store, execai []llm.Model) (id, provider string) {
	if a := subs.ActiveSubscription(); a != nil {
		ms := catalog.For(*a)
		for _, m := range ms {
			if m.ID == selected {
				return m.ID, ""
			}
		}
		// The source's primary model, not the first entry: Z.ai lists newest
		// first, and the first one is a -flashx variant.
		for _, m := range ms {
			if m.IsPrimary {
				return m.ID, ""
			}
		}
		return "", ""
	}
	for _, m := range execai {
		if m.ID == selected {
			return m.ID, m.Provider
		}
	}
	if len(execai) > 0 {
		return execai[0].ID, execai[0].Provider
	}
	return "", ""
}
