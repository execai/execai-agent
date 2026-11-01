// Package subagent runs the Task tool's subagents for every host: the
// terminal, the editor panel (and `execai ui`), and tasks from the site in
// `execai serve`.
//
// One Manager per process owns the subagent slots (EXECAI_SUBAGENTS_PARALLEL,
// 4 by default — separate from the hosts' own turn slots), the registry of
// who is doing what (internal/fleet) and the bookkeeping of background runs.
// A host builds a Runner for every turn from that turn's snapshot (client,
// source, model, directory, approver) and installs it in the turn's context
// with tools.WithSubagentRunner — so a model switch in the middle of a turn
// does not reach subagents already started, and every host runs subagents
// the same way.
//
// Each subagent is its own agent loop with its own session on disk (Origin
// subagent, saved after every iteration), so the person can open what it
// did while it is still working. Its events never go to the parent's feed:
// the parent sees one Task card; everything else is in the fleet record.
package subagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/fleet"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/sessions"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// DefaultTimeout bounds one subagent. Every iteration is billed to the
// person's provider quota, and a subagent that keeps finding one more file
// to read must stop somewhere.
const DefaultTimeout = 5 * time.Minute

// DefaultParallel — how many subagents of one process work at the same time.
const DefaultParallel = 4

// ParallelLimit reads EXECAI_SUBAGENTS_PARALLEL.
func ParallelLimit() int {
	if v, err := strconv.Atoi(os.Getenv("EXECAI_SUBAGENTS_PARALLEL")); err == nil && v > 0 {
		return v
	}
	return DefaultParallel
}

// ModelPicker resolves the model parameter of a Task call ("fast" or an
// explicit ID) into a client; llmpick.Picker implements it.
type ModelPicker interface {
	Pick(model string) (llm.StreamingLLM, string, error)
}

// Deps is a turn's snapshot of what its subagents inherit from the parent.
type Deps struct {
	Client llm.StreamingLLM // the parent's client ("model: same")
	Source string           // source label for the record
	Model  string           // the parent's model ID
	Picker ModelPicker      // nil: only "same" is available
	Cwd    string           // the parent's directory; a subagent never goes above it
	// Approver — the parent's. A write subagent asks it, marked with its
	// title; nil means nobody can answer, and everything is refused.
	Approver agent.Approver
	// ParentChat — the parent's session (or task) ID.
	ParentChat string
	// Notify is called when a background subagent of ParentChat finishes
	// and nobody is waiting for it: the host should hand the parent the
	// result (TakeFinished) as soon as the parent is free.
	Notify func(parentChat string)
}

// Manager owns the subagents of one process.
type Manager struct {
	fleet   *fleet.Fleet
	sem     chan struct{}
	timeout time.Duration
	base    context.Context
	stopAll context.CancelFunc

	mu     sync.Mutex
	runs   map[string]*run
	closed bool
}

// run is one subagent's bookkeeping.
type run struct {
	id, parent, title string
	background        bool
	cancel            context.CancelFunc
	done              chan struct{}

	// Set once done is closed.
	state  fleet.State
	result string
	err    error

	// Under Manager.mu.
	collected bool // the parent got the result (Task, TaskWait, an automatic message)
	waiters   int  // TaskWait calls blocked on it
	stopped   bool // stopped by the person
}

// NewManager makes a manager reporting to f (a new registry when nil).
func NewManager(f *fleet.Fleet) *Manager {
	if f == nil {
		f = fleet.New()
	}
	base, cancel := context.WithCancel(context.Background())
	return &Manager{
		fleet: f, sem: make(chan struct{}, ParallelLimit()), timeout: DefaultTimeout,
		base: base, stopAll: cancel, runs: map[string]*run{},
	}
}

// SetLimit changes how many subagents run at once. Call before first use.
func (m *Manager) SetLimit(n int) {
	if n > 0 {
		m.sem = make(chan struct{}, n)
	}
}

// SetTimeout changes the per-subagent time limit. Call before first use.
func (m *Manager) SetTimeout(d time.Duration) {
	if d > 0 {
		m.timeout = d
	}
}

