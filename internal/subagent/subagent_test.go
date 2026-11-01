package subagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/fleet"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/sessions"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// fakeLLM plays a subagent: on the first call it may ask for one tool, then
// it answers with text. delay makes every call take time (and honours ctx).
type fakeLLM struct {
	delay  time.Duration
	tool   string // tool to call first ("" — answer at once)
	args   string
	answer string
	block  bool // never answer: wait for ctx

	running atomic.Int32
	peak    atomic.Int32
}

func (f *fakeLLM) Stream(ctx context.Context, msgs []llm.AIMessage, _ []map[string]any, cb llm.StreamCallbacks) (*llm.StreamResult, error) {
	n := f.running.Add(1)
	defer f.running.Add(-1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	last := msgs[len(msgs)-1]
	if f.tool != "" && last.Role == "user" {
		return &llm.StreamResult{PromptTokens: 100, ToolCalls: []llm.ToolCall{{ID: "t1", Type: "function",
			Function: llm.ToolCallFunc{Name: f.tool, Arguments: f.args}}}}, nil
	}
	if cb.OnText != nil {
		cb.OnText(f.answer)
	}
	return &llm.StreamResult{Content: f.answer, PromptTokens: 200}, nil
}

func setup(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)
	return t.TempDir()
}

func req(title string) tools.SubagentRequest {
	return tools.SubagentRequest{Description: title, Prompt: "do " + title, Mode: "read", Model: "same",
		MaxIterations: tools.SubagentDefaultIterations, ToolCall: "call-1-0"}
}

// A foreground subagent: the answer comes back, the record ends "done" with
// its steps and tokens, and the transcript is a subagent session of the
// parent's chat.
func TestRun_ForegroundRecordAndSession(t *testing.T) {
	cwd := setup(t)
	os.WriteFile(filepath.Join(cwd, "a.txt"), []byte("hello"), 0o600)
	m := NewManager(nil)
	cli := &fakeLLM{tool: "Read", args: `{"path":"a.txt"}`, answer: "a.txt says hello"}
	r := m.Runner(Deps{Client: cli, Source: "Z.ai", Model: "glm", Cwd: cwd, ParentChat: "chat-1"})

	out, err := r.Run(context.Background(), req("read a"))
	if err != nil || out != "a.txt says hello" {
		t.Fatalf("out %q err %v", out, err)
	}
	recs := m.Fleet().Snapshot()
	if len(recs) != 1 {
		t.Fatalf("records %d", len(recs))
	}
	rec := recs[0]
	if rec.State != fleet.Done || rec.Iter != 2 || rec.LastTool != "Read" || rec.PromptTokens != 100 ||
		rec.ParentChat != "chat-1" || rec.ParentToolCall != "call-1-0" || rec.Title != "read a" ||
		rec.Source != "Z.ai" || rec.Model != "glm" || rec.Result != out || rec.FinishedAt == nil || rec.MaxIter != 15 {
		t.Errorf("record %+v", rec)
	}
	_, sess, err := m.Transcript(rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sess.IsSubagent() || sess.ParentID != "chat-1" || sess.State != "done" || sess.Title != "read a" {
		t.Errorf("session %+v", sess)
	}
	tr := TranscriptOf(sess, 100)
	var roles []string
	for _, l := range tr {
		roles = append(roles, l.Role)
	}
	if strings.Join(roles, ",") != "user,tool,tool_result,assistant" {
		t.Errorf("transcript roles %v", roles)
	}
	// The chat list does not show it.
	list, _ := sessions.List()
	for _, s := range list {
		if s.ID == sess.ID {
			t.Error("subagent session listed as a chat")
		}
	}
}

// background=true returns at once; TaskWait gets the result, and then no
// automatic message is due.
func TestRun_BackgroundAndTaskWait(t *testing.T) {
	cwd := setup(t)
	m := NewManager(nil)
	var notified atomic.Int32
	cli := &fakeLLM{delay: 150 * time.Millisecond, answer: "bg result"}
	r := m.Runner(Deps{Client: cli, Cwd: cwd, ParentChat: "chat-1",
		Notify: func(string) { notified.Add(1) }})

	rq := req("long scan")
	rq.Background = true
	start := time.Now()
	out, err := r.Run(context.Background(), rq)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Error("background Run blocked")
	}
	id := m.Fleet().Snapshot()[0].ID
	if !strings.Contains(out, id) {
		t.Errorf("the note does not carry the id: %q", out)
	}
	res, err := r.Wait(context.Background(), id)
	if err != nil || !strings.Contains(res, "bg result") || !strings.Contains(res, "long scan") {
		t.Fatalf("wait: %q %v", res, err)
	}
	time.Sleep(20 * time.Millisecond)
	if notified.Load() != 0 {
		t.Error("an automatic message was announced although TaskWait took the result")
	}
	if got := m.TakeFinished("chat-1"); got != "" {
		t.Errorf("result delivered twice: %q", got)
	}
	// Waiting again: nothing left.
	if res, _ := r.Wait(context.Background(), "all"); !strings.Contains(res, "no background") {
		t.Errorf("second wait: %q", res)
	}
}

