package ide

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
)

// Chat origins: "web" — started by a task from the ExecAI web chat; "user" —
// by the person in the panel.
const (
	originWeb  = "web"
	originUser = "user"
)

// Who answered a question asked in two places (ask_resolved.by): the web
// chat, the panel, or nobody — the turn was stopped or timed out.
const (
	resolvedByWeb    = "web"
	resolvedByPanel  = "panel"
	resolvedByCancel = "cancel"
)

// webTask is a task from the site running as a chat of the panel: the first
// turn of that chat is the task, and its outcome goes back to the web chat.
// After it the person may keep talking in the chat; those turns stay local.
type webTask struct {
	task   inbox.Task
	client *inbox.Client

	once sync.Once
	done chan struct{} // closed once the result has been delivered (or failed)
}

func newWebTask(t inbox.Task, cl *inbox.Client) *webTask {
	return &webTask{task: t, client: cl, done: make(chan struct{})}
}

// finish delivers the result exactly once. The editor closing reports the
// task first, and the turn that ends a moment later must not report it again.
func (w *webTask) finish(chunk, data string) {
	w.once.Do(func() {
		defer close(w.done)
		if err := w.client.PostResult(context.Background(), w.task.ID, chunk, data); err != nil {
			fmt.Fprintf(os.Stderr, "inbox: result of task %s not delivered: %v\n", inbox.Short(w.task.ID), err)
		}
	})
}

// withdrawTimeout bounds closing a question on the site after the panel has
// answered it.
const withdrawTimeout = 10 * time.Second

// dualApprover asks for a permission in two places at once — the panel
// (ideApprover) and the web chat (POST /tasks/<id>/ask) — because the person
// who started a task from the site may be at the phone or at the editor. The
// first answer decides; the other place is told to drop its question: the
// panel gets ask_resolved, the web chat gets the same answer posted by the
// agent itself (and, failing that, its question closes when the agent drops
// its ask request).
//
// One approver per task. "Session"/"exact" answered in the web chat mean
// "until the end of this task", as the web buttons say; answered in the panel
// they mean "until the end of this chat", as the panel buttons say.
type dualApprover struct {
	panel *ideApprover
	task  *webTask
	ctx   context.Context

	mu        sync.Mutex // guards the task maps (subagents ask in parallel)
	taskTools map[string]bool
	taskExact map[string]bool
}

func newDualApprover(ctx context.Context, panel *ideApprover, task *webTask) *dualApprover {
	return &dualApprover{panel: panel, task: task, ctx: ctx,
		taskTools: map[string]bool{}, taskExact: map[string]bool{}}
}

func (d *dualApprover) AskApprove(toolName string, args json.RawMessage, summary string) agent.ApproveDecision {
	return d.ask(d.ctx, nil, toolName, args, summary)
}

// allowed — answered "session"/"exact" before, in the panel or the web chat.
func (d *dualApprover) allowed(toolName, key string) bool {
	if d.panel.allowed(toolName, key) {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.taskTools[toolName] || d.taskExact[key]
}

// ask puts the question in both places; who is the subagent asking, nil for
// the task's own agent. It ends with the first answer, or with ctx (the
// subagent stopped) or the task's own context.
func (d *dualApprover) ask(ctx context.Context, who *agent.Asker, toolName string, args json.RawMessage, summary string) agent.ApproveDecision {
	key := agent.ExactKey(toolName, args)
	if d.allowed(toolName, key) {
		return agent.ApproveOnce
	}
	// Both contexts end the question: the task's and the asker's.
	qctx, cancelQ := context.WithCancel(d.ctx)
	defer cancelQ()
	stopAfter := context.AfterFunc(ctx, cancelQ)
	defer stopAfter()

	if !d.panel.c.acquireAsk(qctx) {
		return agent.ApproveDeny
	}
	defer d.panel.c.releaseAsk()
	if d.allowed(toolName, key) {
		return agent.ApproveOnce
	}

	id, panelCh, out := d.panel.question(toolName, summary)
	d.panel.show(withAgent(out, who))

	webSummary := summary
	if who != nil {
		// The web chat shows text only: the asker goes into the summary.
		webSummary = agent.AttributedSummary(*who, summary)
	}
	// The web request lives on the task's context, not the question's: when
	// the panel answers, the site's question is closed with that answer
	// first and the request dropped after (see below).
	webCtx, cancelWeb := context.WithCancel(d.ctx)
	webCh := make(chan string, 1)
	go func() {
		answer, _, err := d.task.client.Ask(webCtx, d.task.task.ID, toolName, webSummary, string(args))
		if err != nil {
			// The web chat could not ask (network, an older server): the
			// panel still decides. Not being able to ask in one place must
			// not mean "deny" while the other place can still answer.
			return
		}
		webCh <- answer
	}()

	var answer string
	fromWeb := false
	select {
	case answer = <-panelCh:
		d.panel.c.emit(Out{Type: "ask_resolved", AskID: id, By: resolvedByPanel})
		// Close the question on the site with the same answer, then drop our
		// request. Done in the background: the turn goes on at once.
		go func() {
			defer cancelWeb()
			ctx, cancel := context.WithTimeout(context.Background(), withdrawTimeout)
			defer cancel()
			_, _ = d.task.client.Withdraw(ctx, d.task.task.ID, answer)
		}()
	case answer = <-webCh:
		fromWeb = true
		cancelWeb()
		d.panel.c.srv.drop(id)
		d.panel.c.emit(Out{Type: "ask_resolved", AskID: id, By: resolvedByWeb})
	case <-qctx.Done():
		// Stopped or timed out: nobody answered — deny, and take the
		// question off the panel.
		cancelWeb()
		d.panel.c.srv.drop(id)
		d.panel.c.emit(Out{Type: "ask_resolved", AskID: id, By: resolvedByCancel})
		answer = inbox.AnswerDeny
	}
	d.panel.clear(id)

	if fromWeb {
		d.mu.Lock()
		defer d.mu.Unlock()
		return decide(answer, toolName, key, d.taskTools, d.taskExact)
	}
	return d.panel.decide(answer, toolName, key)
}
