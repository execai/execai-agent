package ide

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/machine"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

// The machine_state event on the wire: the three keys of the contract, and
// nothing of it leaks into the other events.
func TestProtocol_MachineStateShape(t *testing.T) {
	snap := machine.Build(machine.Inputs{OS: "linux", Bindings: -1, Now: time.Now()})
	b, err := json.Marshal(machineStateOut(snap))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"type", "machine", "tutorial", "phone_url"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("machine_state lacks %q: %s", k, b)
		}
	}
	if string(raw["type"]) != `"machine_state"` || string(raw["phone_url"]) != `""` {
		t.Errorf("event %s", b)
	}
	other, _ := json.Marshal(Out{Type: "state", Model: "m"})
	for _, k := range []string{"machine", "tutorial", "phone_url"} {
		if strings.Contains(string(other), `"`+k+`"`) {
			t.Errorf("state event carries %q: %s", k, other)
		}
	}
}

// The whole path through Run: the panel asks, the agent answers; a logout
// and a change of the sync state are announced without asking.
func TestRun_MachineState(t *testing.T) {
	dir := isolateIDE(t)
	t.Chdir(dir)
	prev := machineWatchEvery
	machineWatchEvery = 50 * time.Millisecond
	t.Cleanup(func() { machineWatchEvery = prev })

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/billing-vbai/models_public":
			_, _ = w.Write([]byte(`[{"id":"m1","provider":"p","name":"M1","is_primary":true}]`))
		case "/billing-vbai/my_plan":
			_, _ = w.Write([]byte(`{"tariff_code":"week_work","title":"Work","state":"active","valid_until":"2099-01-01T00:00:00Z","is_accessible":true}`))
		case "/agents-vbai/inbox/poll":
			select {
			case <-r.Context().Done():
			case <-time.After(100 * time.Millisecond):
			}
			_, _ = w.Write([]byte(`{"tasks":[]}`))
		case "/agents-vbai/workspaces/bindings":
			_, _ = w.Write([]byte(`{"bindings":[{"agent_id":"agent-1","workspace_id":"w1","local_path":"/elsewhere"}]}`))
		case "/integrations-vbai/workspaces":
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer api.Close()
	if err := config.SaveCredentials(&config.Credentials{Token: "tok", AgentID: "agent-1",
		Email: "alice@example.com", WebBase: "https://web.example.org"}); err != nil {
		t.Fatal(err)
	}
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
	send := func(line string) {
		if _, err := io.WriteString(inW, line+"\n"); err != nil {
			t.Fatal(err)
		}
	}

	buf.waitEvent(t, func(o Out) bool { return o.Type == "ready" })
	// The inbox has settled: listening, the bindings are known.
	buf.waitEvent(t, func(o Out) bool { return o.Type == "inbox" && o.State == inboxListening })
	asked := time.Now()
	send(`{"type":"command","name":"machine_state"}`)
	ms := buf.waitEvent(t, func(o Out) bool { return o.Type == "machine_state" && o.Machine != nil && o.Machine.LoggedIn })
	// The answer comes from what is known, without waiting on the network.
	if d := time.Since(asked); d > 2*time.Second {
		t.Errorf("machine_state took %v", d)
	}
	m := ms.Machine
	if m.ID != "agent-1" || m.EmailMasked != "a***@e***.com" || m.Plan != "week_work" {
		t.Errorf("machine %+v", m)
	}
	if !m.Presence.ListenerRunning || !m.Presence.ReachableFromWeb {
		t.Errorf("presence %+v", m.Presence)
	}
	if ms.PhoneURL == nil || *ms.PhoneURL != "https://web.example.org/m/agent-1" {
		t.Errorf("phone_url %v", ms.PhoneURL)
	}
	if ms.Tutorial == nil || len(ms.Tutorial.Steps) != 5 {
		t.Fatalf("tutorial %+v", ms.Tutorial)
	}
	want := map[string]bool{"login": true, "models": true, "sync": false, "project": true, "phone": false}
	for _, s := range ms.Tutorial.Steps {
		if s.Done != want[s.ID] {
			t.Errorf("step %s done=%v, want %v", s.ID, s.Done, want[s.ID])
		}
	}

	// Sync switched on elsewhere (execai sync on in a terminal): announced.
	st := &agentsync.State{Enabled: true, LastPull: time.Now(), LastPush: time.Now()}
	if err := st.Save(agentsync.Paths{ConfigDir: filepath.Join(dir, "execai")}); err != nil {
		t.Fatal(err)
	}
	buf.waitEvent(t, func(o Out) bool {
		return o.Type == "machine_state" && o.Machine != nil && o.Machine.Sync.Enabled
	})

	// A logout is announced with the new state.
	send(`{"type":"command","name":"logout"}`)
	out := buf.waitEvent(t, func(o Out) bool {
		return o.Type == "machine_state" && o.Machine != nil && !o.Machine.LoggedIn
	})
	if out.PhoneURL == nil || *out.PhoneURL != "" {
		t.Errorf("phone_url after logout: %v", out.PhoneURL)
	}

	_ = inW.Close()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return")
	}
	_ = outW.Close()
	_ = os.Remove(filepath.Join(dir, "execai", "serve.pid"))
}
