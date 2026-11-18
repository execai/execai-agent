package ide

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Another computer of the account keeps project ws-x at the very path this
// window works in. This machine has not bound ws-x: the window is not
// "bound" to it, it does not count as a bound folder of this machine, and a
// task of ws-x is answered with the refusal instead of running here.
func TestInbox_OtherMachinesBindingIsNotThisFolders(t *testing.T) {
	isolateIDE(t)
	i18n.SetLocale("en")
	here := t.TempDir()
	var mu sync.Mutex
	polls := 0
	results := map[string]map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch {
		case r.URL.Path == "/agents-vbai/inbox/poll":
			mu.Lock()
			polls++
			n := polls
			mu.Unlock()
			if n == 1 {
				_, _ = w.Write([]byte(`{"tasks":[{"id":"t-x","workspace_id":"ws-x","payload":"{\"prompt\":\"fix it\"}"}]}`))
				return
			}
			select {
			case <-r.Context().Done():
			case <-time.After(50 * time.Millisecond):
			}
			_, _ = w.Write([]byte(`{"tasks":[]}`))
		case r.URL.Path == "/agents-vbai/workspaces/bindings":
			_ = json.NewEncoder(w).Encode(map[string]any{"bindings": []map[string]string{
				{"agent_id": "agent-2", "workspace_id": "ws-x", "local_path": here},
			}})
		case r.URL.Path == "/agents-vbai/inbox/ack":
			var body struct {
				IDs []string `json:"task_ids"`
			}
			_ = json.Unmarshal(raw, &body)
			_ = json.NewEncoder(w).Encode(map[string]any{"acked": body.IDs})
		case strings.HasSuffix(r.URL.Path, "/result"):
			var body map[string]string
			_ = json.Unmarshal(raw, &body)
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/agents-vbai/tasks/"), "/result")
			mu.Lock()
			results[id] = body
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/integrations-vbai/workspaces":
			_, _ = w.Write([]byte(`[{"id":"ws-x","name":"site"}]`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)

	ctl, _ := newTestCtl(t, srv.URL, here, gateOf(true))
	ctl.refresh(context.Background(), refreshOpts{binding: true, force: true})
	if b := ctl.snapshot().BoundWorkspace; b != nil {
		t.Errorf("the window shows itself bound to %+v — only another machine bound that project", b)
	}
	if n := ctl.bindings(); n != 0 {
		t.Errorf("bound folders of this machine = %d, want 0", n)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case in := <-ctl.s.userQ:
			if in.Type == "web_task" {
				t.Fatalf("another machine's project ran here: %+v", in.task)
			}
		default:
		}
		mu.Lock()
		r := results["t-x"]
		mu.Unlock()
		if r != nil {
			if r["chunk_type"] != "error" || r["data"] != i18n.T("inbox.task.notBound") {
				t.Errorf("result %v, want the not-bound refusal", r)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the task got no answer")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
