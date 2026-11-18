package llmpick

import (
	"fmt"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/catalog"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// cheapMarks — fragments of model IDs and names that mean "the small,
// cheap variant", most telling first. Catalogs carry no prices, but every
// vendor names its cheap models this way.
var cheapMarks = []struct {
	frag  string
	score int
}{
	{"nano", 7}, {"flash-lite", 7}, {"lite", 6}, {"haiku", 6},
	{"mini", 5}, {"flashx", 5}, {"flash", 5}, {"air", 4}, {"small", 4},
	{"highspeed", 2}, {"turbo", 2}, {"fast", 2},
}

// cheapScore rates how cheap a model looks; higher is cheaper.
func cheapScore(m llm.Model) int {
	low := strings.ToLower(m.ID + " " + m.Name)
	score := 0
	for _, c := range cheapMarks {
		if strings.Contains(low, c.frag) && c.score > score {
			score = c.score
		}
	}
	switch strings.ToLower(m.Tier) {
	case "lite":
		score += 10
	case "flagship":
		score -= 5
	}
	return score
}

// SourceModels — the catalog of the active source: the subscription's
// models, or the ExecAI catalog when no subscription is active.
func SourceModels(subs *subscriptions.Store, execai []llm.Model) []llm.Model {
	if a := subs.ActiveSubscription(); a != nil {
		return catalog.For(*a)
	}
	return execai
}

// Fast picks the cheapest-looking model of the active source — the
// "model: fast" of a subagent sent to grep a codebase. Models that cannot
// call tools are skipped while there is any that can; with no cheap marker
// anywhere the source's usual model (Model) is the answer.
func Fast(subs *subscriptions.Store, execai []llm.Model) (id, provider string) {
	ms := SourceModels(subs, execai)
	withTools := false
	for _, m := range ms {
		if m.HasTools {
			withTools = true
			break
		}
	}
	best, bestScore := -1, 0
	for i, m := range ms {
		if withTools && !m.HasTools {
			continue
		}
		if s := cheapScore(m); s > bestScore {
			best, bestScore = i, s
		}
	}
	if best < 0 {
		return Model("", subs, execai)
	}
	if subs.ActiveSubscription() != nil {
		return ms[best].ID, ""
	}
	return ms[best].ID, ms[best].Provider
}

// Picker builds clients for a subagent's model parameter. It is made at the
// start of a turn from a copy of the settings, so subagents may call it from
// their own goroutines while the person switches the source or the model.
type Picker struct {
	cfg    config.Config
	subs   *subscriptions.Store
	execai []llm.Model
	token  string
}

// NewPicker snapshots what Pick needs: the config, the active subscription,
// the ExecAI catalog and the ExecAI token.
func NewPicker(cfg *config.Config, subs *subscriptions.Store, execai []llm.Model, token string) *Picker {
	p := &Picker{execai: append([]llm.Model(nil), execai...), token: token}
	if cfg != nil {
		p.cfg = *cfg
	}
	if subs != nil {
		snap := &subscriptions.Store{Active: subs.Active, Subscriptions: map[string]subscriptions.Subscription{}}
		if a := subs.ActiveSubscription(); a != nil {
			snap.Subscriptions[subs.Active] = *a
		}
		p.subs = snap
	}
	return p
}

// Pick returns a client for "fast" or for a model ID of the active source.
// It refuses an ID the source does not offer: an ExecAI model sent to a
// person's own key is a request the provider rejects or bills differently.
func (p *Picker) Pick(model string) (llm.StreamingLLM, string, error) {
	var id, provider string
	switch model {
	case "fast":
		id, provider = Fast(p.subs, p.execai)
		if id == "" && p.subs.ActiveSubscription() == nil {
			return nil, "", fmt.Errorf("no model available for the active source")
		}
	default:
		ms := SourceModels(p.subs, p.execai)
		found := false
		var ids []string
		for _, m := range ms {
			if m.ID == model {
				id, found = m.ID, true
				if p.subs.ActiveSubscription() == nil {
					provider = m.Provider
				}
				break
			}
			if len(ids) < 12 {
				ids = append(ids, m.ID)
			}
		}
		if !found {
			return nil, "", fmt.Errorf("model %q is not offered by the active source %s (available: %s)",
				model, p.subs.SourceLabel(), strings.Join(ids, ", "))
		}
	}
	return Client(&p.cfg, p.subs, id, provider, p.token), id, nil
}
