package ide

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/fleet"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/subagent"
)

// Subagents in the editor panel (and `execai ui`, the same protocol):
//
//   - agents {items:[record…]} — the full registry (internal/fleet) after
//     every change, at most twice a second, and once after ready;
//   - list_agents — the same snapshot on request;
//   - open_agent {value:id} → agent_loaded {id, title, state, messages} —
//     the subagent's transcript from its session, for a read-only tab;
//   - stop_agent {value:id} — stop one subagent;
//   - extend_agent {value:"id [steps]"} — give one more steps: a working one
//     gets a higher max_iter at once, one with exhausted=true goes on;
//   - set_subagent_iterations {value:N} — steps of new subagents (state
//     carries it as subagent_iter);
//   - ask carries agent {id, title} when a write subagent asks.
//
// Subagents' own events never reach the parent chat's feed: the chat shows
// one Task card (tool_call with call_id = the record's parent_tool_call).

// AgentRef names a subagent in an ask event.
type AgentRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// MarshalJSON writes Out as usual, except that the agents event puts the
// registry snapshot under "items" — the name the contract gives it, already
// used by the workspaces event with another element type.
func (o Out) MarshalJSON() ([]byte, error) {
	type plain Out
	if o.Agents == nil {
		return json.Marshal(plain(o))
	}
	return json.Marshal(struct {
		plain
		Items []fleet.Record `json:"items"`
	}{plain(o), *o.Agents})
}

// agentsOut — the agents event for a snapshot (an empty list is sent as []).
func agentsOut(recs []fleet.Record) Out {
	items := append([]fleet.Record{}, recs...)
	return Out{Type: "agents", Agents: &items}
}

// attachAgents connects the process's subagents to the panel: every change
// of the registry goes out as the agents event. Returns the unsubscribe.
func (s *server) attachAgents(m *subagent.Manager) func() {
	s.agents = m
	return m.Fleet().Subscribe(func(recs []fleet.Record) { s.emit(agentsOut(recs)) })
}

// agentCommand handles list_agents, open_agent, stop_agent and extend_agent.
func (s *server) agentCommand(in In) {
	if s.agents == nil {
		s.emit(agentsOut(nil))
		return
	}
	switch in.Name {
	case "list_agents":
		s.emit(agentsOut(s.agents.Fleet().Snapshot()))
	case "open_agent":
		rec, sess, err := s.agents.Transcript(in.Value)
		if err != nil {
			text := i18n.Tf("fleet.ide.notFound", in.Value)
			if _, known := s.agents.Fleet().Get(in.Value); known {
				text += ": " + err.Error() // known, but its session is unreadable
			}
			s.emit(Out{Type: "error", Text: text})
			return
		}
		o := replayOut(sess.Messages)
		s.emit(Out{Type: "agent_loaded", ID: rec.ID, Title: rec.Title, State: string(rec.State),
			Messages: append([]ReplayMsg{}, o.Msgs...)})
	case "extend_agent":
		f := strings.Fields(in.Value)
		if len(f) == 0 {
			s.emit(Out{Type: "error", Text: i18n.Tf("fleet.ide.notFound", "")})
			return
		}
		steps := 0
		if len(f) > 1 {
			steps, _ = strconv.Atoi(f[1])
		}
		msg, err := s.agents.Extend(f[0], steps)
		if err != nil {
			s.emit(Out{Type: "error", Text: err.Error()})
			return
		}
		s.emit(Out{Type: "notice", Text: msg})
	case "stop_agent":
		rec, ok := s.agents.Fleet().Get(in.Value)
		if !ok {
			s.emit(Out{Type: "error", Text: i18n.Tf("fleet.ide.notFound", in.Value)})
			return
		}
		if s.agents.Stop(rec.ID) {
			s.emit(Out{Type: "notice", Text: i18n.Tf("fleet.ide.stopping", rec.Title)})
		} else {
			s.emit(Out{Type: "notice", Text: i18n.Tf("fleet.tui.notRunning", rec.Title)})
		}
	}
}

// subagentNotify — background subagents of a chat finished with nobody
// waiting: the main loop hands their results to the chat as an automatic
// turn once the chat is free.
func (s *server) subagentNotify(parent string) {
	go func() { s.userQ <- In{Type: "subagent_done", Chat: parent} }()
}

// subagentDone runs on the main loop: an idle chat gets the results now; a
// busy one gets them when its turn ends (finishTurn asks again).
func (s *server) subagentDone(in In, deps turnDeps) {
	c := s.getChat(in.Chat)
	if c == nil || s.agents == nil || c.isBusy() {
		return
	}
	text := s.agents.TakeFinished(c.id)
	if text == "" {
		return
	}
	c.startTurn(In{Type: "user", Text: text, Chat: c.id, auto: true}, deps)
}

// askSem lets one permission question of a chat onto the panel at a time:
// the main agent and its write subagents may ask together, and the chat
// remembers one pending question for replay.
func (c *chatSession) askSem() chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.asks == nil {
		c.asks = make(chan struct{}, 1)
	}
	return c.asks
}

// acquireAsk takes the chat's question slot; false — ctx ended first.
func (c *chatSession) acquireAsk(ctx context.Context) bool {
	select {
	case c.askSem() <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *chatSession) releaseAsk() { <-c.askSem() }

// AskApproveAs — a write subagent asks in its parent's chat. The ask event
// carries agent {id, title}; a subagent stopped before the answer takes its
// question off the panel (ask_resolved by cancel) and gets "no".
func (a *ideApprover) AskApproveAs(ctx context.Context, who agent.Asker, toolName string, args json.RawMessage, summary string) agent.ApproveDecision {
	return a.ask(ctx, &who, toolName, args, summary)
}

// AskApproveAs for a task from the site: the panel question carries the
// agent, the web chat gets the title in the summary (it shows text only).
func (d *dualApprover) AskApproveAs(ctx context.Context, who agent.Asker, toolName string, args json.RawMessage, summary string) agent.ApproveDecision {
	return d.ask(ctx, &who, toolName, args, summary)
}

// withAgent marks a question with its asker.
func withAgent(o Out, who *agent.Asker) Out {
	if who != nil {
		o.Agent = &AgentRef{ID: who.ID, Title: who.Title}
	}
	return o
}

// cancelQuestion takes an unanswered question off the panel.
func (a *ideApprover) cancelQuestion(id string) {
	a.c.srv.drop(id)
	a.c.emit(Out{Type: "ask_resolved", AskID: id, By: resolvedByCancel})
	a.clear(id)
}

// shutdownGrace — how long subagents get to record "cancelled" on exit.
const shutdownGrace = 2 * time.Second
