package ide

// The editor's side of the "web → agent" channel.
//
// A task started from the ExecAI web chat used to need a separate process,
// `execai serve`. Now the panel itself can own this machine's inbox: a task
// from the site becomes a new chat of the panel (origin "web"), the whole
// turn is visible there and can be stopped or steered, and its answer goes
// back to the web chat.
//
// One owner per machine (internal/inbox lock): when the inbox is free the
// window takes it; when `execai serve` owns it the panel says so and offers
// "Accept here" (inbox_take: stop the daemon, take the lock); when another
// window owns it the panel only says which one. A window takes only the tasks
// of its own folder's project (and tasks outside any project); the others stay
// pending for whoever works there.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

// Inbox states as the panel sees them (event inbox, field state).
const (
	inboxListening   = "listening"     // this window accepts tasks from the site
	inboxOtherServe  = "other_serve"   // the background execai serve does
	inboxOtherIDE    = "other_ide"     // another editor window does
	inboxNoPlan      = "no_plan"       // the plan does not include tasks from the site
	inboxDisabled    = "disabled"      // accepting is turned off (--no-inbox)
	inboxNotLoggedIn = "not_logged_in" // not signed in to ExecAI as an agent
)

const (
	// inboxRecheck — how often a window that does not own the inbox looks
	// whether it became free (the daemon or the other window exited).
	inboxRecheck = 30 * time.Second
	// planRecheck — how often a refusal by the plan is re-asked: the person
	// may pay on the site, and the window should notice without a restart.
	planRecheck = 10 * time.Minute
	// takeWait — how long "Accept here" waits for execai serve to exit.
	takeWait = 30 * time.Second
	// skipSlowdown — the pause after a poll that offered only tasks left for
	// other owners (an older server offers them again at once).
	skipSlowdown    = 5 * time.Second
	skipSlowdownMax = 10 * time.Second
	// skipKeep — how long a left task is remembered (the server's task TTL).
	skipKeep = 24 * time.Hour
)

// WorkspaceRef — the project this folder is bound to.
type WorkspaceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// WorkspaceItem — a project in the bind picker (event workspaces).
type WorkspaceItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Default — the account's default project.
	Default bool `json:"default,omitempty"`
	// Active — bound to this folder.
	Active bool `json:"active,omitempty"`
	// Path — the directory bound to this project on this machine, if any.
	Path string `json:"path,omitempty"`
}

// inboxCtl owns the inbox for the panel: the lock, the poll loop and what the
// panel shows about it.
type inboxCtl struct {
	s        *server
	apiBase  string
	cwd      string
	gate     inbox.FeatureGate
	disabled bool
	pollWait time.Duration

	refreshMu sync.Mutex // one refresh at a time

	mu        sync.Mutex
	creds     config.Credentials
	state     string
	owner     *inbox.Owner
	bound     *WorkspaceRef
	release   func()
	pollStop  context.CancelFunc
	pollDone  chan struct{}
	noticed   map[string]bool
	planAt    time.Time // when the plan last refused
	lastEvent string    // the last inbox event sent, to skip repeats
	// bindCount — folders of this machine bound to web projects, as the
	// latest bindings request saw them; -1 = not known yet.
	bindCount int
}

func newInboxCtl(s *server, apiBase, cwd string, gate inbox.FeatureGate, disabled bool) *inboxCtl {
	return &inboxCtl{s: s, apiBase: apiBase, cwd: cwd, gate: gate, disabled: disabled,
		pollWait: inbox.PollWait, noticed: map[string]bool{}, bindCount: -1}
}

// bindings — how many folders of this machine are bound to web projects
// (-1 = not known).
func (c *inboxCtl) bindings() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bindCount
}

// setBindings records the count and, when it changed, asks for a fresh
// machine_state: the getting-started step "bind a folder" depends on it.
func (c *inboxCtl) setBindings(n int) {
	c.mu.Lock()
	changed := c.bindCount != n
	c.bindCount = n
	c.mu.Unlock()
	if changed {
		c.s.machineChanged()
	}
}

