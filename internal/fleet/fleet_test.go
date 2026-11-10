package fleet

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
)

// Subscribers get full snapshots, coalesced: a burst of a hundred changes
// is not a hundred redraws, and the last state always arrives.
func TestFleet_SnapshotsAreCoalesced(t *testing.T) {
	f := New()
	f.MinGap = 100 * time.Millisecond
	defer f.Close()
	var mu sync.Mutex
	var got [][]Record
	f.Subscribe(func(recs []Record) {
		mu.Lock()
		got = append(got, recs)
		mu.Unlock()
	})
	f.Add(Record{ID: "a", Title: "one", State: Running})
	for i := 0; i < 100; i++ {
		f.Update("a", func(r *Record) { r.Iter = i + 1 })
		time.Sleep(time.Millisecond)
	}
	f.Update("a", func(r *Record) { r.State = Done; r.Result = "ok" })
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	// ~100 ms of updates at a 100 ms gap: a handful of deliveries, not 100.
	if len(got) == 0 || len(got) > 6 {
		t.Fatalf("deliveries: %d", len(got))
	}
	last := got[len(got)-1]
	if len(last) != 1 || last[0].State != Done || last[0].Iter != 100 || last[0].Result != "ok" {
		t.Errorf("last snapshot %+v", last)
	}
}

// Two a second at most by default: the contract with the panel.
func TestFleet_DefaultGap(t *testing.T) {
	if New().MinGap != 500*time.Millisecond {
		t.Errorf("default gap %v", New().MinGap)
	}
}

func TestFleet_OrderTrimAndCounts(t *testing.T) {
	f := New()
	f.Keep = 3
	f.Add(Record{ID: "1", State: Done})
	f.Add(Record{ID: "2", State: Running})
	f.Add(Record{ID: "3", State: Failed})
	f.Add(Record{ID: "4", State: Queued})
	snap := f.Snapshot()
	ids := []string{}
	for _, r := range snap {
		ids = append(ids, r.ID)
	}
	// The oldest finished one goes first; running ones are never dropped.
	if strings.Join(ids, ",") != "2,3,4" {
		t.Errorf("after trim: %v", ids)
	}
	a, d, x := Counts(snap)
	if a != 2 || d != 0 || x != 1 {
		t.Errorf("counts %d %d %d", a, d, x)
	}
	if !f.HasActive() {
		t.Error("HasActive")
	}
	if f.Update("nope", func(*Record) {}) {
		t.Error("update of a missing record reported success")
	}
}

func TestFleet_PreviewIsCut(t *testing.T) {
	f := New()
	f.Add(Record{ID: "a", Preview: strings.Repeat("я", 500)})
	r, _ := f.Get("a")
	if n := len([]rune(r.Preview)); n != PreviewLimit {
		t.Errorf("preview %d runes", n)
	}
}

// fleetServer is a stand-in for agents-vbai PUT /presence/fleet.
type fleetServer struct {
	mu     sync.Mutex
	bodies []ReportPayload
	raw    []string
	auth   []string
}

func (s *fleetServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/agents-vbai/presence/fleet" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		data, _ := io.ReadAll(r.Body)
		var p ReportPayload
		if err := json.Unmarshal(data, &p); err != nil {
			t.Errorf("bad body: %v", err)
		}
		s.mu.Lock()
		s.bodies = append(s.bodies, p)
		s.raw = append(s.raw, string(data))
		s.auth = append(s.auth, r.Header.Get("Authorization"))
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *fleetServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The reporter sends when work starts, periodically while it goes on, once
// when it ends — results never leave the machine — and nothing at all
// when there is nothing to tell.
func TestReporter_SendsWhileActiveAndOnceAtTheEnd(t *testing.T) {
	srv := &fleetServer{}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()

	f := New()
	f.MinGap = 20 * time.Millisecond
	defer f.Close()
	rep := StartReporter(f, HTTPSender(ts.URL, func() string { return "jwt-1" }), "/proj", 80*time.Millisecond)
	defer rep.Close()

	time.Sleep(100 * time.Millisecond)
	if srv.count() != 0 {
		t.Fatal("a process without subagents reported")
	}

	f.Add(Record{ID: "sa-1", Title: "scan", State: Running, Mode: "read", MaxIter: 15})
	waitFor(t, "the first report", func() bool { return srv.count() >= 1 })
	waitFor(t, "a periodic report", func() bool { return srv.count() >= 3 })

	f.Update("sa-1", func(r *Record) { r.State = Done; r.Result = "SECRET RESULT"; r.Preview = "found it" })
	waitFor(t, "the final report", func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		last := srv.bodies[len(srv.bodies)-1]
		return len(last.Agents) == 1 && last.Agents[0].State == Done
	})
	n := srv.count()
	time.Sleep(250 * time.Millisecond)
	if srv.count() != n {
		t.Errorf("kept reporting after everything finished: %d → %d", n, srv.count())
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	for i, raw := range srv.raw {
		if strings.Contains(raw, "SECRET RESULT") || strings.Contains(raw, `"result"`) {
			t.Errorf("report %d carries the result: %s", i, raw)
		}
		if srv.auth[i] != "Bearer jwt-1" {
			t.Errorf("report %d auth %q", i, srv.auth[i])
		}
	}
	last := srv.bodies[len(srv.bodies)-1]
	if last.Cwd != "/proj" || last.PID == 0 || last.Agents[0].Preview != "found it" {
		t.Errorf("payload %+v", last)
	}
	for _, field := range []string{`"parent_chat"`, `"parent_tool_call"`, `"max_iter"`, `"prompt_tokens"`, `"started_at"`, `"session"`} {
		if !strings.Contains(srv.raw[len(srv.raw)-1], field) {
			t.Errorf("contract field %s missing", field)
		}
	}
}

// On exit the subagents are marked cancelled; Close delivers that state even
// though no periodic report is due.
func TestReporter_CloseSendsTheLastChange(t *testing.T) {
	srv := &fleetServer{}
	ts := httptest.NewServer(srv.handler(t))
	defer ts.Close()
	f := New()
	f.MinGap = time.Hour // no regular delivery in this test
	rep := StartReporter(f, HTTPSender(ts.URL, func() string { return "jwt" }), "/p", time.Hour)
	f.Add(Record{ID: "a", State: Running})
	f.Update("a", func(r *Record) { r.State = Cancelled })
	rep.Close()
	if srv.count() != 1 || srv.bodies[0].Agents[0].State != Cancelled {
		t.Fatalf("reports %+v", srv.bodies)
	}
}

// Not signed in: nothing is sent and nothing breaks.
func TestReporter_NoToken(t *testing.T) {
	err := HTTPSender("http://127.0.0.1:1", func() string { return "" })(context.Background(), []byte(`{}`))
	if err != ErrNoToken {
		t.Errorf("err %v", err)
	}
}

// The body stays under the server's limit: the oldest finished records go.
func TestBuildReport_FitsTheLimit(t *testing.T) {
	var recs []Record
	for i := 0; i < 600; i++ {
		recs = append(recs, Record{ID: strings.Repeat("x", 10) + string(rune('a'+i%26)), State: Done,
			Title: strings.Repeat("t", 100), Preview: strings.Repeat("p", 300)})
	}
	recs = append(recs, Record{ID: "live", State: Running})
	body, err := BuildReport(recs, 1, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > MaxReportBody {
		t.Fatalf("body %d bytes", len(body))
	}
	if !strings.Contains(string(body), `"live"`) {
		t.Error("the running subagent was dropped before finished ones")
	}
}