// Fleet returns the registry the manager reports to.
func (m *Manager) Fleet() *fleet.Fleet { return m.fleet }

// Runner returns the tools.SubagentRunner for one turn of a parent.
func (m *Manager) Runner(d Deps) *Runner { return &Runner{m: m, d: d} }

// Runner runs subagents for one turn of one parent.
type Runner struct {
	m *Manager
	d Deps
}

var _ tools.SubagentRunner = (*Runner)(nil)

// ErrClosed — the process is shutting down.
var ErrClosed = errors.New("subagents are shutting down")

func newID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("sa-%x", time.Now().UnixNano()&0xffffffff)
	}
	return "sa-" + hex.EncodeToString(b)
}

// Run starts a subagent. In the foreground it blocks until the subagent is
// done and returns its answer; with Background it returns a note with the id
// at once.
func (r *Runner) Run(ctx context.Context, req tools.SubagentRequest) (string, error) {
	d := r.d
	cwd, err := ResolveCwd(d.Cwd, req.Cwd)
	if err != nil {
		return "", err
	}
	cli, modelID := d.Client, d.Model
	if req.Model != "" && req.Model != "same" {
		if d.Picker == nil {
			return "", errors.New(i18n.Tf("fleet.err.model", req.Model, d.Source))
		}
		c, id, err := d.Picker.Pick(req.Model)
		if err != nil {
			return "", err
		}
		cli, modelID = c, id
	}
	if cli == nil {
		return "", tools.ErrSubagentUnavailable
	}
	if req.MaxIterations <= 0 {
		req.MaxIterations = tools.SubagentDefaultIterations
	}
	if req.MaxIterations > tools.SubagentMaxIterations {
		req.MaxIterations = tools.SubagentMaxIterations
	}

	m := r.m
	rn := &run{id: newID(), parent: d.ParentChat, title: req.Description, background: req.Background,
		done: make(chan struct{})}
	var runCtx context.Context
	var cancel context.CancelFunc
	if req.Background {
		// Outlives the turn that started it; stopped with the parent's
		// chat (CancelParent), by the person (Stop) or on exit (Shutdown).
		runCtx, cancel = context.WithTimeout(m.base, m.timeout)
	} else {
		runCtx, cancel = context.WithTimeout(ctx, m.timeout)
		stopOnExit := context.AfterFunc(m.base, cancel)
		defer stopOnExit()
	}
	rn.cancel = cancel

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		cancel()
		return "", ErrClosed
	}
	m.runs[rn.id] = rn
	m.mu.Unlock()

	sess := sessions.New(modelID, d.Source, cwd)
	sess.Origin = sessions.OriginSubagent
	sess.ParentID = d.ParentChat
	sess.ParentToolCall = req.ToolCall
	sess.Title = req.Description
	sess.State = string(fleet.Queued)
	sess.Messages = []llm.AIMessage{{Role: "user", Content: req.Prompt}}
	_ = sess.Save()

	m.fleet.Add(fleet.Record{
		ID: rn.id, ParentChat: d.ParentChat, ParentToolCall: req.ToolCall, Title: req.Description,
		State: fleet.Queued, Mode: req.Mode, MaxIter: req.MaxIterations, Session: sess.ID,
		Source: d.Source, Model: modelID,
	})

	if req.Background {
		go func() {
			defer cancel()
			m.execute(runCtx, rn, req, d, cli, cwd, sess)
		}()
		return i18n.Tf("fleet.started", rn.id, rn.title, rn.id), nil
	}
	m.execute(runCtx, rn, req, d, cli, cwd, sess)
	cancel()
	m.mu.Lock()
	rn.collected = true
	m.mu.Unlock()
	if rn.state == fleet.Done {
		return rn.result, nil
	}
	return rn.result, fmt.Errorf("%s: %w", rn.title, rn.err)
}

