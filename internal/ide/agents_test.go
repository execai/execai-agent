package ide

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/fleet"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subagent"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// parentAndChild plays both sides of a turn: the chat's agent asks for two
// subagents in one message, then answers; a subagent (recognized by its
// system prompt) answers with its own text after a short pause.
type parentAndChild struct {
	mu        sync.Mutex
	parentHit int
}

func (p *parentAndChild) Stream(ctx context.Context, msgs []llm.AIMessage, _ []map[string]any, cb llm.StreamCallbacks) (*llm.StreamResult, error) {
	if strings.Contains(llm.ContentText(msgs[0].Content), "subagent of the execai") {
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if cb.OnText != nil {
			cb.OnText("CHILD TEXT")
		}
		return &llm.StreamResult{Content: "CHILD TEXT"}, nil
	}
	p.mu.Lock()
	p.parentHit++
	n := p.parentHit
	p.mu.Unlock()
	if n == 1 {
		call := func(id, title string) llm.ToolCall {
			args, _ := json.Marshal(map[string]any{"description": title, "prompt": "look"})
			return llm.ToolCall{ID: id, Type: "function", Function: llm.ToolCallFunc{Name: "Task", Arguments: string(args)}}
		}
		return &llm.StreamResult{ToolCalls: []llm.ToolCall{call("call_a", "scan api"), call("call_b", "scan ui")}}, nil
	}
	if cb.OnText != nil {
		cb.OnText("parent done")
	}
	return &llm.StreamResult{Content: "parent done"}, nil
}

// rawEvents returns the raw JSON lines of the given type.
func rawEvents(b *syncBuf, typ string) []map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.b.String()), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil && m["type"] == typ {
			out = append(out, m)
		}
	}
	return out
}

func newAgentsServer(t *testing.T) (*server, *syncBuf) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)
	buf := &syncBuf{}
	s := newServer(json.NewEncoder(buf), t.TempDir())
	f := fleet.New()
	f.MinGap = 10 * time.Millisecond
	unsub := s.attachAgents(subagent.NewManager(f))
	t.Cleanup(func() {
		unsub()
		s.agents.Shutdown(time.Second)
		f.Close()
	})
	return s, buf
}

