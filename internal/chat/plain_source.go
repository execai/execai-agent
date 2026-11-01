package chat

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/velesbsdllc/agent-vbai/internal/catalog"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/llmpick"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// streamFunc — one plain request: history in, streamed text out.
type streamFunc func(ctx context.Context, history []llm.Message, onDelta func(string)) (string, error)

// plainBackend — what `execai run` and the plain REPL talk to. It follows the
// active source exactly like the TUI: an own subscription (picked by
// llmpick, no sign-in, no plan) or the ExecAI models (sign-in, their
// catalog, the plan gate). Plain mode stays plain: no tools, no system
// prompt — only the source choice is shared with the TUI.
type plainBackend struct {
	where, who string // banner of the REPL
	models     []llm.Model
	current    llm.Model
	// gate checks a call before the model; nil for an own source.
	gate func(context.Context) plan.Verdict
	open func(llm.Model) streamFunc
}

// newPlainBackend picks the backend for the active source. gateFirst — run
// asks the gate before fetching the ExecAI catalog, so a refusal needs no
// network beyond billing; the REPL asks it on every turn instead.
func newPlainBackend(ctx context.Context, cfg *config.Config, gateFirst bool) (*plainBackend, error) {
	subs, _ := subscriptions.Load()
	route := plan.RouteOf(subs)
	if route.Own {
		return ownPlainBackend(cfg, subs), nil
	}
	if route.Fallback != "" {
		// One line, before anything else: the person chose claude/codex and
		// would otherwise not know why the ExecAI rules apply.
		fmt.Fprintln(os.Stderr, plan.FallbackNote(route.Fallback))
	}
	cr, err := requireLogin()
	if err != nil {
		return nil, err
	}
	checker := plan.New(cfg.APIBase)
	gate := func(ctx context.Context) plan.Verdict {
		v := checker.Check(ctx, route)
		v.Fallback = "" // already said above
		return v
	}
	if gateFirst {
		if v := gate(ctx); !v.Allowed {
			return nil, errors.New(v.Message())
		}
	}
	models, err := llm.FetchModels(ctx, cfg.APIBase, cr.Token)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("plain.err.fetchModels"), err)
	}
	if len(models) == 0 {
		return nil, errors.New(i18n.T("plain.err.emptyModels"))
	}
	current := llm.PickDefault(models, cfg.SelectedModelID)
	if current == nil {
		return nil, errors.New(i18n.T("plain.err.pickModelEmpty"))
	}
	return &plainBackend{
		where: cfg.APIBase, who: cr.Email, models: models, current: *current, gate: gate,
		open: func(m llm.Model) streamFunc {
			return llm.New(cfg.APIBase, cr.Token, m.ID, m.Provider).Stream
		},
	}, nil
}

// ownPlainBackend — the active subscription, with the model chosen the way
// the TUI chooses it (pickForSource over the source's own catalog).
func ownPlainBackend(cfg *config.Config, subs *subscriptions.Store) *plainBackend {
	sub := *subs.ActiveSubscription()
	models := catalog.For(sub)
	current := pickForSource(models, llm.Model{ID: cfg.SelectedModelID}, sub.Provider)
	token, who := "", i18n.T("status.notLogin")
	if cr, _ := config.LoadCredentials(); cr != nil && cr.Token != "" {
		token, who = cr.Token, cr.Email
	}
	return &plainBackend{
		where: subs.SourceLabel(), who: who, models: models, current: current,
		open: func(m llm.Model) streamFunc {
			cli := llmpick.Client(cfg, subs, m.ID, m.Provider, token)
			return func(ctx context.Context, history []llm.Message, onDelta func(string)) (string, error) {
				msgs := make([]llm.AIMessage, 0, len(history))
				for _, h := range history {
					msgs = append(msgs, llm.AIMessage{Role: string(h.Role), Content: h.Content})
				}
				res, err := cli.Stream(ctx, msgs, nil, llm.StreamCallbacks{OnText: onDelta})
				if err != nil {
					return "", err
				}
				return res.Content, nil
			}
		},
	}
}