// recount re-reads the bindings after this window bound or unbound the
// folder.
func (c *inboxCtl) recount(ctx context.Context) {
	if binds, err := c.client().OwnBindings(ctx, c.credsSnapshot().AgentID); err == nil {
		c.setBindings(len(binds))
	}
}

// setCreds replaces the credentials (start, login, logout).
func (c *inboxCtl) setCreds(cr *config.Credentials) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cr == nil {
		c.creds = config.Credentials{}
		return
	}
	c.creds = *cr
}

func (c *inboxCtl) credsSnapshot() config.Credentials {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.creds
}

func (c *inboxCtl) client() *inbox.Client {
	cr := c.credsSnapshot()
	return inbox.NewClient(c.apiBase, cr.Token)
}

// snapshot — the inbox part of an event: state, owner, binding.
func (c *inboxCtl) snapshot() Out {
	c.mu.Lock()
	defer c.mu.Unlock()
	o := Out{Type: "inbox", State: c.state, BoundWorkspace: c.bound}
	if c.owner != nil && (c.state == inboxOtherServe || c.state == inboxOtherIDE) {
		o.OwnerPID = c.owner.PID
		o.OwnerCwd = c.owner.Cwd
	}
	return o
}

// decorate adds the inbox state to a state event.
func (c *inboxCtl) decorate(o *Out) {
	snap := c.snapshot()
	o.Inbox = snap.State
	o.OwnerPID, o.OwnerCwd, o.BoundWorkspace = snap.OwnerPID, snap.OwnerCwd, snap.BoundWorkspace
}

// emit sends the inbox event — a full snapshot every time (no
// bound_workspace means "not bound") — unless it repeats the last one.
func (c *inboxCtl) emit(force bool) {
	o := c.snapshot()
	if o.State == "" {
		return // not decided yet
	}
	sig := fmt.Sprintf("%s|%d|%s", o.State, o.OwnerPID, o.OwnerCwd)
	if o.BoundWorkspace != nil {
		sig += "|" + o.BoundWorkspace.ID + "|" + o.BoundWorkspace.Name
	}
	c.mu.Lock()
	same := sig == c.lastEvent
	c.lastEvent = sig
	c.mu.Unlock()
	if same && !force {
		return
	}
	c.s.emit(o)
}

// refreshOpts — what a refresh may do.
type refreshOpts struct {
	binding bool // re-read which project this folder is bound to (network)
	force   bool // send the inbox event even when nothing changed
	// ask — the plan may be asked from billing now. my_plan may renew
	// (charge) an expired period, so only the person's actions and the
	// ExecAI source ask; background re-checks use the saved answer.
	ask bool
}

// refresh settles the state again: who owns the inbox, whether the plan
// allows it, whether to (still) listen.
func (c *inboxCtl) refresh(ctx context.Context, o refreshOpts) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	state, owner := c.settle(ctx, o.ask)
	var bound *WorkspaceRef
	boundKnown := false
	if o.binding {
		bound, boundKnown = c.fetchBound(ctx)
	}
	c.mu.Lock()
	c.state, c.owner = state, owner
	if boundKnown {
		c.bound = bound
	}
	if state == inboxNotLoggedIn {
		c.bound = nil
	}
	c.mu.Unlock()
	c.emit(o.force)
}