// A turn with two Task calls: the subagents run, the panel gets the agents
// snapshot with the contract fields, the parent chat shows two Task cards
// (with call_id) and NONE of the subagents' own events.
func TestIDE_SubagentsInATurn(t *testing.T) {
	s, buf := newAgentsServer(t)
	c := s.newChat("m", "p")
	reg := tools.NewRegistry()
	reg.Register(&tools.TaskTool{})
	deps := turnDeps{cli: &parentAndChild{}, maxIter: 5, registry: reg, modelID: "glm", source: "Z.ai"}
	if !c.startTurn(In{Type: "user", Text: "go"}, deps) {
		t.Fatal("turn did not start")
	}
	buf.waitEvent(t, func(o Out) bool { return o.Type == "done" && o.Chat == c.id })

	// Parent feed: two Task cards with call ids, no child text.
	var cards []Out
	for _, o := range buf.events() {
		if o.Chat == c.id && o.Type == "tool_call" {
			cards = append(cards, o)
		}
		if o.Chat == c.id && o.Type == "text_delta" && strings.Contains(o.Text, "CHILD") {
			t.Error("a subagent's text leaked into the parent's feed")
		}
	}
	if len(cards) != 2 || cards[0].CallID != "call_a" || cards[1].CallID != "call_b" ||
		!strings.Contains(cards[0].Summary, "scan api") {
		t.Fatalf("cards %+v", cards)
	}

	// The agents event: items with the contract fields, both done.
	var last map[string]any
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		evs := rawEvents(buf, "agents")
		if len(evs) > 0 {
			last = evs[len(evs)-1]
			items, _ := last["items"].([]any)
			doneN := 0
			for _, it := range items {
				if it.(map[string]any)["state"] == "done" {
					doneN++
				}
			}
			if doneN == 2 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	items, _ := last["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("agents event %v", last)
	}
	rec := items[0].(map[string]any)
	for _, k := range []string{"id", "parent_chat", "parent_tool_call", "title", "state", "mode", "iter",
		"max_iter", "last_tool", "started_at", "updated_at", "finished_at", "preview", "result", "session",
		"source", "model", "prompt_tokens"} {
		if _, ok := rec[k]; !ok {
			t.Errorf("record lacks %q: %v", k, rec)
		}
	}
	if rec["parent_chat"] != c.id || rec["state"] != "done" || rec["source"] != "Z.ai" {
		t.Errorf("record %v", rec)
	}
	pt := map[any]bool{}
	for _, it := range items {
		pt[it.(map[string]any)["parent_tool_call"]] = true
	}
	if !pt["call_a"] || !pt["call_b"] {
		t.Errorf("parent_tool_call does not match the cards: %v", pt)
	}

	// open_agent: the transcript for a read-only tab.
	id, _ := rec["id"].(string)
	s.agentCommand(In{Type: "command", Name: "open_agent", Value: id})
	loaded := rawEvents(buf, "agent_loaded")
	if len(loaded) != 1 {
		t.Fatalf("agent_loaded: %v", loaded)
	}
	l := loaded[0]
	if l["id"] != id || l["title"] == "" || l["state"] != "done" {
		t.Errorf("agent_loaded %v", l)
	}
	msgs, _ := l["messages"].([]any)
	if len(msgs) < 2 || msgs[0].(map[string]any)["role"] != "user" ||
		msgs[len(msgs)-1].(map[string]any)["text"] != "CHILD TEXT" {
		t.Errorf("messages %v", msgs)
	}

	// list_agents: the same snapshot on request.
	before := len(rawEvents(buf, "agents"))
	s.agentCommand(In{Type: "command", Name: "list_agents"})
	if len(rawEvents(buf, "agents")) != before+1 {
		t.Error("list_agents sent nothing")
	}
}

// The workspaces event keeps its own items next to the agents' ones.
func TestOut_ItemsForBothEvents(t *testing.T) {
	ws := []WorkspaceItem{{ID: "w1"}}
	b, _ := json.Marshal(Out{Type: "workspaces", Items: &ws})
	if !strings.Contains(string(b), `"items":[{"id":"w1"`) {
		t.Errorf("workspaces: %s", b)
	}
	b, _ = json.Marshal(agentsOut(nil))
	if string(b) != `{"type":"agents","items":[]}` {
		t.Errorf("empty agents: %s", b)
	}
	b, _ = json.Marshal(agentsOut([]fleet.Record{{ID: "sa-1", State: fleet.Running}}))
	if !strings.Contains(string(b), `"items":[{"id":"sa-1"`) || !strings.Contains(string(b), `"state":"running"`) {
		t.Errorf("agents: %s", b)
	}
}

// stop_agent stops one subagent; an unknown id is an error.
func TestIDE_StopAgent(t *testing.T) {
	s, buf := newAgentsServer(t)
	r := s.agents.Runner(subagent.Deps{Client: &textLLM{block: true}, Cwd: s.cwd, ParentChat: "c"})
	rq := tools.SubagentRequest{Description: "stuck", Prompt: "x", Mode: "read", MaxIterations: 5, Background: true}
	if _, err := r.Run(context.Background(), rq); err != nil {
		t.Fatal(err)
	}
	id := s.agents.Fleet().Snapshot()[0].ID
	time.Sleep(20 * time.Millisecond)
	s.agentCommand(In{Type: "command", Name: "stop_agent", Value: id})
	buf.waitEvent(t, func(o Out) bool { return o.Type == "notice" && strings.Contains(o.Text, "stuck") })
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if rec, _ := s.agents.Fleet().Get(id); rec.State == fleet.Cancelled {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if rec, _ := s.agents.Fleet().Get(id); rec.State != fleet.Cancelled {
		t.Errorf("state %q", rec.State)
	}
	s.agentCommand(In{Type: "command", Name: "stop_agent", Value: "sa-nope"})
	buf.waitEvent(t, func(o Out) bool { return o.Type == "error" && strings.Contains(o.Text, "sa-nope") })
}

// A write subagent's question carries agent {id, title}; when the subagent
// is gone before the answer, the question is withdrawn and the answer is no.
func TestIdeApprover_SubagentAsk(t *testing.T) {
	buf := &syncBuf{}
	s := newServer(json.NewEncoder(buf), "")
	c := s.newChat("m", "p")
	who := agent.Asker{ID: "sa-1", Title: "fix tests"}

	go answerFirstAsk(s, "once")
	if d := c.ap.AskApproveAs(context.Background(), who, "Bash", json.RawMessage(`{"command":"go test"}`), "go test"); d != agent.ApproveOnce {
		t.Fatalf("decision %v", d)
	}
	var ask *Out
	for _, o := range buf.events() {
		if o.Type == "ask" {
			o := o
			ask = &o
		}
	}
	if ask == nil || ask.Agent == nil || ask.Agent.ID != "sa-1" || ask.Agent.Title != "fix tests" {
		t.Fatalf("ask %+v", ask)
	}

	ctx, cancel := context.WithCancel(context.Background())
	res := make(chan agent.ApproveDecision, 1)
	go func() {
		res <- c.ap.AskApproveAs(ctx, who, "Bash", json.RawMessage(`{"command":"rm -rf build"}`), "rm")
	}()
	buf.waitEvent(t, func(o Out) bool { return o.Type == "ask" && strings.Contains(o.Summary, "rm") })
	cancel()
	select {
	case d := <-res:
		if d != agent.ApproveDeny {
			t.Errorf("a withdrawn question gave %v", d)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the question did not end with its subagent")
	}
	buf.waitEvent(t, func(o Out) bool { return o.Type == "ask_resolved" && o.By == resolvedByCancel })

	// The chat's own agent and a subagent never show two questions at once.
	go answerFirstAsk(s, "deny")
	done := make(chan struct{})
	go func() {
		c.ap.AskApproveAs(context.Background(), who, "Write", json.RawMessage(`{"path":"a"}`), "a")
		close(done)
	}()
	<-done
}