// execute runs the subagent's loop and records the outcome. It closes rn.done.
func (m *Manager) execute(ctx context.Context, rn *run, req tools.SubagentRequest, d Deps,
	cli llm.StreamingLLM, cwd string, sess *sessions.Session) {
	st := &streamer{fleet: m.fleet, id: rn.id}
	var hist []llm.AIMessage
	var runErr error
	exhausted := false

	select {
	case m.sem <- struct{}{}:
		m.fleet.Update(rn.id, func(r *fleet.Record) { r.State = fleet.Running })
		write := req.Mode == "write"
		var ap agent.Approver = denyAll{}
		if write {
			ap = &attributed{ctx: ctx, parent: d.Approver, who: agent.Asker{ID: rn.id, Title: rn.title},
				fleet: m.fleet}
		}
		a := agent.New(cli, tools.ForSubagent(cwd, write), systemPrompt(write, cwd), ap, st)
		a.MaxIterations = req.MaxIterations
		a.Checkpoint = func(h []llm.AIMessage, promptTokens int) {
			sess.Messages = append([]llm.AIMessage(nil), h...)
			sess.State = string(fleet.Running)
			_ = sess.Save()
			if promptTokens > 0 {
				m.fleet.Update(rn.id, func(r *fleet.Record) { r.PromptTokens = promptTokens })
			}
		}
		hist, runErr = a.Run(ctx, nil, req.Prompt)
		exhausted = a.Exhausted
		<-m.sem
	case <-ctx.Done():
		runErr = ctx.Err()
	}

	answer := lastAssistantText(hist, exhausted)
	if answer == "" {
		answer = strings.TrimSpace(st.text())
	}
	state := fleet.Done
	var err error
	m.mu.Lock()
	stopped := rn.stopped
	m.mu.Unlock()
	switch {
	case ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded):
		state, err = fleet.Failed, errors.New(i18n.Tf("fleet.result.timeout", m.timeout))
	case ctx.Err() != nil:
		state = fleet.Cancelled
		err = errors.New(i18n.T("fleet.result.cancelled"))
		if stopped {
			err = errors.New(i18n.T("fleet.result.stopped"))
		}
	case runErr != nil:
		state, err = fleet.Failed, runErr
	case answer == "":
		state, err = fleet.Failed, errors.New(i18n.T("subagent.emptyResult"))
	}
	if state == fleet.Done && exhausted {
		answer += "\n\n" + i18n.Tf("fleet.result.limit", req.MaxIterations)
	}

	if len(hist) > 0 {
		sess.Messages = hist
	}
	sess.State = string(state)
	_ = sess.Save()

	now := time.Now().UTC()
	m.fleet.Update(rn.id, func(r *fleet.Record) {
		r.State = state
		r.Result = answer
		r.FinishedAt = &now
		if answer != "" {
			r.Preview = answer
		}
		if err != nil {
			r.Error = err.Error()
		}
	})

	rn.state, rn.result, rn.err = state, answer, err
	m.mu.Lock()
	notify := rn.background && !rn.collected && rn.waiters == 0
	m.mu.Unlock()
	close(rn.done)
	if notify && d.Notify != nil {
		d.Notify(rn.parent)
	}
}

// lastAssistantText returns the subagent's conclusion: the text of the last
// assistant message that has any. With exhausted, the loop's own "iteration
// limit" hint at the end is skipped.
func lastAssistantText(history []llm.AIMessage, exhausted bool) string {
	end := len(history)
	if exhausted && end > 0 && history[end-1].Role == "assistant" {
		end--
	}
	for i := end - 1; i >= 0; i-- {
		if history[i].Role != "assistant" {
			continue
		}
		if body := strings.TrimSpace(llm.ContentText(history[i].Content)); body != "" {
			return body
		}
	}
	return ""
}

