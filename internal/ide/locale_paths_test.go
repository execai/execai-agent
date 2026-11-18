package ide

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// The two paths the live probe of 7.17 caught in Russian on an English
// panel: a start without the model catalog, and a tool the person declined.

// writeThenText asks to write a file, then answers with text.
type writeThenText struct {
	mu   sync.Mutex
	n    int
	path string
}

func (w *writeThenText) Stream(ctx context.Context, _ []llm.AIMessage, _ []map[string]any,
	cb llm.StreamCallbacks) (*llm.StreamResult, error) {
	w.mu.Lock()
	w.n++
	n := w.n
	w.mu.Unlock()
	if n == 1 {
		args, _ := json.Marshal(map[string]string{"path": w.path, "content": "x"})
		return &llm.StreamResult{ToolCalls: []llm.ToolCall{{ID: "w1", Type: "function",
			Function: llm.ToolCallFunc{Name: "Write", Arguments: string(args)}}}}, nil
	}
	if cb.OnText != nil {
		cb.OnText("ok")
	}
	return &llm.StreamResult{Content: "ok"}, nil
}

// The person declines a tool in the panel: the tool's result the panel
// shows is in the panel's language.
func TestPanelEnglish_DeclinedTool(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	dir := isolateIDE(t)
	buf := &syncBuf{}
	s := newServer(json.NewEncoder(buf), dir)
	c := s.newChat("m", "p")
	go answerFirstAsk(s, "deny")
	deps := turnDeps{cli: &writeThenText{path: filepath.Join(dir, "x.txt")}, maxIter: 3, registry: tools.Default(dir)}
	if !c.startTurn(In{Type: "user", Text: "write it"}, deps) {
		t.Fatal("turn not started")
	}
	res := buf.waitEvent(t, func(o Out) bool { return o.Type == "tool_result" })
	buf.waitEvent(t, func(o Out) bool { return o.Type == "done" })
	if res.Tail == "" || cyrillic.MatchString(res.Tail) {
		t.Errorf("declined tool result %q", res.Tail)
	}
	for _, o := range buf.events() {
		raw, _ := json.Marshal(o)
		if cyrillic.Match(raw) {
			t.Errorf("event %s", raw)
		}
	}
}

// The panel starts while the model catalog cannot be fetched (the server
// fails, no cache yet): the notice about working without it is English.
func TestPanelEnglish_StartWithoutModelCatalog(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	dir := isolateIDE(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer api.Close()
	cfg := config.Defaults()
	cfg.APIBase = api.URL
	cfg.Locale = "en"

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	buf := &syncBuf{}
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
		for sc.Scan() {
			_, _ = buf.Write(append(sc.Bytes(), '\n'))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, Options{Cwd: dir, In: inR, Out: outW}) }()
	buf.waitEvent(t, func(o Out) bool { return o.Type == "ready" })
	var notice string
	for _, o := range buf.events() {
		if o.Type == "notice" && strings.Contains(o.Text, "⚠") {
			notice = o.Text
		}
	}
	_ = inW.Close()
	cancel()
	<-done
	if notice == "" || cyrillic.MatchString(notice) {
		t.Errorf("start without the catalog: notice %q", notice)
	}
}

// The model backend refuses a turn (401, 402, 429) or ends it with a plan
// notice (billing_status): what the panel shows is in its language.
func TestPanelEnglish_ModelErrors(t *testing.T) {
	defer i18n.SetLocale("en")
	i18n.SetLocale("en")
	dir := isolateIDE(t)
	var mu sync.Mutex
	mode := ""
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		m := mode
		mu.Unlock()
		switch m {
		case "401", "402", "429":
			code := map[string]int{"401": 401, "402": 402, "429": 429}[m]
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"detail":"refused"}`))
		default:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"billing_status\",\"status\":\"" + m + "\",\"message\":\"\"}\n\ndata: [DONE]\n\n"))
		}
	}))
	defer api.Close()
	for _, m := range []string{"401", "402", "429", "rate_limit_exceeded", "insufficient_funds", "plan_inactive"} {
		t.Run(m, func(t *testing.T) {
			mu.Lock()
			mode = m
			mu.Unlock()
			buf := &syncBuf{}
			s := newServer(json.NewEncoder(buf), dir)
			c := s.newChat("m", "p")
			deps := turnDeps{cli: llm.NewAICoreClient(api.URL, "tok", "m", "p"), maxIter: 2, registry: tools.ReadOnly(dir)}
			if !c.startTurn(In{Type: "user", Text: "hello"}, deps) {
				t.Fatal("turn not started")
			}
			buf.waitEvent(t, func(o Out) bool { return o.Type == "done" })
			shown := false
			for _, o := range buf.events() {
				raw, _ := json.Marshal(o)
				if cyrillic.Match(raw) {
					t.Errorf("event %s", raw)
				}
				shown = shown || o.Type == "error" || (o.Type == "text_delta" && strings.TrimSpace(o.Text) != "")
			}
			if !shown {
				t.Errorf("nothing shown: %v", buf.events())
			}
		})
	}
}
