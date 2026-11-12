package uiserve

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// alarmLog — what OnIntruder was told.
type alarmLog struct {
	mu      sync.Mutex
	reasons []Intrusion
}

func (a *alarmLog) add(r Intrusion) {
	a.mu.Lock()
	a.reasons = append(a.reasons, r)
	a.mu.Unlock()
}

func (a *alarmLog) list() []Intrusion {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Intrusion(nil), a.reasons...)
}

func (a *alarmLog) wait(t *testing.T, n int) []Intrusion {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := a.list(); len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("alarms %v, want %d", a.list(), n)
	return nil
}

func (h *harness) getKey(key string, cookie *http.Cookie) (*http.Response, *http.Cookie) {
	h.t.Helper()
	saved := h.cookie
	h.cookie = cookie
	defer func() { h.cookie = saved }()
	resp := h.req("GET", "/?t="+key, nil, nil)
	resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == h.s.cookieName() {
			return resp, c
		}
	}
	return resp, nil
}

func (h *harness) status(method, path string, cookie *http.Cookie) int {
	h.t.Helper()
	saved := h.cookie
	h.cookie = cookie
	defer func() { h.cookie = saved }()
	hdr := map[string]string{}
	if method == "POST" {
		hdr["Origin"], hdr["Content-Type"] = h.origin, "application/json"
	}
	resp := h.req(method, path, strings.NewReader(`{"type":"ping"}`), hdr)
	resp.Body.Close()
	return resp.StatusCode
}

// The one-time key is meant for one browser. When a second one comes with
// the same key moments after the first, somebody else has read the link —
// and one of the two is not the person. Every session is dropped, open
// event streams are cut, a new key is issued, and the panel is told.
func TestKey_UsedTwiceIsAnAlarm(t *testing.T) {
	var alarms alarmLog
	h := newHarness(t, Options{OnIntruder: alarms.add})
	key := h.token()

	// The one who read the link is first.
	resp, stolen := h.getKey(key, nil)
	if resp.StatusCode != http.StatusOK || stolen == nil {
		t.Fatalf("the first comer: %d", resp.StatusCode)
	}
	h.cookie = stolen
	_, closeStream := h.openEvents(nil)
	defer closeStream()
	h.cookie = nil
	if h.status("POST", "/in", stolen) != http.StatusNoContent {
		t.Fatal("the first comer is not in")
	}
	before := h.in.String()

	// The person's own browser comes with the same key.
	resp, got := h.getKey(key, nil)
	if resp.StatusCode != http.StatusForbidden || got != nil {
		t.Fatalf("the second comer: %d, cookie %v", resp.StatusCode, got)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Errorf("the second comer gets %q, want the sign-in page", resp.Header.Get("Content-Type"))
	}
	if got := alarms.wait(t, 1); len(got) != 1 || got[0] != KeyUsedTwice {
		t.Fatalf("alarms %v", got)
	}
	// The first comer's session is dead for everything.
	for _, c := range []struct{ method, path string }{{"GET", "/"}, {"GET", "/events"}, {"POST", "/in"}, {"POST", "/upload"}} {
		if got := h.status(c.method, c.path, stolen); got != http.StatusForbidden {
			t.Errorf("%s %s with the dropped session: %d", c.method, c.path, got)
		}
	}
	if h.in.String() != before {
		t.Errorf("a message reached the agent after the alarm: %q", h.in.String()[len(before):])
	}
	deadline := time.Now().Add(5 * time.Second)
	for h.s.Viewers() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := h.s.Viewers(); n != 0 {
		t.Errorf("%d event streams survived the alarm", n)
	}

	// A new key: the old one is just a wrong key now, the new one works once.
	next := h.token()
	if next == key || len(next) != 64 {
		t.Fatalf("the key after the alarm: %q", next)
	}
	if h.s.TokenUsed() {
		t.Error("the new key counts as used")
	}
	if resp, c := h.getKey(key, nil); resp.StatusCode != http.StatusForbidden || c != nil {
		t.Errorf("the old key: %d", resp.StatusCode)
	}
	resp, fresh := h.getKey(next, nil)
	if resp.StatusCode != http.StatusOK || fresh == nil || fresh.Value == stolen.Value {
		t.Fatalf("the new key: %d", resp.StatusCode)
	}
	if h.status("GET", "/", fresh) != http.StatusOK {
		t.Error("the new session does not open the page")
	}
	if got := alarms.list(); len(got) != 1 {
		t.Errorf("alarms %v: the old key or the new sign-in rang again", got)
	}
}