// Wait blocks until the given background subagent of this parent ("all": every
// one not handed to the parent yet) is done, and returns its result. What
// is returned here is not delivered again as an automatic message.
func (r *Runner) Wait(ctx context.Context, id string) (string, error) {
	m := r.m
	m.mu.Lock()
	var targets []*run
	if id == "" || id == "all" {
		for _, rn := range m.runs {
			if rn.parent == r.d.ParentChat && rn.background && !rn.collected {
				targets = append(targets, rn)
			}
		}
	} else if rn := m.runs[id]; rn != nil && rn.parent == r.d.ParentChat {
		targets = append(targets, rn)
	}
	for _, rn := range targets {
		rn.waiters++
	}
	m.mu.Unlock()
	if len(targets) == 0 {
		if id == "" || id == "all" {
			return i18n.T("fleet.wait.none"), nil
		}
		return "", errors.New(i18n.Tf("fleet.wait.unknown", id))
	}
	sortRuns(targets)
	// release gives up waiting (the parent's turn was stopped): a subagent
	// that finished meanwhile must still reach the parent, now as an
	// automatic message.
	release := func() {
		m.mu.Lock()
		orphaned := false
		for _, rn := range targets {
			rn.waiters--
			select {
			case <-rn.done:
				if rn.waiters == 0 && !rn.collected {
					orphaned = true
				}
			default:
			}
		}
		m.mu.Unlock()
		if orphaned && r.d.Notify != nil {
			r.d.Notify(r.d.ParentChat)
		}
	}
	for _, rn := range targets {
		select {
		case <-rn.done:
		case <-ctx.Done():
			release()
			return "", ctx.Err()
		}
	}
	m.mu.Lock()
	for _, rn := range targets {
		rn.waiters--
		rn.collected = true
	}
	m.mu.Unlock()
	return formatResults(targets), nil
}

// Drain waits until every background subagent of this parent not yet handed
// to it is done and returns their results as one message ("" when there
// are none). Hosts with no later turn to deliver an automatic message — a
// task from the site — call it before they report the task.
func (r *Runner) Drain(ctx context.Context) string {
	m := r.m
	m.mu.Lock()
	var targets []*run
	for _, rn := range m.runs {
		if rn.parent == r.d.ParentChat && rn.background && !rn.collected {
			targets = append(targets, rn)
			rn.waiters++
		}
	}
	m.mu.Unlock()
	if len(targets) == 0 {
		return ""
	}
	sortRuns(targets)
	for _, rn := range targets {
		select {
		case <-rn.done:
		case <-ctx.Done():
		}
	}
	m.mu.Lock()
	var ready []*run
	for _, rn := range targets {
		rn.waiters--
		select {
		case <-rn.done:
			if !rn.collected {
				rn.collected = true
				ready = append(ready, rn)
			}
		default:
		}
	}
	m.mu.Unlock()
	if len(ready) == 0 {
		return ""
	}
	return i18n.T("fleet.note.header") + "\n\n" + formatResults(ready)
}

// TakeFinished returns the automatic message for the background subagents of
// parent that finished and were not handed to it yet, and marks them handed.
// "" — nothing to deliver.
func (m *Manager) TakeFinished(parent string) string {
	m.mu.Lock()
	var ready []*run
	for _, rn := range m.runs {
		if rn.parent != parent || !rn.background || rn.collected {
			continue
		}
		select {
		case <-rn.done:
			rn.collected = true
			ready = append(ready, rn)
		default:
		}
	}
	m.mu.Unlock()
	if len(ready) == 0 {
		return ""
	}
	sortRuns(ready)
	return i18n.T("fleet.note.header") + "\n\n" + formatResults(ready)
}

// HasFinished reports whether parent has background results to take.
func (m *Manager) HasFinished(parent string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rn := range m.runs {
		if rn.parent != parent || !rn.background || rn.collected {
			continue
		}
		select {
		case <-rn.done:
			return true
		default:
		}
	}
	return false
}

func sortRuns(rs []*run) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].id < rs[j].id })
}

// formatResults renders finished runs for the parent model (and the person,
// who sees the automatic message in the feed).
func formatResults(rs []*run) string {
	var b strings.Builder
	for i, rn := range rs {
		if i > 0 {
			b.WriteString("\n\n")
		}
		body := rn.result
		if rn.err != nil {
			if body != "" {
				body = rn.err.Error() + "\n\n" + body
			} else {
				body = rn.err.Error()
			}
		}
		b.WriteString(i18n.Tf("fleet.note.item", rn.title, rn.id, StateLabel(rn.state)))
		b.WriteString("\n")
		b.WriteString(body)
	}
	return b.String()
}

