package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
)

type recorded struct {
	method, path, query, auth string
	body                      map[string]any
}

// fakeAPI records requests and answers by path.
func fakeAPI(t *testing.T, answer func(r *http.Request) (int, string)) (*httptest.Server, func() []recorded) {
	t.Helper()
	var mu sync.Mutex
	var got []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		got = append(got, recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), body})
		mu.Unlock()
		code, resp := answer(r)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []recorded {
		mu.Lock()
		defer mu.Unlock()
		return append([]recorded(nil), got...)
	}
}

// Poll sends the tasks left for other owners, so a server that knows the
// field does not offer them again at once.
func TestPoll_SendsSkip(t *testing.T) {
	srv, reqs := fakeAPI(t, func(*http.Request) (int, string) {
		return 200, `{"tasks":[{"id":"t1","workspace_id":"w","payload":"{\"prompt\":\"hi\"}"}]}`
	})
	cl := NewClient(srv.URL+"/", "tok")
	tasks, err := cl.Poll(context.Background(), 45*time.Second, []string{"a", "b"})
	if err != nil || len(tasks) != 1 || tasks[0].WorkspaceID != "w" {
		t.Fatalf("Poll = %+v, %v", tasks, err)
	}
	r := reqs()[0]
	if r.method != "POST" || r.path != "/agents-vbai/inbox/poll" || r.query != "wait=45" || r.auth != "Bearer tok" {
		t.Errorf("request %+v", r)
	}
	if skip, _ := r.body["skip"].([]any); len(skip) != 2 {
		t.Errorf("skip not sent: %+v", r.body)
	}
	// No skip — the old body, {}.
	_, _ = cl.Poll(context.Background(), time.Second, nil)
	if b := reqs()[1].body; len(b) != 0 {
		t.Errorf("body without skip: %+v", b)
	}
}

// Ask returns the answer and the question id; Withdraw answers only the open
// questions of its own task.
func TestAskAndWithdraw(t *testing.T) {
	srv, reqs := fakeAPI(t, func(r *http.Request) (int, string) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/ask"):
			return 200, `{"question_id":"q9","answer":"exact"}`
		case r.URL.Path == "/agents-vbai/questions" && r.Method == "GET":
			return 200, `{"questions":[
				{"id":"q1","task_id":"T","status":"open"},
				{"id":"q2","task_id":"OTHER","status":"open"},
				{"id":"q3","task_id":"T","status":"answered"}]}`
		default:
			return 200, `{"accepted":true}`
		}
	})
	cl := NewClient(srv.URL, "tok")
	ans, qid, err := cl.Ask(context.Background(), "T", "Bash", "ls", `{"command":"ls"}`)
	if err != nil || ans != AnswerExact || qid != "q9" {
		t.Fatalf("Ask = %q %q %v", ans, qid, err)
	}
	if r := reqs()[0]; r.path != "/agents-vbai/tasks/T/ask" || r.body["tool"] != "Bash" || r.body["args"] != `{"command":"ls"}` {
		t.Errorf("ask request %+v", r)
	}
	n, err := cl.Withdraw(context.Background(), "T", AnswerOnce)
	if err != nil || n != 1 {
		t.Fatalf("Withdraw = %d, %v", n, err)
	}
	var answered []string
	for _, r := range reqs() {
		if strings.HasSuffix(r.path, "/answer") {
			answered = append(answered, r.path)
			if r.body["answer"] != AnswerOnce {
				t.Errorf("answer %v", r.body["answer"])
			}
		}
	}
	if len(answered) != 1 || answered[0] != "/agents-vbai/questions/q1/answer" {
		t.Errorf("answered %v — only the open question of task T", answered)
	}
}

// The result goes out even when the caller's context is already cancelled
// (Ctrl+C, the editor closed): the web chat must not hang until its timeout.
func TestPostResult_SurvivesCancel(t *testing.T) {
	srv, reqs := fakeAPI(t, func(*http.Request) (int, string) { return 200, `{}` })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewClient(srv.URL, "tok").PostResult(ctx, "T", ChunkFinal, "done"); err != nil {
		t.Fatal(err)
	}
	r := reqs()[0]
	if r.path != "/agents-vbai/tasks/T/result" || r.body["chunk_type"] != "final" || r.body["data"] != "done" {
		t.Errorf("result request %+v", r)
	}
}

func TestAck_OnlyConfirmed(t *testing.T) {
	srv, _ := fakeAPI(t, func(*http.Request) (int, string) { return 200, `{"acked":["t2"]}` })
	got, err := NewClient(srv.URL, "tok").Ack(context.Background(), []Task{{ID: "t1"}, {ID: "t2"}})
	if err != nil || len(got) != 1 || got[0].ID != "t2" {
		t.Errorf("Ack = %+v, %v", got, err)
	}
}

func TestIsUnauthorized(t *testing.T) {
	srv, _ := fakeAPI(t, func(*http.Request) (int, string) { return 401, `no` })
	_, err := NewClient(srv.URL, "tok").Bindings(context.Background())
	if !IsUnauthorized(err) {
		t.Errorf("401 not recognised: %v", err)
	}
	if IsUnauthorized(errors.New("HTTP 401")) {
		t.Error("a plain error taken for 401")
	}
}

// The same rule for serve and the editor: text when done; partial text with
// the reason on an error; the error alone without text.
func TestOutcome(t *testing.T) {
	i18n.SetLocale("en")
	if c, d := Outcome("answer", nil); c != ChunkFinal || d != "answer" {
		t.Errorf("done: %s %q", c, d)
	}
	if c, d := Outcome("  ", nil); c != ChunkFinal || d != i18n.T("inbox.result.noText") {
		t.Errorf("no text: %s %q", c, d)
	}
	if c, d := Outcome("half", errors.New("boom")); c != ChunkFinal || !strings.HasPrefix(d, "half") || !strings.Contains(d, "boom") {
		t.Errorf("partial: %s %q", c, d)
	}
	if c, d := Outcome("", errors.New("boom")); c != ChunkError || d != "boom" {
		t.Errorf("error: %s %q", c, d)
	}
}

func TestExtractPrompt(t *testing.T) {
	for in, want := range map[string]string{
		`{"prompt":"  build it "}`: "build it",
		`plain text`:               "plain text",
		`{"prompt":""}`:            "",
	} {
		if got := ExtractPrompt(in); got != want {
			t.Errorf("ExtractPrompt(%q) = %q, want %q", in, got, want)
		}
	}
}

// Adding the agent to a project is idempotent: integrations-vbai does not
// deduplicate.
func TestAddAgent_Idempotent(t *testing.T) {
	srv, reqs := fakeAPI(t, func(*http.Request) (int, string) { return 200, `{}` })
	cl := NewClient(srv.URL, "tok")
	w := Workspace{ID: "ws", Tools: []WorkspaceTool{{ID: 3, ToolID: AgentToolID, Profile: "me"}}}
	if err := cl.AddAgent(context.Background(), w, "me"); err != nil || len(reqs()) != 0 {
		t.Errorf("already there: %v, %d requests", err, len(reqs()))
	}
	if err := cl.AddAgent(context.Background(), Workspace{ID: "ws"}, "me"); err != nil {
		t.Fatal(err)
	}
	r := reqs()[0]
	if r.path != "/integrations-vbai/workspaces/ws/tools" || r.body["profile"] != "me" || r.body["enabled"] != true {
		t.Errorf("add request %+v", r)
	}
}