// A second comer with the used key is an alarm however late it comes — an
// hour after the panel opened, a link somebody kept is as much "in other
// hands" as in the first second. What is not an alarm is the person: a
// request that carries the session — the page reloaded, the printed link
// clicked once more in the browser that already holds the cookie.
func TestKey_ReuseIsAnAlarmHoweverLate(t *testing.T) {
	var alarms alarmLog
	h := newHarness(t, Options{OnIntruder: alarms.add})
	key := h.token()
	_, mine := h.getKey(key, nil)
	if mine == nil {
		t.Fatal("no session")
	}
	h.cookie = mine
	_, closeStream := h.openEvents(nil)
	defer closeStream()
	h.cookie = nil
	time.Sleep(150 * time.Millisecond)

	// The person: the link again, the page reloaded, the link again.
	for i := 0; i < 3; i++ {
		if resp, _ := h.getKey(key, mine); resp.StatusCode != http.StatusOK {
			t.Fatalf("the same browser with the link again: %d", resp.StatusCode)
		}
		if got := h.status("GET", "/", mine); got != http.StatusOK {
			t.Fatalf("the page reloaded: %d", got)
		}
	}
	if got := alarms.list(); len(got) != 0 {
		t.Fatalf("the person's own browser rang the alarm: %v", got)
	}
	if h.token() != key {
		t.Fatal("the key was replaced without an alarm")
	}

	// Somebody else, later, with the same link.
	resp, c := h.getKey(key, nil)
	if resp.StatusCode != http.StatusForbidden || c != nil {
		t.Fatalf("another browser later: %d", resp.StatusCode)
	}
	if got := alarms.wait(t, 1); got[0] != KeyUsedTwice {
		t.Fatalf("alarms %v", got)
	}
	if h.status("GET", "/", mine) != http.StatusForbidden || h.token() == key {
		t.Error("the alarm left the session or the key")
	}
	// The link of before the alarm is a dead one now: refused, and no more.
	if resp, _ := h.getKey(key, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("the old link: %d", resp.StatusCode)
	}
	if got := alarms.list(); len(got) != 1 {
		t.Errorf("the dead link rang again: %v", got)
	}
}