// StateLabel — the state in the person's language.
func StateLabel(s fleet.State) string {
	return i18n.T("fleet.state." + string(s))
}

// Stop cancels one subagent. It reports whether it was still at work.
func (m *Manager) Stop(id string) bool {
	m.mu.Lock()
	rn := m.runs[id]
	if rn == nil {
		m.mu.Unlock()
		return false
	}
	select {
	case <-rn.done:
		m.mu.Unlock()
		return false
	default:
	}
	rn.stopped = true
	cancel := rn.cancel
	m.mu.Unlock()
	cancel()
	return true
}

// CancelParent stops every subagent of a parent — the person stopped the
// parent's turn. Returns how many were still at work.
func (m *Manager) CancelParent(parent string) int {
	m.mu.Lock()
	var cancels []context.CancelFunc
	for _, rn := range m.runs {
		if rn.parent != parent {
			continue
		}
		select {
		case <-rn.done:
		default:
			cancels = append(cancels, rn.cancel)
		}
	}
	m.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	return len(cancels)
}

// Shutdown stops every subagent and waits up to wait for them to record
// "cancelled" in their sessions and records. New ones are refused.
func (m *Manager) Shutdown(wait time.Duration) {
	m.mu.Lock()
	m.closed = true
	var pending []*run
	for _, rn := range m.runs {
		select {
		case <-rn.done:
		default:
			pending = append(pending, rn)
		}
	}
	m.mu.Unlock()
	m.stopAll()
	for _, rn := range pending {
		rn.cancel()
	}
	deadline := time.After(wait)
	for _, rn := range pending {
		select {
		case <-rn.done:
		case <-deadline:
			return
		}
	}
}

// Transcript returns a subagent's record and its saved session. id is the
// subagent ID; a session ID works too (a subagent of an earlier process).
func (m *Manager) Transcript(id string) (fleet.Record, *sessions.Session, error) {
	rec, ok := m.fleet.Get(id)
	sid := id
	if ok {
		sid = rec.Session
	}
	sess, err := sessions.Load(sid)
	if err != nil {
		if !ok {
			return rec, nil, errors.New(i18n.Tf("fleet.wait.unknown", id))
		}
		return rec, nil, err
	}
	if !ok {
		rec = fleet.Record{ID: id, Title: sess.Title, State: fleet.State(sess.State), Session: sess.ID,
			ParentChat: sess.ParentID, ParentToolCall: sess.ParentToolCall, Model: sess.Model,
			StartedAt: sess.StartedAt, UpdatedAt: sess.UpdatedAt}
	}
	return rec, sess, nil
}

// ResolveCwd returns the subagent's directory: the parent's, or want inside
// it. Going above the parent's directory (.., an absolute path elsewhere, a
// symlink out) is refused.
func ResolveCwd(parent, want string) (string, error) {
	if parent == "" {
		parent, _ = os.Getwd()
	}
	if strings.TrimSpace(want) == "" || want == "." {
		return parent, nil
	}
	p := want
	if !filepath.IsAbs(p) {
		p = filepath.Join(parent, p)
	}
	p = filepath.Clean(p)
	if !within(parent, p) {
		return "", errors.New(i18n.Tf("fleet.err.cwdOutside", want, parent))
	}
	fi, err := os.Stat(p)
	if err != nil || !fi.IsDir() {
		return "", errors.New(i18n.Tf("fleet.err.cwdMissing", want))
	}
	// A symlink inside the project may point anywhere.
	realParent, err1 := filepath.EvalSymlinks(parent)
	realP, err2 := filepath.EvalSymlinks(p)
	if err1 == nil && err2 == nil && !within(realParent, realP) {
		return "", errors.New(i18n.Tf("fleet.err.cwdOutside", want, parent))
	}
	return p, nil
}

func within(parent, p string) bool {
	rel, err := filepath.Rel(parent, p)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
