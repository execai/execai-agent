package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// scriptedLLM answers with the prepared results in order.
type scriptedLLM struct {
	mu      sync.Mutex
	replies []*llm.StreamResult
	seen    [][]llm.AIMessage
}

func (s *scriptedLLM) Stream(_ context.Context, msgs []llm.AIMessage, _ []map[string]any, _ llm.StreamCallbacks) (*llm.StreamResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, append([]llm.AIMessage(nil), msgs...))
	if len(s.replies) == 0 {
		return &llm.StreamResult{Content: "done"}, nil
	}
	r := s.replies[0]
	s.replies = s.replies[1:]
	return r, nil
}

func taskCall(id, title string) llm.ToolCall {
	args, _ := json.Marshal(map[string]any{"description": title, "prompt": "look at " + title})
	return llm.ToolCall{ID: id, Type: "function", Function: llm.ToolCallFunc{Name: "Task", Arguments: string(args)}}
}

// sleepyRunner pretends every subagent takes d.
type sleepyRunner struct {
	d       time.Duration
	mu      sync.Mutex
	running int
	peak    int
	calls   []string // tool-call keys seen
}

func (r *sleepyRunner) Run(ctx context.Context, req tools.SubagentRequest) (string, error) {
	r.mu.Lock()
	r.running++
	if r.running > r.peak {
		r.peak = r.running
	}
	r.calls = append(r.calls, req.ToolCall)
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.running--
		r.mu.Unlock()
	}()
	select {
	case <-time.After(r.d):
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return "result of " + req.Description, nil
}

func (r *sleepyRunner) Wait(context.Context, string) (string, error) { return "", nil }

// idStreamer records start/end keys.
type idStreamer struct {
	mu     sync.Mutex
	starts []string
	ends   []string
	order  []string
}

func (s *idStreamer) OnText(string)                      {}
func (s *idStreamer) OnReasoning(string)                 {}
func (s *idStreamer) OnToolCall(string, json.RawMessage) { panic("ToolCallStreamer must be preferred") }
func (s *idStreamer) OnToolChunk(string, string)         {}
func (s *idStreamer) OnToolResult(string, string, error) { panic("ToolCallStreamer must be preferred") }
func (s *idStreamer) OnIterationStart(int)               {}
func (s *idStreamer) OnToolStart(key, name string, _ json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starts = append(s.starts, key)
	s.order = append(s.order, "start:"+name)
}
func (s *idStreamer) OnToolEnd(key, name, _ string, _ error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ends = append(s.ends, key)
	s.order = append(s.order, "end:"+name)
}

// Three Task calls in one message run at the same time: the turn takes about
// one subagent's time, not three, and the results keep the model's order.
func TestRun_ParallelTasksFasterThanSum(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cli := &scriptedLLM{replies: []*llm.StreamResult{
		{ToolCalls: []llm.ToolCall{taskCall("c1", "one"), taskCall("c2", "two"), taskCall("c3", "three")}},
		{Content: "all done"},
	}}
	run := &sleepyRunner{d: 300 * time.Millisecond}
	st := &idStreamer{}
	reg := tools.NewRegistry()
	reg.Register(&tools.TaskTool{})
	a := New(cli, reg, "sys", nil, st)

	ctx := tools.WithSubagentRunner(context.Background(), run)
	start := time.Now()
	hist, err := a.Run(ctx, nil, "go")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed >= 3*run.d {
		t.Errorf("3 subagents took %v — not faster than running them one by one (%v)", elapsed, 3*run.d)
	}
	if run.peak != 3 {
		t.Errorf("peak concurrency %d, want 3", run.peak)
	}
	// Tool results in the original order, matched by call id.
	var toolMsgs []llm.AIMessage
	for _, m := range hist {
		if m.Role == "tool" {
			toolMsgs = append(toolMsgs, m)
		}
	}
	want := []struct{ id, title string }{{"c1", "one"}, {"c2", "two"}, {"c3", "three"}}
	if len(toolMsgs) != 3 {
		t.Fatalf("tool messages: %d", len(toolMsgs))
	}
	for i, w := range want {
		if toolMsgs[i].ToolCallID != w.id || !strings.Contains(llm.ContentText(toolMsgs[i].Content), w.title) {
			t.Errorf("result %d = %s %q, want %s with %q", i, toolMsgs[i].ToolCallID, llm.ContentText(toolMsgs[i].Content), w.id, w.title)
		}
	}
	// The streamer got a start and an end for each call key; the runner saw
	// the same keys as the parent's tool call.
	if len(st.starts) != 3 || len(st.ends) != 3 {
		t.Errorf("starts %v ends %v", st.starts, st.ends)
	}
	got := strings.Join(run.calls, ",")
	for _, id := range []string{"c1", "c2", "c3"} {
		if !strings.Contains(got, id) {
			t.Errorf("runner did not get tool call key %s: %v", id, run.calls)
		}
	}
}

