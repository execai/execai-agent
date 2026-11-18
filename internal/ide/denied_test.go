package ide

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// toolResults runs calls through a streamer and returns its tool_result
// events as the panel gets them: decoded from the wire, raw fields included.
func toolResults(t *testing.T, feed func(st *streamer)) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	srv := newServer(json.NewEncoder(&buf), "")
	st := &streamer{c: srv.newChat("m", "p"), changed: map[string]bool{}, turnStart: time.Now()}
	feed(st)
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("not JSON on the wire: %q", line)
		}
		if ev["type"] == "tool_result" {
			out = append(out, ev)
		}
	}
	return out
}

// A call the person refused used to reach the panel as a call that ran:
// the tool loop returns the refusal as an ordinary result, without an error
// (the model has to read it), and tool_result said ok — a green tick on a
// command that was never executed. The event now says denied; ok stays true,
// so a panel released before the field shows what it showed.
func TestToolResult_RefusalIsMarkedDenied(t *testing.T) {
	evs := toolResults(t, func(st *streamer) {
		st.OnToolStart("k1", "Bash", json.RawMessage(`{"command":"rm -rf build"}`))
		st.OnToolEnd("k1", "Bash", i18n.T("agent.tool.declined"), nil)
		st.OnToolStart("k2", "Bash", json.RawMessage(`{"command":"ls"}`))
		st.OnToolEnd("k2", "Bash", "a.txt", nil)
		st.OnToolStart("k3", "Bash", json.RawMessage(`{"command":"false"}`))
		st.OnToolEnd("k3", "Bash", "", errors.New("exit status 1"))
		// The words of a refusal inside a longer output are an output, not a refusal.
		st.OnToolStart("k4", "Read", json.RawMessage(`{"path":"log.txt"}`))
		st.OnToolEnd("k4", "Read", "log: "+i18n.T("agent.tool.declined"), nil)
	})
	if len(evs) != 4 {
		t.Fatalf("tool_result events: %d, want 4: %v", len(evs), evs)
	}
	refused, ran, failed, quoted := evs[0], evs[1], evs[2], evs[3]
	if refused["denied"] != true {
		t.Errorf("a refused call is not marked denied: %v", refused)
	}
	if refused["ok"] != true {
		t.Errorf("ok of a refused call changed (%v): panels without the field would draw a failure", refused["ok"])
	}
	if refused["tail"] != i18n.T("agent.tool.declined") {
		t.Errorf("the refusal text is gone from the event: %v", refused["tail"])
	}
	for name, ev := range map[string]map[string]any{"a call that ran": ran, "a call that failed": failed, "an output quoting the refusal": quoted} {
		if _, has := ev["denied"]; has {
			t.Errorf("%s carries denied: %v", name, ev)
		}
	}
	if ran["ok"] != true || failed["ok"] != false {
		t.Errorf("ok of the other calls: ran %v, failed %v", ran["ok"], failed["ok"])
	}
}
