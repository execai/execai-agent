package machine

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
	"github.com/velesbsdllc/agent-vbai/internal/version"
)

// BindingsTimeout bounds the one network question of a snapshot.
const BindingsTimeout = 4 * time.Second

// Local gathers what this machine knows about itself without the network:
// sync.json, the inbox lock, the marks and the source checks. The caller
// passes what it holds in memory (credentials, sources, the plan answer);
// Bindings stays -1 until CountBindings fills it.
func Local(apiBase string, creds *config.Credentials, subs *subscriptions.Store, p *plan.Plan) Inputs {
	host, _ := os.Hostname()
	in := Inputs{
		Creds:    creds,
		APIBase:  apiBase,
		Hostname: host,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Version:  version.Get(),
		Plan:     p,
		Subs:     subs,
		Checks:   LoadChecks(),
		Listener: inbox.CurrentOwner(),
		Bindings: -1,
		Marks:    LoadMarks(),
		Now:      time.Now(),
	}
	if paths, err := agentsync.DefaultPaths(); err == nil {
		if st, err := agentsync.LoadState(paths); err == nil {
			in.Sync = st
		}
	}
	return in
}

// CountBindings asks the server how many folders of this machine are bound
// to web projects; -1 when it cannot tell (signed out, offline).
func CountBindings(ctx context.Context, apiBase, token string) int {
	if token == "" {
		return -1
	}
	ctx, cancel := context.WithTimeout(ctx, BindingsTimeout)
	defer cancel()
	binds, err := inbox.NewClient(apiBase, token).Bindings(ctx)
	if err != nil {
		return -1
	}
	return len(binds)
}
