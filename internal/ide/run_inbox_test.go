package ide

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
)

// The whole path through Run: the window takes the free inbox, a task from
// the site becomes a "web" chat (a tab appears without asking), the turn
// runs, its outcome goes back to the site, and closing stdin gives the inbox
// back.
func TestRun_WebTaskEndToEnd(t *testing.T) {
	dir := isolateIDE(t)
	t.Chdir(dir)
	var mu sync.Mutex
	polls := 0
	results := map[string]map[string]string{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch {
		case r.URL.Path == "/billing-vbai/models_public":
			_, _ = w.Write([]byte(`[{"id":"m1","provider":"p","name":"M1","is_primary":true}]`))
		case r.URL.Path == "/billing-vbai/my_plan":
			_, _ = w.Write([]byte(`{"tariff_code":"week_work","state":"active","valid_until":"2099-01-01T00:00:00Z","is_accessible":true}`))
		case r.URL.Path == "/agents-vbai/inbox/poll":
			mu.Lock()
			polls++
			n := polls
			mu.Unlock()
			if n == 1 {
				_, _ = w.Write([]byte(`{"tasks":[{"id":"T","payload":"{\"prompt\":\"hello from the site\"}"}]}`))
				return
			}
			select {
			case <-r.Context().Done():
			case <-time.After(100 * time.Millisecond):
			}
			_, _ = w.Write([]byte(`{"tasks":[]}`))
		case r.URL.Path == "/agents-vbai/inbox/ack":
			var body struct {
				IDs []string `json:"task_ids"`
			}
			_ = json.Unmarshal(raw, &body)
			_ = json.NewEncoder(w).Encode(map[string]any{"acked": body.IDs})
		case r.URL.Path == "/agents-vbai/workspaces/bindings":
			_, _ = w.Write([]byte(`{"bindings":[]}`))
		case r.URL.Path == "/integrations-vbai/workspaces":
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/result"):
			var body map[string]string
			_ = json.Unmarshal(raw, &body)
			mu.Lock()
			results[strings.Split(r.URL.Path, "/")[3]] = body
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		default:
			// The model backend: fail fast, the outcome still has to reach the site.
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`model backend is down`))
		}
	}))
	defer api.Close()
	if err := config.SaveCredentials(&config.Credentials{Token: "tok", AgentID: "agent-1", Email: "a@b"}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.APIBase = api.URL
	cfg.Locale = "en"

	inR, inW, _ := os.Pipe()
	outR, outW, _ := os.Pipe()
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = inR, outW
	defer func() { os.Stdin, os.Stdout = oldIn, oldOut }()

	buf := &syncBuf{}
	copied := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
		for sc.Scan() {
			_, _ = buf.Write(append(sc.Bytes(), '\n'))
		}
		close(copied)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, Options{Cwd: dir}) }()

	buf.waitEvent(t, func(o Out) bool { return o.Type == "inbox" && o.State == inboxListening })
	chats := buf.waitEvent(t, func(o Out) bool { return o.Type == "chats" && len(o.Chats) > 0 })
	if chats.Chats[0].Origin != originWeb || !chats.Chats[0].Busy {
		t.Errorf("the web chat's tab: %+v", chats.Chats[0])
	}
	auto := buf.waitEvent(t, func(o Out) bool { return o.Type == "auto_user" })
	if auto.Origin != originWeb || auto.Text != "hello from the site" || auto.Chat != chats.Chats[0].ID {
		t.Errorf("auto_user %+v", auto)
	}
	buf.waitEvent(t, func(o Out) bool { return o.Type == "done" && o.Chat == auto.Chat })

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		r := results["T"]
		mu.Unlock()
		if r != nil {
			if r["chunk_type"] != inbox.ChunkError || r["data"] == "" {
				t.Errorf("result %v", r)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the task's outcome never reached the site")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if o := inbox.CurrentOwner(); o == nil || o.PID != os.Getpid() || o.Kind != inbox.KindIDE {
		t.Errorf("lock %+v", o)
	}

	if os.Getenv("IDE_E2E_DUMP") != "" {
		for _, o := range buf.events() {
			b, _ := json.Marshal(o)
			t.Log(string(b))
		}
		mu.Lock()
		t.Log(results["T"])
		mu.Unlock()
	}
	_ = inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return on closed stdin")
	}
	_ = outW.Close()
	<-copied
	if o := inbox.CurrentOwner(); o != nil && o.PID == os.Getpid() {
		t.Error("the inbox was not given back when the editor closed")
	}
}