// settle is the state machine of refresh; it takes or gives up the lock.
func (c *inboxCtl) settle(ctx context.Context, ask bool) (string, *inbox.Owner) {
	cr := c.credsSnapshot()
	switch {
	case c.isDisabled():
		c.stopListening()
		return inboxDisabled, nil
	case cr.Token == "" || cr.AgentID == "":
		// Tasks are addressed to an agent session; a browser token or no
		// login at all has no inbox.
		c.stopListening()
		return inboxNotLoggedIn, nil
	}
	if !inbox.Allowed(ctx, c.gate, ask) {
		// No point holding the lock: the daemon may take it if the person
		// runs one, and it answers tasks with the same refusal.
		c.stopListening()
		c.mu.Lock()
		c.planAt = time.Now()
		c.mu.Unlock()
		return inboxNoPlan, nil
	}
	if c.listening() {
		return inboxListening, nil
	}
	release, err := inbox.Acquire(inbox.KindIDE, c.apiBase, c.cwd)
	if err != nil {
		var locked *inbox.LockedError
		if errors.As(err, &locked) {
			o := locked.Owner
			if o.Kind == inbox.KindIDE {
				return inboxOtherIDE, &o
			}
			return inboxOtherServe, &o
		}
		fmt.Fprintln(os.Stderr, "inbox: lock:", err)
		c.s.emit(Out{Type: "error", Text: err.Error()})
		return inboxDisabled, nil
	}
	if !c.startListening(release) {
		return inboxDisabled, nil // the panel closed meanwhile
	}
	return inboxListening, nil
}

func (c *inboxCtl) isDisabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disabled
}

// fetchBound finds the project bound to this folder. The second value is
// false when it could not be found out (network): keep what is known.
func (c *inboxCtl) fetchBound(ctx context.Context) (*WorkspaceRef, bool) {
	cr := c.credsSnapshot()
	if cr.Token == "" {
		c.setBindings(-1)
		return nil, true
	}
	cl := c.client()
	binds, err := cl.OwnBindings(ctx, cr.AgentID)
	if err != nil {
		return nil, false
	}
	c.setBindings(len(binds))
	wsID := ""
	for _, b := range binds {
		if inbox.SamePath(b.LocalPath, c.cwd) {
			wsID = b.WorkspaceID
			break
		}
	}
	if wsID == "" {
		return nil, true
	}
	ref := &WorkspaceRef{ID: wsID, Name: wsID}
	if wss, err := cl.Workspaces(ctx); err == nil {
		for _, w := range wss {
			if w.ID == wsID {
				ref.Name = w.Name
			}
		}
	}
	return ref, true
}

func (c *inboxCtl) listening() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.release != nil
}

// startListening starts the poll loop with the lock just taken. False — the
// panel is closing (shutdown came in between): the lock goes straight back.
func (c *inboxCtl) startListening(release func()) bool {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	c.mu.Lock()
	if c.disabled {
		c.mu.Unlock()
		cancel()
		release()
		return false
	}
	c.release, c.pollStop, c.pollDone = release, cancel, done
	c.mu.Unlock()
	go func() {
		defer close(done)
		c.pollLoop(ctx)
	}()
	return true
}

// stopListening ends the poll loop and gives the lock back. Never called
// from the poll loop itself (it would wait for its own exit).
func (c *inboxCtl) stopListening() {
	c.mu.Lock()
	release, stop, done := c.release, c.pollStop, c.pollDone
	c.release, c.pollStop, c.pollDone = nil, nil, nil
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
	if release != nil {
		release()
	}
}

// shutdown gives the inbox up for good (the editor closed the panel).
func (c *inboxCtl) shutdown() {
	c.mu.Lock()
	c.disabled = true
	c.mu.Unlock()
	c.stopListening()
}

// take is "Accept here": stop the background execai serve and take the
// inbox. Another window is never stopped from here — its chats would die.
func (c *inboxCtl) take(ctx context.Context) {
	c.refresh(ctx, refreshOpts{ask: true})
	snap := c.snapshot()
	switch snap.State {
	case inboxOtherServe:
		if err := inbox.StopServe(takeWait); err != nil {
			c.s.emit(Out{Type: "error", Text: err.Error()})
		}
		c.refresh(ctx, refreshOpts{force: true, ask: true})
		if c.snapshot().State == inboxListening {
			c.s.emit(Out{Type: "notice", Text: i18n.T("inbox.ide.taken")})
		}
	case inboxOtherIDE:
		c.s.emit(Out{Type: "notice", Text: i18n.Tf("inbox.ide.anotherWindow", snap.OwnerCwd)})
		c.emit(true)
	case inboxNoPlan:
		c.s.emit(Out{Type: "notice", Text: inbox.PlanRefusal()})
		c.emit(true)
	default:
		c.emit(true)
	}
}