// Nobody waits: the host is told, and TakeFinished hands the result over
// exactly once.
func TestRun_BackgroundNotifiesParent(t *testing.T) {
	cwd := setup(t)
	m := NewManager(nil)
	got := make(chan string, 1)
	r := m.Runner(Deps{Client: &fakeLLM{answer: "done quickly"}, Cwd: cwd, ParentChat: "chat-9",
		Notify: func(p string) { got <- p }})
	rq := req("quick")
	rq.Background = true
	if _, err := r.Run(context.Background(), rq); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-got:
		if p != "chat-9" {
			t.Errorf("notified %q", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no notification")
	}
	if !m.HasFinished("chat-9") {
		t.Error("HasFinished")
	}
	note := m.TakeFinished("chat-9")
	if !strings.Contains(note, "done quickly") || !strings.Contains(note, "quick") {
		t.Errorf("note %q", note)
	}
	if m.TakeFinished("chat-9") != "" {
		t.Error("taken twice")
	}
}

// EXECAI_SUBAGENTS_PARALLEL: no more than the limit work at once; the rest
// wait in the queue with state "queued".
func TestManager_Semaphore(t *testing.T) {
	cwd := setup(t)
	t.Setenv("EXECAI_SUBAGENTS_PARALLEL", "2")
	m := NewManager(nil)
	cli := &fakeLLM{delay: 150 * time.Millisecond, answer: "ok"}
	r := m.Runner(Deps{Client: cli, Cwd: cwd, ParentChat: "c"})
	sawQueued := make(chan struct{}, 1)
	m.Fleet().MinGap = time.Millisecond
	m.Fleet().Subscribe(func(recs []fleet.Record) {
		for _, rec := range recs {
			if rec.State == fleet.Queued {
				select {
				case sawQueued <- struct{}{}:
				default:
				}
			}
		}
	})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := r.Run(context.Background(), req("job")); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if p := cli.peak.Load(); p != 2 {
		t.Errorf("peak concurrency %d, want 2", p)
	}
	select {
	case <-sawQueued:
	default:
		t.Error("no subagent was ever shown as queued")
	}
	if ParallelLimit() != 2 {
		t.Error("ParallelLimit ignores the environment")
	}
}

// askRecorder is a parent approver that knows about askers.
type askRecorder struct {
	mu    sync.Mutex
	who   []agent.Asker
	plain int
	fleet *fleet.Fleet
	state fleet.State
}

func (a *askRecorder) AskApprove(string, json.RawMessage, string) agent.ApproveDecision {
	a.mu.Lock()
	a.plain++
	a.mu.Unlock()
	return agent.ApproveOnce
}

func (a *askRecorder) AskApproveAs(_ context.Context, who agent.Asker, _ string, _ json.RawMessage, _ string) agent.ApproveDecision {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.who = append(a.who, who)
	if rec, ok := a.fleet.Get(who.ID); ok {
		a.state = rec.State
	}
	return agent.ApproveOnce
}

// A write subagent asks the PARENT's approver, marked with its title, and
// shows "waiting" meanwhile; the write happens after the yes.
func TestRun_WriteModeAsksParentWithTitle(t *testing.T) {
	cwd := setup(t)
	m := NewManager(nil)
	ap := &askRecorder{fleet: m.Fleet()}
	target := filepath.Join(cwd, "out.txt")
	args, _ := json.Marshal(map[string]string{"path": target, "content": "hi"})
	cli := &fakeLLM{tool: "Write", args: string(args), answer: "written"}
	r := m.Runner(Deps{Client: cli, Cwd: cwd, ParentChat: "c", Approver: ap})
	rq := req("write file")
	rq.Mode = "write"
	if _, err := r.Run(context.Background(), rq); err != nil {
		t.Fatal(err)
	}
	if len(ap.who) != 1 || ap.who[0].Title != "write file" || !strings.HasPrefix(ap.who[0].ID, "sa-") {
		t.Fatalf("asked as %+v", ap.who)
	}
	if ap.state != fleet.Waiting {
		t.Errorf("state while asking: %q", ap.state)
	}
	if data, _ := os.ReadFile(target); string(data) != "hi" {
		t.Error("the approved write did not happen")
	}
	if rec := m.Fleet().Snapshot()[0]; rec.State != fleet.Done || rec.Mode != "write" {
		t.Errorf("record %+v", rec)
	}
}

// A read subagent never asks anybody: what needs consent is refused.
func TestRun_ReadModeNeverAsks(t *testing.T) {
	cwd := setup(t)
	m := NewManager(nil)
	ap := &askRecorder{fleet: m.Fleet()}
	cli := &fakeLLM{tool: "Read", args: `{"path":"/etc/hostname"}`, answer: "could not read it"}
	r := m.Runner(Deps{Client: cli, Cwd: cwd, ParentChat: "c", Approver: ap})
	if _, err := r.Run(context.Background(), req("peek outside")); err != nil {
		t.Fatal(err)
	}
	if ap.plain != 0 || len(ap.who) != 0 {
		t.Errorf("a read subagent asked the person: %d %v", ap.plain, ap.who)
	}
}

// Stopping the parent stops its background subagents; the record and the
// session say "cancelled".
func TestCancelParent_StopsChildren(t *testing.T) {
	cwd := setup(t)
	m := NewManager(nil)
	r := m.Runner(Deps{Client: &fakeLLM{block: true}, Cwd: cwd, ParentChat: "p1"})
	rq := req("forever")
	rq.Background = true
	if _, err := r.Run(context.Background(), rq); err != nil {
		t.Fatal(err)
	}
	other := m.Runner(Deps{Client: &fakeLLM{block: true}, Cwd: cwd, ParentChat: "p2"})
	if _, err := other.Run(context.Background(), rq); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if n := m.CancelParent("p1"); n != 1 {
		t.Errorf("cancelled %d", n)
	}
	waitState(t, m, "p1", fleet.Cancelled)
	for _, rec := range m.Fleet().Snapshot() {
		if rec.ParentChat == "p2" && !rec.State.Active() {
			t.Error("another parent's subagent was stopped")
		}
		if rec.ParentChat == "p1" {
			_, sess, _ := m.Transcript(rec.ID)
			if sess.State != "cancelled" {
				t.Errorf("session state %q", sess.State)
			}
		}
	}
	// On exit everything still running ends cancelled.
	m.Shutdown(2 * time.Second)
	waitState(t, m, "p2", fleet.Cancelled)
	if _, err := other.Run(context.Background(), req("late")); err != ErrClosed {
		t.Errorf("after shutdown: %v", err)
	}
}

// Stop by the person: one subagent, with its own reason.
func TestStop_One(t *testing.T) {
	cwd := setup(t)
	m := NewManager(nil)
	r := m.Runner(Deps{Client: &fakeLLM{block: true}, Cwd: cwd, ParentChat: "p"})
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(context.Background(), req("stuck"))
		done <- err
	}()
	var id string
	for i := 0; i < 100 && id == ""; i++ {
		time.Sleep(5 * time.Millisecond)
		if recs := m.Fleet().Snapshot(); len(recs) > 0 {
			id = recs[0].ID
		}
	}
	if !m.Stop(id) {
		t.Fatal("Stop reported nothing to stop")
	}
	err := <-done
	if err == nil || !strings.Contains(err.Error(), "stopped by the person") {
		t.Errorf("err %v", err)
	}
	if m.Stop(id) {
		t.Error("stopping a finished subagent reported success")
	}
}