// orderTool records when it ran relative to the subagents.
type orderTool struct {
	run *sleepyRunner
	log *[]string
	mu  *sync.Mutex
}

func (o orderTool) Spec() tools.Spec                      { return tools.Spec{Name: "Probe"} }
func (o orderTool) RequiresApproval(json.RawMessage) bool { return false }
func (o orderTool) Execute(context.Context, json.RawMessage) (string, error) {
	o.run.mu.Lock()
	running := o.run.running
	o.run.mu.Unlock()
	o.mu.Lock()
	*o.log = append(*o.log, fmt.Sprintf("probe running=%d", running))
	o.mu.Unlock()
	return "ok", nil
}

// A sequential tool written after Task calls runs after them: the model may
// rely on the order of side effects within one message.
func TestRun_SequentialCallWaitsForEarlierTasks(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	probe := llm.ToolCall{ID: "p", Type: "function", Function: llm.ToolCallFunc{Name: "Probe", Arguments: "{}"}}
	cli := &scriptedLLM{replies: []*llm.StreamResult{
		{ToolCalls: []llm.ToolCall{taskCall("c1", "one"), taskCall("c2", "two"), probe}},
	}}
	run := &sleepyRunner{d: 100 * time.Millisecond}
	var log []string
	var mu sync.Mutex
	reg := tools.NewRegistry()
	reg.Register(&tools.TaskTool{})
	reg.Register(orderTool{run: run, log: &log, mu: &mu})
	a := New(cli, reg, "sys", nil, nil)
	if _, err := a.Run(tools.WithSubagentRunner(context.Background(), run), nil, "go"); err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0] != "probe running=0" {
		t.Errorf("probe ran while subagents were still running: %v", log)
	}
}

// Checkpoint sees the history after every iteration; Exhausted marks the
// soft stop at the iteration limit.
func TestRun_CheckpointAndExhausted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	probe := llm.ToolCall{ID: "p", Type: "function", Function: llm.ToolCallFunc{Name: "Probe", Arguments: "{}"}}
	cli := &scriptedLLM{replies: []*llm.StreamResult{
		{ToolCalls: []llm.ToolCall{probe}, PromptTokens: 11},
		{ToolCalls: []llm.ToolCall{probe}, PromptTokens: 22},
	}}
	var log []string
	var mu sync.Mutex
	reg := tools.NewRegistry()
	reg.Register(orderTool{run: &sleepyRunner{}, log: &log, mu: &mu})
	a := New(cli, reg, "sys", nil, nil)
	a.MaxIterations = 2
	var sizes []int
	var tokens []int
	a.Checkpoint = func(h []llm.AIMessage, pt int) {
		sizes = append(sizes, len(h))
		tokens = append(tokens, pt)
	}
	if _, err := a.Run(context.Background(), nil, "go"); err != nil {
		t.Fatal(err)
	}
	if !a.Exhausted {
		t.Error("Exhausted not set at the iteration limit")
	}
	if len(sizes) != 3 || !(sizes[0] < sizes[1] && sizes[1] < sizes[2]) {
		t.Errorf("checkpoints %v", sizes)
	}
	if tokens[1] != 22 {
		t.Errorf("prompt tokens %v", tokens)
	}
}

// Session-wide answers ("all Bash in this session") are written by whichever
// goroutine asked; the maps must not race when tool calls run in parallel.
func TestAgent_AllowedMapsAreSafeForParallelCalls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	a := New(nil, nil, "", approveAll{ApproveTool}, nil)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = a.approve(nil, fmt.Sprintf("T%d", i%4), json.RawMessage(`{}`))
		}(i)
	}
	wg.Wait()
}

type approveAll struct{ d ApproveDecision }

func (a approveAll) AskApprove(string, json.RawMessage, string) ApproveDecision { return a.d }