// watch re-checks the state while the window does not own the inbox: the
// owner may exit, the person may pay for the plan.
func (c *inboxCtl) watch(ctx context.Context) {
	t := time.NewTicker(inboxRecheck)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		c.mu.Lock()
		state, planAt := c.state, c.planAt
		c.mu.Unlock()
		switch {
		case state == inboxOtherServe || state == inboxOtherIDE:
			c.refresh(ctx, refreshOpts{ask: inbox.Background()})
		case state == inboxNoPlan && time.Since(planAt) >= planRecheck:
			c.refresh(ctx, refreshOpts{ask: inbox.Background()})
		}
	}
}

// pollLoop takes tasks while the window owns the inbox.
func (c *inboxCtl) pollLoop(ctx context.Context) {
	backoff := time.Second
	slow := skipSlowdown
	skip := map[string]time.Time{} // tasks left for another owner
	for ctx.Err() == nil {
		cl := c.client()
		tasks, err := cl.Poll(ctx, c.pollWait, skipList(skip))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// 401 usually means the token was rotated by a login elsewhere on
			// this machine: re-read credentials.json before giving up.
			if inbox.IsUnauthorized(err) && c.reloadCreds() {
				backoff = time.Second
				continue
			}
			fmt.Fprintf(os.Stderr, "inbox: %v — retry in %s\n", err, backoff)
			if !sleepCtx(ctx, backoff) {
				return
			}
			if backoff < time.Minute {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		if len(tasks) == 0 {
			slow = skipSlowdown
			continue
		}
		take, placed := c.sort(ctx, cl, tasks, skip)
		if ctx.Err() != nil {
			return
		}
		if len(take) == 0 {
			// Only tasks for other owners: a server that knows skip does not
			// offer them again, an older one does at once — do not spin.
			if !sleepCtx(ctx, slow) {
				return
			}
			if slow < skipSlowdownMax {
				slow *= 2
			}
			continue
		}
		slow = skipSlowdown
		acked, err := cl.Ack(ctx, take)
		if err != nil {
			fmt.Fprintf(os.Stderr, "inbox: ack: %v\n", err)
			continue // still pending — they come back with the next poll
		}
		if len(acked) == 0 {
			continue
		}
		// Tasks from the site arrived: the person's action, billing may be
		// asked.
		if !inbox.Allowed(ctx, c.gate, true) {
			// The plan ended while listening: answer what is already ours,
			// then give the inbox up.
			for _, t := range acked {
				_ = cl.PostResult(ctx, t.ID, inbox.ChunkError, inbox.PlanRefusal())
			}
			go c.refresh(context.Background(), refreshOpts{})
			return
		}
		for _, t := range acked {
			if !c.deliver(ctx, cl, t, placed[t.ID]) {
				return
			}
		}
	}
}

// placement — where an acked task goes.
type placement struct {
	where inbox.Placement
	err   error
}

// sort splits offered tasks into those this window takes and those it leaves
// for another owner (remembered in skip; the panel hears about each once).
func (c *inboxCtl) sort(ctx context.Context, cl *inbox.Client, tasks []inbox.Task,
	skip map[string]time.Time) ([]inbox.Task, map[string]placement) {
	for id, at := range skip {
		if time.Since(at) > skipKeep {
			delete(skip, id)
		}
	}
	self := c.credsSnapshot().AgentID
	var binds []inbox.Binding
	bindsOK := true
	for _, t := range tasks {
		if t.WorkspaceID != "" {
			b, err := cl.OwnBindings(ctx, self)
			if err != nil {
				// Without bindings nothing project-bound can be placed: leave
				// those pending for the next poll, take the rest.
				fmt.Fprintf(os.Stderr, "inbox: bindings: %v\n", err)
				bindsOK = false
			}
			binds = b
			break
		}
	}
	take := make([]inbox.Task, 0, len(tasks))
	placed := map[string]placement{}
	for _, t := range tasks {
		if t.WorkspaceID != "" && !bindsOK {
			continue
		}
		where, dir, err := inbox.Place(t, binds, self, c.cwd)
		if where == inbox.Elsewhere {
			if _, seen := skip[t.ID]; !seen {
				skip[t.ID] = time.Now()
				c.s.emit(Out{Type: "notice", Text: i18n.Tf("inbox.ide.otherProject", dir)})
			}
			continue
		}
		placed[t.ID] = placement{where: where, err: err}
		take = append(take, t)
	}
	return take, placed
}

// deliver hands an acked task to the main loop as a new chat, or answers it
// with the reason it cannot run. False — the loop is ending.
func (c *inboxCtl) deliver(ctx context.Context, cl *inbox.Client, t inbox.Task, p placement) bool {
	if p.where == inbox.Broken {
		msg := i18n.T("inbox.task.notBound")
		if p.err != nil {
			msg = p.err.Error()
		}
		_ = cl.PostResult(ctx, t.ID, inbox.ChunkError, msg)
		return true
	}
	prompt := inbox.ExtractPrompt(t.Payload)
	if prompt == "" {
		_ = cl.PostResult(ctx, t.ID, inbox.ChunkError, i18n.T("inbox.task.empty"))
		return true
	}
	wt := newWebTask(t, cl)
	select {
	case c.s.userQ <- In{Type: "web_task", Text: prompt, auto: true, task: wt}:
		return true
	case <-ctx.Done():
		// The panel is closing: the task is ours already, so say so.
		wt.finish(inbox.ChunkError, i18n.T("inbox.task.editorClosed"))
		return false
	}
}

// reloadCreds re-reads credentials.json; true when it holds another token.
func (c *inboxCtl) reloadCreds() bool {
	fresh, err := config.LoadCredentials()
	if err != nil || fresh == nil || fresh.Token == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if fresh.Token == c.creds.Token {
		return false
	}
	c.creds = *fresh
	return true
}

func equalFoldTrim(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func joinNames(names []string) string { return strings.Join(names, ", ") }

func skipList(skip map[string]time.Time) []string {
	if len(skip) == 0 {
		return nil
	}
	out := make([]string, 0, len(skip))
	for id := range skip {
		out = append(out, id)
	}
	return out
}

// sleepCtx waits d; false when ctx ended first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// bind binds this folder to a project (by id or by name) and adds the agent
// to it — the same two halves as /project bind in the terminal.
func (c *inboxCtl) bind(ctx context.Context, idOrName string) {
	cr := c.credsSnapshot()
	if cr.Token == "" {
		c.s.emit(Out{Type: "error", Text: i18n.T("project.needLogin")})
		return
	}
	if idOrName == "" {
		c.s.emit(Out{Type: "error", Text: i18n.T("inbox.ide.bindWhich")})
		return
	}
	cl := c.client()
	wss, err := cl.Workspaces(ctx)
	if err != nil {
		c.s.emit(Out{Type: "error", Text: i18n.Tf("project.error", err)})
		return
	}
	var target *inbox.Workspace
	for i := range wss {
		if wss[i].ID == idOrName || equalFoldTrim(wss[i].Name, idOrName) {
			target = &wss[i]
			break
		}
	}
	if target == nil {
		names := make([]string, 0, len(wss))
		for _, w := range wss {
			names = append(names, w.Name)
		}
		c.s.emit(Out{Type: "error", Text: i18n.Tf("project.notFound", idOrName, joinNames(names))})
		return
	}
	if cr.AgentID != "" {
		// One folder holds one project's memory (internal/sync): another
		// project bound here keeps it.
		if binds, err := cl.OwnBindings(ctx, cr.AgentID); err == nil {
			if other := inbox.TakenBy(binds, target.ID, c.cwd, agentsync.SameFolder); other != "" {
				for _, w := range wss {
					if w.ID == other && w.Name != "" {
						other = w.Name
					}
				}
				c.s.emit(Out{Type: "error", Text: i18n.Tf("project.folderTaken", other, target.Name)})
				return
			}
		}
	}
	if err := cl.Bind(ctx, target.ID, c.cwd); err != nil {
		c.s.emit(Out{Type: "error", Text: i18n.Tf("project.error", err)})
		return
	}
	msg := i18n.Tf("project.bound", target.Name, agentsync.HomePath(c.cwd))
	if cr.AgentID != "" {
		// The second half: the agent must appear in the project itself,
		// otherwise the web chat does not see it and cannot switch it off.
		if err := cl.AddAgent(ctx, *target, cr.AgentID); err != nil {
			msg = i18n.Tf("project.boundNoTool", target.Name, agentsync.HomePath(c.cwd), err)
		}
	}
	c.mu.Lock()
	c.bound = &WorkspaceRef{ID: target.ID, Name: target.Name}
	c.mu.Unlock()
	c.s.emit(Out{Type: "notice", Text: msg})
	c.emit(true)
	c.recount(ctx)
}

// unbind removes this folder's binding and takes the agent out of the
// project, so the web chat does not keep a machine that answers nothing.
func (c *inboxCtl) unbind(ctx context.Context) {
	cr := c.credsSnapshot()
	if cr.Token == "" {
		c.s.emit(Out{Type: "error", Text: i18n.T("project.needLogin")})
		return
	}
	cl := c.client()
	binds, err := cl.OwnBindings(ctx, cr.AgentID)
	if err != nil {
		c.s.emit(Out{Type: "error", Text: i18n.Tf("project.error", err)})
		return
	}
	for _, b := range binds {
		if !inbox.SamePath(b.LocalPath, c.cwd) {
			continue
		}
		if err := cl.Unbind(ctx, b.WorkspaceID); err != nil {
			c.s.emit(Out{Type: "error", Text: i18n.Tf("project.error", err)})
			return
		}
		if cr.AgentID != "" {
			if wss, err := cl.Workspaces(ctx); err == nil {
				for _, w := range wss {
					if w.ID == b.WorkspaceID {
						_ = cl.RemoveAgent(ctx, w, cr.AgentID)
						break
					}
				}
			}
		}
		c.mu.Lock()
		c.bound = nil
		c.mu.Unlock()
		c.s.emit(Out{Type: "notice", Text: i18n.Tf("project.unbound", agentsync.HomePath(c.cwd))})
		c.emit(true)
		c.recount(ctx)
		return
	}
	c.s.emit(Out{Type: "notice", Text: i18n.Tf("project.notBound", c.cwd)})
	c.emit(true)
}

// listWorkspaces sends the account's projects for the bind picker.
func (c *inboxCtl) listWorkspaces(ctx context.Context) {
	cr := c.credsSnapshot()
	if cr.Token == "" {
		c.s.emit(Out{Type: "error", Text: i18n.T("project.needLogin")})
		return
	}
	cl := c.client()
	wss, err := cl.Workspaces(ctx)
	if err != nil {
		c.s.emit(Out{Type: "error", Text: i18n.Tf("project.error", err)})
		return
	}
	binds, _ := cl.OwnBindings(ctx, cr.AgentID) // the list is useful without them too
	path := map[string]string{}
	for _, b := range binds {
		path[b.WorkspaceID] = b.LocalPath
	}
	items := make([]WorkspaceItem, 0, len(wss))
	for _, w := range wss {
		items = append(items, WorkspaceItem{ID: w.ID, Name: w.Name, Default: w.IsDefault,
			Path: path[w.ID], Active: inbox.SamePath(path[w.ID], c.cwd)})
	}
	c.s.emit(Out{Type: "workspaces", Items: &items})
}