// Whoever held the other session may have sent the agent a message before
// the alarm. So the alarm also stops what runs and refuses what waits:
// every running turn gets the panel's own "stop", every permission request
// that waits for an answer gets "deny" — the agent's own messages, as a
// person at the panel would send them.
func TestKey_AlarmStopsTurnsAndDeniesRequests(t *testing.T) {
	var alarms alarmLog
	h := newHarness(t, Options{OnIntruder: alarms.add})
	emit := func(lines ...string) {
		for _, l := range lines {
			if _, err := h.s.Out().Write([]byte(l + "\n")); err != nil {
				t.Fatal(err)
			}
		}
	}
	emit(
		`{"type":"chats","chats":[{"id":"old","label":"yesterday"},{"id":"woken","busy":true}]}`,
		`{"type":"turn_start","chat":"c1"}`,
		`{"type":"ask","chat":"c1","id":"q1","tool":"Bash","summary":"rm -rf build"}`,
		`{"type":"turn_start","chat":"c2"}`,
		`{"type":"ask_user","chat":"c2","id":"q2","question":"which one?"}`,
		`{"type":"ask_resolved","chat":"c2","ask_id":"q2","by":"web"}`,
		`{"type":"done","chat":"c2","text":"ok"}`,
		`{"type":"turn_start","chat":"c3"}`,
		`{"type":"ask","chat":"c3","id":"q3","tool":"Write"}`,
		// A turn that waits for its place among the running ones has begun
		// all the same.
		`{"type":"queued","chat":"c4","text":"waiting for a free slot"}`,
	)
	key := h.token()
	_, first := h.getKey(key, nil)
	// The person answers one request themselves.
	if got := func() int {
		saved := h.cookie
		h.cookie = first
		defer func() { h.cookie = saved }()
		return h.postIn(`{"type":"answer","id":"q3","value":"once"}`, nil).StatusCode
	}(); got != http.StatusNoContent {
		t.Fatalf("an answer: %d", got)
	}
	before := h.in.String()

	h.getKey(key, nil) // the second comer
	alarms.wait(t, 1)
	got := h.in.String()[len(before):]
	for _, want := range []string{
		`{"type":"answer","id":"q1","value":"deny"}`,
		`{"type":"stop","chat":"c1"}`,
		`{"type":"stop","chat":"c3"}`,
		`{"type":"stop","chat":"c4"}`,
		`{"type":"stop","chat":"woken"}`,
	} {
		if strings.Count(got, want+"\n") != 1 {
			t.Errorf("the agent was not sent %s once:\n%s", want, got)
		}
	}
	for _, not := range []string{`"q2"`, `"q3"`, `"chat":"c2"`, `"chat":"old"`} {
		if strings.Contains(got, not) {
			t.Errorf("the agent was sent something about %s:\n%s", not, got)
		}
	}
	// Refusals first: a turn that is stopped while its request still waits
	// must not see the request granted by a late answer.
	if strings.Index(got, `"deny"`) > strings.Index(got, `"type":"stop"`) {
		t.Errorf("turns were stopped before the requests were refused:\n%s", got)
	}
}

// The key was used and no page ever connected: whoever took the link was
// not a panel in a browser.
func TestKey_UsedAndNoPageCame(t *testing.T) {
	var alarms alarmLog
	h := newHarness(t, Options{OnIntruder: alarms.add, ConnectWait: 80 * time.Millisecond})
	key := h.token()
	_, taken := h.getKey(key, nil)
	if taken == nil {
		t.Fatal("no session")
	}
	if got := alarms.wait(t, 1); got[0] != KeyUsedNoPage {
		t.Fatalf("alarms %v", got)
	}
	if h.status("GET", "/", taken) != http.StatusForbidden || h.token() == key {
		t.Error("the session or the key outlived the alarm")
	}

	// A page that connects in time is a panel: no alarm.
	alarms = alarmLog{}
	h = newHarness(t, Options{OnIntruder: alarms.add, ConnectWait: 80 * time.Millisecond})
	_, mine := h.getKey(h.token(), nil)
	h.cookie = mine
	_, closeStream := h.openEvents(nil)
	defer closeStream()
	time.Sleep(160 * time.Millisecond)
	if got := alarms.list(); len(got) != 0 {
		t.Fatalf("alarms %v", got)
	}
	if h.status("GET", "/", mine) != http.StatusOK {
		t.Error("the panel was signed out")
	}
}

// OnSignIn — the panel learns that its key was used (it then removes the
// file that carried the key to the window).
func TestKey_OnSignIn(t *testing.T) {
	var mu sync.Mutex
	n := 0
	h := newHarness(t, Options{OnSignIn: func() { mu.Lock(); n++; mu.Unlock() }})
	h.getKey(strings.Repeat("0", 64), nil)
	mu.Lock()
	if n != 0 {
		t.Fatal("a wrong key signed in")
	}
	mu.Unlock()
	h.login()
	h.getKey(h.token(), h.cookie)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := n
		mu.Unlock()
		if got == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("OnSignIn called %d times", n)
}
