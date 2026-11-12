package ide

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// syncBuf is the protocol stdout of a test server, safe to read while
// goroutines write.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *syncBuf) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

// events parses everything written so far.
func (w *syncBuf) events() []Out {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []Out
	for _, line := range strings.Split(strings.TrimSpace(w.b.String()), "\n") {
		var o Out
		if json.Unmarshal([]byte(line), &o) == nil {
			out = append(out, o)
		}
	}
	return out
}

// waitEvent waits for an event matching ok.
func (w *syncBuf) waitEvent(t *testing.T, ok func(Out) bool) Out {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, o := range w.events() {
			if ok(o) {
				return o
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("event not seen; got %+v", w.events())
	return Out{}
}

type fakeGate struct{ allow atomic.Bool }

func (g *fakeGate) Allows(context.Context, plan.Feature, bool) bool { return g.allow.Load() }

// budgetGate allows a number of checks, then refuses.
type budgetGate struct{ left atomic.Int32 }

func (g *budgetGate) Allows(context.Context, plan.Feature, bool) bool { return g.left.Add(-1) >= 0 }

func gateOf(allow bool) *fakeGate {
	g := &fakeGate{}
	g.allow.Store(allow)
	return g
}

func isolateIDE(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	return dir
}

var agentCreds = &config.Credentials{Token: "tok", AgentID: "agent-1", Email: "a@b"}

// newTestCtl — a controller over a test server, signed in as an agent.
func newTestCtl(t *testing.T, apiBase, cwd string, gate inbox.FeatureGate) (*inboxCtl, *syncBuf) {
	t.Helper()
	buf := &syncBuf{}
	s := newServer(json.NewEncoder(buf), cwd)
	c := newInboxCtl(s, apiBase, cwd, gate, false)
	c.pollWait = time.Second
	c.setCreds(agentCreds)
	t.Cleanup(c.shutdown)
	return c, buf
}

func liveOwner(t *testing.T, kind, cwd string) int {
	t.Helper()
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Skipf("no sh: %v", err)
	}
	done := make(chan struct{})
	go func() { _, _ = cmd.Process.Wait(); close(done) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
	p, _ := inbox.LockPath()
	line := strconv.Itoa(cmd.Process.Pid) + " 1780000000 https://x " + kind + " " + cwd
	if err := os.WriteFile(p, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

// idleAPI answers poll with nothing (after a short pause) and bindings with
// none — enough for a window that only has to hold the inbox.
func idleAPI(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agents-vbai/inbox/poll":
			select {
			case <-r.Context().Done():
			case <-time.After(50 * time.Millisecond):
			}
			_, _ = w.Write([]byte(`{"tasks":[]}`))
		case "/agents-vbai/workspaces/bindings":
			_, _ = w.Write([]byte(`{"bindings":[]}`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The six states of the inbox as the panel sees them.
func TestInbox_States(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(t *testing.T) int // returns the owner pid, if any
		creds    *config.Credentials
		allow    bool
		disabled bool
		want     string
		lockMine bool
	}{
		{"free inbox — this window listens", nil, agentCreds, true, false, inboxListening, true},
		{"live serve owns it", func(t *testing.T) int { return liveOwner(t, "serve", "-") }, agentCreds, true, false, inboxOtherServe, false},
		{"another window owns it", func(t *testing.T) int { return liveOwner(t, "ide", "/other repo") }, agentCreds, true, false, inboxOtherIDE, false},
		{"plan without tasks from the site", nil, agentCreds, false, false, inboxNoPlan, false},
		{"not signed in", nil, &config.Credentials{}, true, false, inboxNotLoggedIn, false},
		{"browser token, not an agent", nil, &config.Credentials{Token: "tok"}, true, false, inboxNotLoggedIn, false},
		{"turned off", nil, agentCreds, true, true, inboxDisabled, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolateIDE(t)
			api := idleAPI(t)
			pid := 0
			if c.setup != nil {
				pid = c.setup(t)
			}
			ctl, buf := newTestCtl(t, api.URL, t.TempDir(), gateOf(c.allow))
			ctl.disabled = c.disabled
			ctl.setCreds(c.creds)
			ctl.refresh(context.Background(), refreshOpts{binding: true, force: true})

			ev := buf.waitEvent(t, func(o Out) bool { return o.Type == "inbox" })
			if ev.State != c.want {
				t.Fatalf("state %q, want %q", ev.State, c.want)
			}
			if pid != 0 && ev.OwnerPID != pid {
				t.Errorf("owner_pid %d, want %d", ev.OwnerPID, pid)
			}
			if c.want == inboxOtherIDE && ev.OwnerCwd != "/other repo" {
				t.Errorf("owner_cwd %q", ev.OwnerCwd)
			}
			owner := inbox.CurrentOwner()
			mine := owner != nil && owner.PID == os.Getpid()
			if mine != c.lockMine {
				t.Errorf("lock held by this window: %v, want %v (owner %+v)", mine, c.lockMine, owner)
			}
			// The state event carries the same.
			var st Out
			ctl.decorate(&st)
			if st.Inbox != c.want {
				t.Errorf("state event inbox %q", st.Inbox)
			}
			ctl.shutdown()
			if o := inbox.CurrentOwner(); c.lockMine && o != nil && o.PID == os.Getpid() {
				t.Error("lock not given back on shutdown")
			}
		})
	}
}

// "Accept here": the window stops execai serve and takes the inbox.
func TestInbox_TakeFromServe(t *testing.T) {
	isolateIDE(t)
	api := idleAPI(t)
	pid := liveOwner(t, "serve", "-")
	ctl, buf := newTestCtl(t, api.URL, t.TempDir(), gateOf(true))
	ctl.refresh(context.Background(), refreshOpts{force: true})
	if ctl.snapshot().State != inboxOtherServe {
		t.Fatalf("state %q", ctl.snapshot().State)
	}
	ctl.take(context.Background())
	if st := ctl.snapshot().State; st != inboxListening {
		t.Fatalf("after take: %q; events %+v", st, buf.events())
	}
	if inbox.ProcessAlive(pid) {
		t.Error("serve still alive")
	}
	if o := inbox.CurrentOwner(); o == nil || o.PID != os.Getpid() || o.Kind != inbox.KindIDE {
		t.Errorf("lock now %+v", o)
	}
}

// Another window is never taken over from here.
func TestInbox_TakeNeverStopsAWindow(t *testing.T) {
	isolateIDE(t)
	api := idleAPI(t)
	pid := liveOwner(t, "ide", "/w")
	ctl, buf := newTestCtl(t, api.URL, t.TempDir(), gateOf(true))
	ctl.take(context.Background())
	if !inbox.ProcessAlive(pid) {
		t.Fatal("another window's agent was stopped")
	}
	buf.waitEvent(t, func(o Out) bool { return o.Type == "notice" && strings.Contains(o.Text, "/w") })
	if ctl.snapshot().State != inboxOtherIDE {
		t.Errorf("state %q", ctl.snapshot().State)
	}
}

// taskAPI is agents-vbai with a first poll offering tasks of every kind.
type taskAPI struct {
	srv     *httptest.Server
	mu      sync.Mutex
	polls   int
	skips   [][]string
	acked   []string
	results map[string]map[string]string
}

func newTaskAPI(t *testing.T, here, there string, tasks string) *taskAPI {
	a := &taskAPI{results: map[string]map[string]string{}}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch {
		case r.URL.Path == "/agents-vbai/inbox/poll":
			var body struct {
				Skip []string `json:"skip"`
			}
			_ = json.Unmarshal(raw, &body)
			a.mu.Lock()
			a.polls++
			n := a.polls
			a.skips = append(a.skips, body.Skip)
			a.mu.Unlock()
			if n == 1 {
				_, _ = w.Write([]byte(tasks))
				return
			}
			select {
			case <-r.Context().Done():
			case <-time.After(50 * time.Millisecond):
			}
			_, _ = w.Write([]byte(`{"tasks":[]}`))
		case r.URL.Path == "/agents-vbai/workspaces/bindings":
			_ = json.NewEncoder(w).Encode(map[string]any{"bindings": []map[string]string{
				{"agent_id": "agent-1", "workspace_id": "ws-here", "local_path": here},
				{"agent_id": "agent-1", "workspace_id": "ws-there", "local_path": there},
			}})
		case r.URL.Path == "/agents-vbai/inbox/ack":
			var body struct {
				IDs []string `json:"task_ids"`
			}
			_ = json.Unmarshal(raw, &body)
			a.mu.Lock()
			a.acked = append(a.acked, body.IDs...)
			a.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"acked": body.IDs})
		case strings.HasSuffix(r.URL.Path, "/result"):
			var body map[string]string
			_ = json.Unmarshal(raw, &body)
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/agents-vbai/tasks/"), "/result")
			a.mu.Lock()
			a.results[id] = body
			a.mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(a.srv.Close)
	return a
}

func (a *taskAPI) snapshot() (acked []string, results map[string]map[string]string, skips [][]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	res := map[string]map[string]string{}
	for k, v := range a.results {
		res[k] = v
	}
	return append([]string(nil), a.acked...), res, append([][]string(nil), a.skips...)
}

const offered = `{"tasks":[
	{"id":"t-here","workspace_id":"ws-here","payload":"{\"prompt\":\"fix the tests\"}"},
	{"id":"t-none","workspace_id":"","payload":"{\"prompt\":\"what time is it\"}"},
	{"id":"t-there","workspace_id":"ws-there","payload":"{\"prompt\":\"not mine\"}"},
	{"id":"t-unbound","workspace_id":"ws-unknown","payload":"{\"prompt\":\"nobody's\"}"}]}`

// The window takes the tasks of its folder's project and those outside any
// project; another project's task is left pending (not acked, skipped on the
// next poll, one notice in the panel); a task nobody here can run is taken
// and answered with the reason.
func TestInbox_TakesOnlyItsFolder(t *testing.T) {
	isolateIDE(t)
	i18n.SetLocale("en")
	here, there := t.TempDir(), t.TempDir()
	api := newTaskAPI(t, here, there, offered)
	ctl, buf := newTestCtl(t, api.srv.URL, here, gateOf(true))
	ctl.refresh(context.Background(), refreshOpts{force: true})
	if ctl.snapshot().State != inboxListening {
		t.Fatalf("state %q", ctl.snapshot().State)
	}

	got := map[string]string{}
	for len(got) < 2 {
		select {
		case in := <-ctl.s.userQ:
			if in.Type != "web_task" || in.task == nil || !in.auto {
				t.Fatalf("queued %+v", in)
			}
			got[in.task.task.ID] = in.Text
		case <-time.After(5 * time.Second):
			t.Fatalf("tasks delivered: %v", got)
		}
	}
	if got["t-here"] != "fix the tests" || got["t-none"] != "what time is it" {
		t.Errorf("delivered %v", got)
	}
	select {
	case in := <-ctl.s.userQ:
		t.Errorf("one more task delivered: %+v", in)
	case <-time.After(200 * time.Millisecond):
	}

	deadline := time.Now().Add(10 * time.Second) // generous: the full suite runs under load
	for {
		acked, results, skips := api.snapshot()
		skipped := false
		for _, s := range skips {
			if len(s) == 1 && s[0] == "t-there" {
				skipped = true
			}
		}
		if results["t-unbound"] != nil && skipped || time.Now().After(deadline) {
			for _, id := range acked {
				if id == "t-there" {
					t.Error("another project's task was acked")
				}
			}
			if len(acked) != 3 {
				t.Errorf("acked %v, want t-here, t-none, t-unbound", acked)
			}
			if r := results["t-unbound"]; r == nil || r["chunk_type"] != "error" {
				t.Errorf("unbound task result %v", r)
			}
			if !skipped {
				t.Errorf("next polls did not skip t-there: %v", skips)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	notices := 0
	for _, o := range buf.events() {
		if o.Type == "notice" && strings.Contains(o.Text, there) {
			notices++
		}
	}
	if notices != 1 {
		t.Errorf("notices about the other project: %d, want 1", notices)
	}
}

// The plan ended while the window was listening: what is already taken is
// answered with the plan text, and the window gives the inbox up.
func TestInbox_PlanGateRefusesAndReleases(t *testing.T) {
	isolateIDE(t)
	i18n.SetLocale("ru")
	defer i18n.SetLocale("en")
	here := t.TempDir()
	api := newTaskAPI(t, here, t.TempDir(), `{"tasks":[{"id":"t1","payload":"{\"prompt\":\"x\"}"}]}`)
	// The plan allows the first check (taking the inbox) and refuses after.
	gate := &budgetGate{}
	gate.left.Store(1)
	ctl, buf := newTestCtl(t, api.srv.URL, here, gate)
	ctl.refresh(context.Background(), refreshOpts{force: true})

	buf.waitEvent(t, func(o Out) bool { return o.Type == "inbox" && o.State == inboxNoPlan })
	_, results, _ := api.snapshot()
	r := results["t1"]
	if r == nil || r["chunk_type"] != "error" || !strings.Contains(r["data"], "«Работа» и «Макс»") ||
		!strings.Contains(r["data"], plan.SubscribeURL) {
		t.Errorf("result %v", r)
	}
	if o := inbox.CurrentOwner(); o != nil && o.PID == os.Getpid() {
		t.Error("lock kept without a plan")
	}
	select {
	case in := <-ctl.s.userQ:
		t.Errorf("refused task reached the panel: %+v", in)
	default:
	}
}

// askAPI is the web side of a permission question.
type askAPI struct {
	srv       *httptest.Server
	mode      string // "block" — wait for the person; "answer" — answer at once; "fail"
	answer    string
	cancelled atomic.Bool
	mu        sync.Mutex
	answered  map[string]string
}

func newAskAPI(t *testing.T, mode, answer string) *askAPI {
	a := &askAPI{mode: mode, answer: answer, answered: map[string]string{}}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/ask"):
			// Read the body as the real handler does: only then does the
			// server notice the client going away.
			_, _ = io.ReadAll(r.Body)
			switch a.mode {
			case "fail":
				w.WriteHeader(http.StatusBadGateway)
			case "answer":
				_ = json.NewEncoder(w).Encode(map[string]string{"answer": a.answer, "question_id": "q1"})
			default:
				// The person has not answered on the site; the request is
				// dropped by the agent when the panel wins.
				select {
				case <-r.Context().Done():
					a.cancelled.Store(true)
				case <-time.After(10 * time.Second):
				}
			}
		case r.URL.Path == "/agents-vbai/questions":
			_, _ = w.Write([]byte(`{"questions":[{"id":"q1","task_id":"T","status":"open"},
				{"id":"q2","task_id":"OTHER","status":"open"}]}`))
		case strings.HasSuffix(r.URL.Path, "/answer"):
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			a.mu.Lock()
			a.answered[r.URL.Path] = body["answer"]
			a.mu.Unlock()
			_, _ = w.Write([]byte(`{"accepted":true}`))
		}
	}))
	t.Cleanup(a.srv.Close)
	return a
}

func dualFor(t *testing.T, api *askAPI, ctx context.Context) (*dualApprover, *syncBuf) {
	buf := &syncBuf{}
	srv := newServer(json.NewEncoder(buf), "")
	c := srv.newChat("m", "p")
	wt := newWebTask(inbox.Task{ID: "T"}, inbox.NewClient(api.srv.URL, "tok"))
	return newDualApprover(ctx, c.ap, wt), buf
}

// The panel answers first: its answer decides, the web chat's question is
// closed with the same answer, and the agent's ask request is dropped.
func TestDualApprover_PanelFirst(t *testing.T) {
	api := newAskAPI(t, "block", "")
	d, buf := dualFor(t, api, context.Background())
	go answerFirstAsk(d.panel.c.srv, "session")
	if got := d.AskApprove("Bash", json.RawMessage(`{"command":"ls"}`), "ls"); got != agent.ApproveOnce {
		t.Fatalf("decision %v", got)
	}
	deadline := time.Now().Add(10 * time.Second) // generous: the full suite runs under load
	for time.Now().Before(deadline) && !api.cancelled.Load() {
		time.Sleep(10 * time.Millisecond)
	}
	if !api.cancelled.Load() {
		t.Error("the web ask request was not dropped")
	}
	api.mu.Lock()
	answered := api.answered
	api.mu.Unlock()
	if answered["/agents-vbai/questions/q1/answer"] != "session" || len(answered) != 1 {
		t.Errorf("web questions answered: %v — want only this task's, with the panel's answer", answered)
	}
	ev := buf.waitEvent(t, func(o Out) bool { return o.Type == "ask_resolved" })
	if ev.By != resolvedByPanel {
		t.Errorf("ask_resolved by %q", ev.By)
	}
	// "session" from the panel is the chat's scope: no new question.
	if got := d.AskApprove("Bash", json.RawMessage(`{"command":"pwd"}`), "pwd"); got != agent.ApproveOnce {
		t.Errorf("same tool after session: %v", got)
	}
}

// The site answers first: its answer decides, and the panel is told to drop
// its question (ask_resolved by=web); a late panel answer goes nowhere.
func TestDualApprover_WebFirst(t *testing.T) {
	api := newAskAPI(t, "answer", "exact")
	d, buf := dualFor(t, api, context.Background())
	args := json.RawMessage(`{"command":"go test"}`)
	if got := d.AskApprove("Bash", args, "go test"); got != agent.ApproveOnce {
		t.Fatalf("decision %v", got)
	}
	ask := buf.waitEvent(t, func(o Out) bool { return o.Type == "ask" })
	res := buf.waitEvent(t, func(o Out) bool { return o.Type == "ask_resolved" })
	if res.AskID != ask.ID || res.By != resolvedByWeb || res.Chat != d.panel.c.id {
		t.Errorf("ask_resolved %+v for ask %q", res, ask.ID)
	}
	d.panel.c.srv.answersMu.Lock()
	left := len(d.panel.c.srv.answers)
	d.panel.c.srv.answersMu.Unlock()
	if left != 0 {
		t.Errorf("the panel question is still waiting (%d)", left)
	}
	d.panel.c.srv.deliver(ask.ID, "deny") // a late click: harmless
	if d.panel.c.pendingAsk != nil {
		t.Error("the question is still pending in the chat")
	}
	// "exact" from the site holds for this task, not for the chat.
	if got := d.AskApprove("Bash", args, "go test"); got != agent.ApproveOnce {
		t.Errorf("same call after exact: %v", got)
	}
	if d.panel.sessionExact[agent.ExactKey("Bash", args)] {
		t.Error("the site's task-scoped answer leaked into the chat's scope")
	}
}

// The web chat cannot ask (an older server, a network error): the panel
// still decides — failing in one place must not mean deny.
func TestDualApprover_WebFailsPanelDecides(t *testing.T) {
	api := newAskAPI(t, "fail", "")
	d, _ := dualFor(t, api, context.Background())
	go answerFirstAsk(d.panel.c.srv, "once")
	if got := d.AskApprove("Write", json.RawMessage(`{"path":"x"}`), "x"); got != agent.ApproveOnce {
		t.Errorf("decision %v", got)
	}
}

// The task is stopped or times out while nobody answered: deny, and the
// panel drops the question (by=cancel).
func TestDualApprover_CancelDenies(t *testing.T) {
	api := newAskAPI(t, "block", "")
	ctx, cancel := context.WithCancel(context.Background())
	d, buf := dualFor(t, api, ctx)
	go func() {
		buf.waitEvent(t, func(o Out) bool { return o.Type == "ask" })
		cancel()
	}()
	if got := d.AskApprove("Bash", json.RawMessage(`{"command":"rm -rf /"}`), "rm"); got != agent.ApproveDeny {
		t.Errorf("decision %v", got)
	}
	if ev := buf.waitEvent(t, func(o Out) bool { return o.Type == "ask_resolved" }); ev.By != resolvedByCancel {
		t.Errorf("by %q", ev.By)
	}
}

// textLLM answers with text through the streaming callback.
type textLLM struct {
	text  string
	block bool
}

func (f *textLLM) Stream(ctx context.Context, _ []llm.AIMessage, _ []map[string]any,
	cb llm.StreamCallbacks) (*llm.StreamResult, error) {
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if cb.OnText != nil {
		cb.OnText(f.text)
	}
	return &llm.StreamResult{Content: f.text}, nil
}

// A task from the site is a turn of a "web" chat: the panel sees it start as
// such, and its answer goes back to the web chat.
func TestWebTask_TurnReportsResult(t *testing.T) {
	dir := isolateIDE(t)
	api := newTaskAPI(t, dir, dir, `{"tasks":[]}`)
	buf := &syncBuf{}
	srv := newServer(json.NewEncoder(buf), dir)
	c := srv.newChat("m", "p")
	c.sess.Origin = originWeb
	wt := newWebTask(inbox.Task{ID: "T1"}, inbox.NewClient(api.srv.URL, "tok"))
	deps := turnDeps{cli: &textLLM{text: "all green"}, maxIter: 3, registry: tools.ReadOnly(dir)}
	if !c.startTurn(In{Type: "web_task", Text: "run the tests", auto: true, task: wt}, deps) {
		t.Fatal("turn not started")
	}
	select {
	case <-wt.done:
	case <-time.After(5 * time.Second):
		t.Fatal("no result")
	}
	_, results, _ := api.snapshot()
	if r := results["T1"]; r["chunk_type"] != "final" || r["data"] != "all green" {
		t.Errorf("result %v", r)
	}
	start := buf.waitEvent(t, func(o Out) bool { return o.Type == "turn_start" })
	auto := buf.waitEvent(t, func(o Out) bool { return o.Type == "auto_user" })
	if start.Origin != originWeb || auto.Origin != originWeb || auto.Text != "run the tests" {
		t.Errorf("turn_start %+v, auto_user %+v", start, auto)
	}
	// The next turn in this chat is the person's own: nothing goes to the site.
	if c.pendingTask() != nil {
		t.Error("the task is still attached to the chat")
	}
}

// A task that runs past its limit is reported as such.
func TestWebTask_TimeoutReported(t *testing.T) {
	dir := isolateIDE(t)
	i18n.SetLocale("en")
	api := newTaskAPI(t, dir, dir, `{"tasks":[]}`)
	srv := newServer(json.NewEncoder(&syncBuf{}), dir)
	srv.taskTimeout = 100 * time.Millisecond
	c := srv.newChat("m", "p")
	wt := newWebTask(inbox.Task{ID: "T2"}, inbox.NewClient(api.srv.URL, "tok"))
	deps := turnDeps{cli: &textLLM{block: true}, maxIter: 3, registry: tools.ReadOnly(dir)}
	c.startTurn(In{Type: "web_task", Text: "x", auto: true, task: wt}, deps)
	select {
	case <-wt.done:
	case <-time.After(5 * time.Second):
		t.Fatal("no result")
	}
	_, results, _ := api.snapshot()
	if r := results["T2"]; r["chunk_type"] != "error" || !strings.Contains(r["data"], "did not finish") {
		t.Errorf("result %v", r)
	}
}

// Binding from the panel: the folder is bound to the project, the agent is
// added to it, and the panel learns the binding; unbinding undoes both.
func TestInbox_BindListUnbind(t *testing.T) {
	isolateIDE(t)
	i18n.SetLocale("en")
	cwd := t.TempDir()
	var mu sync.Mutex
	var calls []string
	bound := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(raw))
		b := bound
		mu.Unlock()
		switch r.URL.Path {
		case "/integrations-vbai/workspaces":
			_, _ = w.Write([]byte(`[{"id":"w1","name":"site","tools":[{"id":5,"tool_id":"agent","profile":"agent-1","enabled":true}]},
				{"id":"w2","name":"api","is_default":true,"tools":[]}]`))
		case "/agents-vbai/workspaces/bindings":
			if b == "" {
				_, _ = w.Write([]byte(`{"bindings":[]}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"bindings": []map[string]string{{"agent_id": "agent-1", "workspace_id": b, "local_path": cwd}}})
		case "/agents-vbai/workspaces/bind":
			var body map[string]string
			_ = json.Unmarshal(raw, &body)
			mu.Lock()
			bound = body["workspace_id"]
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		case "/agents-vbai/workspaces/unbind":
			mu.Lock()
			bound = ""
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	ctl, buf := newTestCtl(t, srv.URL, cwd, gateOf(false))
	ctl.refresh(context.Background(), refreshOpts{binding: true, force: true})

	ctl.bind(context.Background(), "api") // by name works too
	ev := buf.waitEvent(t, func(o Out) bool { return o.Type == "inbox" && o.BoundWorkspace != nil })
	if ev.BoundWorkspace.ID != "w2" || ev.BoundWorkspace.Name != "api" {
		t.Errorf("bound %+v", ev.BoundWorkspace)
	}
	mu.Lock()
	joined := strings.Join(calls, "\n")
	mu.Unlock()
	if !strings.Contains(joined, `POST /agents-vbai/workspaces/bind {"local_path":"`+cwd+`","workspace_id":"w2"}`) {
		t.Errorf("bind request missing:\n%s", joined)
	}
	if !strings.Contains(joined, "POST /integrations-vbai/workspaces/w2/tools") {
		t.Errorf("agent not added to the project:\n%s", joined)
	}

	ctl.listWorkspaces(context.Background())
	ws := buf.waitEvent(t, func(o Out) bool { return o.Type == "workspaces" })
	if ws.Items == nil || len(*ws.Items) != 2 || !(*ws.Items)[1].Active || (*ws.Items)[1].Path != cwd {
		t.Errorf("workspaces %+v", ws.Items)
	}

	n := len(buf.events())
	ctl.unbind(context.Background())
	deadline := time.Now().Add(10 * time.Second) // generous: the full suite runs under load
	for time.Now().Before(deadline) {
		evs := buf.events()
		if len(evs) > n && evs[len(evs)-1].Type == "inbox" {
			if evs[len(evs)-1].BoundWorkspace != nil {
				t.Errorf("still bound after unbind: %+v", evs[len(evs)-1])
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	joined = strings.Join(calls, "\n")
	mu.Unlock()
	if !strings.Contains(joined, "POST /agents-vbai/workspaces/unbind") {
		t.Errorf("unbind request missing:\n%s", joined)
	}
}

// Н-4 of the second loss review: one folder holds one project's memory. A
// folder this machine already bound to «api» is not bound to «site» from
// the panel — sync would mix the two projects' files on every computer —
// and the person is told which project has it; binding «api» again is fine.
func TestInbox_BindRefusesAFolderOfAnotherProject(t *testing.T) {
	isolateIDE(t)
	i18n.SetLocale("en")
	cwd := t.TempDir()
	var mu sync.Mutex
	var binds []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/integrations-vbai/workspaces":
			_, _ = w.Write([]byte(`[{"id":"w1","name":"site","tools":[]},{"id":"w2","name":"api","tools":[]}]`))
		case "/agents-vbai/workspaces/bindings":
			_ = json.NewEncoder(w).Encode(map[string]any{"bindings": []map[string]string{{"agent_id": "agent-1", "workspace_id": "w2", "local_path": cwd}}})
		case "/agents-vbai/workspaces/bind":
			mu.Lock()
			binds = append(binds, string(raw))
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	ctl, buf := newTestCtl(t, srv.URL, cwd, gateOf(false))

	ctl.bind(context.Background(), "site")
	ev := buf.waitEvent(t, func(o Out) bool { return o.Type == "error" })
	if !strings.Contains(ev.Text, "already bound to the project \"api\"") || !strings.Contains(ev.Text, "\"site\" was not bound") {
		t.Errorf("told: %q", ev.Text)
	}
	mu.Lock()
	n := len(binds)
	mu.Unlock()
	if n != 0 {
		t.Fatalf("the folder was bound to a second project: %v", binds)
	}

	ctl.bind(context.Background(), "api")
	buf.waitEvent(t, func(o Out) bool { return o.Type == "notice" && strings.Contains(o.Text, "\"api\" bound") })
	mu.Lock()
	defer mu.Unlock()
	if len(binds) != 1 || !strings.Contains(binds[0], `"workspace_id":"w2"`) {
		t.Errorf("binding the same project again: %v", binds)
	}
}