func TestRun_TimeoutAndCwd(t *testing.T) {
	cwd := setup(t)
	m := NewManager(nil)
	m.SetTimeout(50 * time.Millisecond)
	r := m.Runner(Deps{Client: &fakeLLM{block: true}, Cwd: cwd, ParentChat: "p"})
	_, err := r.Run(context.Background(), req("slow"))
	if err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Errorf("timeout: %v", err)
	}
	if rec := m.Fleet().Snapshot()[0]; rec.State != fleet.Failed {
		t.Errorf("state %q", rec.State)
	}

	os.Mkdir(filepath.Join(cwd, "sub"), 0o700)
	rq := req("x")
	rq.Cwd = "../"
	if _, err := r.Run(context.Background(), rq); err == nil {
		t.Error("cwd above the parent accepted")
	}
	if got, err := ResolveCwd(cwd, "sub"); err != nil || got != filepath.Join(cwd, "sub") {
		t.Errorf("sub: %q %v", got, err)
	}
	if _, err := ResolveCwd(cwd, "/"); err == nil {
		t.Error("absolute path outside accepted")
	}
	os.Symlink("/", filepath.Join(cwd, "escape"))
	if _, err := ResolveCwd(cwd, "escape"); err == nil {
		t.Error("symlink out of the project accepted")
	}
}

func waitState(t *testing.T, m *Manager, parent string, want fleet.State) {
	t.Helper()
	for i := 0; i < 300; i++ {
		for _, rec := range m.Fleet().Snapshot() {
			if rec.ParentChat == parent && rec.State == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never became %s: %+v", parent, want, m.Fleet().Snapshot())
}
