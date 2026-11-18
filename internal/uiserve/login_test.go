package uiserve

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

var codeForm = regexp.MustCompile(`^[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}$`)

// askCode — what `execai ui --code` does: asks the panel for a code with the
// control secret.
func (h *harness) askCode(hdr map[string]string) (*http.Response, string) {
	h.t.Helper()
	all := map[string]string{"X-Execai-Control": h.s.Control()}
	for k, v := range hdr {
		all[k] = v
	}
	saved := h.cookie
	h.cookie = nil
	defer func() { h.cookie = saved }()
	resp := h.req("POST", "/code", nil, all)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var out struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &out)
	return resp, out.Code
}

// A window without the link (a web app in the Dock, another browser) gets
// the sign-in page, which tells nothing about the project. The way in by a
// code is shut until the person asks for a code in a terminal, and then
// open for that one code, for a few minutes.
func TestLogin_CodeOnRequestOnly(t *testing.T) {
	prev := i18n.Locale()
	i18n.SetLocale("en")
	defer i18n.SetLocale(prev)
	h := newHarness(t, Options{Cwd: "/secret/project", Version: "9.9-secret", Locale: "en"})

	resp, body := h.body("GET", "/", nil, nil)
	if resp.StatusCode != http.StatusForbidden || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("sign-in page: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !strings.Contains(body, `id="code"`) {
		t.Errorf("no code field:\n%s", body)
	}
	// The command comes first: it is what the person has to do.
	cmdAt, linkAt := strings.Index(body, "execai ui --code"), strings.Index(body, "Or open the link")
	if cmdAt < 0 || linkAt < 0 || cmdAt > linkAt {
		t.Errorf("the page names the command at %d and the link at %d:\n%s", cmdAt, linkAt, body)
	}
	for _, leak := range []string{"secret", h.token(), h.s.Control(), "{{"} {
		if strings.Contains(body, leak) {
			t.Errorf("the sign-in page shows %q", leak)
		}
	}
	csp := resp.Header.Get("Content-Security-Policy")
	nonce := regexp.MustCompile(`script-src 'nonce-([0-9a-f]+)'`).FindStringSubmatch(csp)
	if nonce == nil || !strings.Contains(body, `nonce="`+nonce[1]+`"`) || !strings.Contains(csp, "default-src 'none'") ||
		!strings.Contains(csp, "form-action 'none'") {
		t.Errorf("sign-in page policy %q", csp)
	}

	// Nobody asked for a code: there is nothing to guess, and nothing to
	// burn — however many tries. The answer is the one a wrong code gets
	// (refused below): from outside nobody can tell when a code is waiting.
	refusal := func(code string) (int, string) {
		t.Helper()
		b, _ := json.Marshal(map[string]string{"code": code})
		resp := h.req("POST", "/login", strings.NewReader(string(b)), map[string]string{"Content-Type": "application/json", "Origin": h.origin})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if len(resp.Cookies()) != 0 {
			t.Fatalf("a refusal with a cookie (%d)", resp.StatusCode)
		}
		return resp.StatusCode, string(body)
	}
	noCodeStatus, noCodeBody := refusal("AAAA-AAAA")
	if noCodeStatus != http.StatusForbidden {
		t.Fatalf("a try with no code asked for: %d", noCodeStatus)
	}
	for i := 0; i < 50; i++ {
		if st, body := refusal("AAAA-AAAA"); st != noCodeStatus || body != noCodeBody {
			t.Fatalf("try %d with no code asked for: %d %q", i, st, body)
		}
	}

	// Asking takes the control secret, and a page may not ask.
	for name, hdr := range map[string]map[string]string{
		"no secret":      {"X-Execai-Control": ""},
		"a wrong secret": {"X-Execai-Control": strings.Repeat("0", 64)},
		"from a page":    {"Origin": h.origin},
		"a foreign Host": {"Host": "evil.example:" + strconv.Itoa(h.s.opts.Port)},
	} {
		if resp, code := h.askCode(hdr); resp.StatusCode != http.StatusForbidden || code != "" {
			t.Errorf("a code asked for with %s: %d %q", name, resp.StatusCode, code)
		}
	}
	h.login()
	saved := h.cookie
	resp = h.req("POST", "/code", nil, nil) // a signed-in page, no secret
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a code asked for by a page with a session: %d", resp.StatusCode)
	}
	resp = h.req("GET", "/code", nil, map[string]string{"X-Execai-Control": h.s.Control()})
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /code: %d", resp.StatusCode)
	}
	h.cookie = nil

	resp, code := h.askCode(nil)
	if resp.StatusCode != http.StatusOK || !codeForm.MatchString(code) {
		t.Fatalf("asking for a code: %d %q", resp.StatusCode, code)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("the answer with the code may be stored")
	}

	// The right code from the wrong place is refused and is not a try.
	for name, hdr := range map[string]map[string]string{
		"another origin":    {"Origin": "http://evil.example"},
		"another port":      {"Origin": "http://127.0.0.1:1"},
		"no Origin":         {"Origin": ""},
		"a foreign Host":    {"Host": "evil.example:" + strconv.Itoa(h.s.opts.Port)},
		"not JSON (a form)": {"Content-Type": "application/x-www-form-urlencoded"},
	} {
		for i := 0; i < 2*maxCodeFails; i++ {
			resp := h.postLogin(code, hdr)
			if resp.StatusCode == http.StatusNoContent || len(resp.Cookies()) != 0 {
				t.Fatalf("%s: let in (%d)", name, resp.StatusCode)
			}
		}
	}
	// A wrong code is a try, a few of them are allowed — and each is
	// answered exactly as a try was when no code waited.
	for i := 0; i < maxCodeFails-1; i++ {
		if st, body := refusal("AAAA-AAAA"); st != noCodeStatus || body != noCodeBody {
			t.Fatalf("wrong try %d: %d %q, with no code it was %d %q", i+1, st, body, noCodeStatus, noCodeBody)
		}
	}

	// The code as the person types it: lower case, a space for the dash.
	resp = h.postLogin(" "+strings.ToLower(strings.ReplaceAll(code, "-", " "))+" ", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("the right code: %d", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == h.s.cookieName() {
			h.cookie = c
		}
	}
	if h.cookie == nil || !h.cookie.HttpOnly || h.cookie.SameSite != http.SameSiteStrictMode || len(h.cookie.Value) != 64 {
		t.Fatalf("cookie %+v", h.cookie)
	}
	resp, body = h.body("GET", "/", nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"version":"9.9-secret"`) {
		t.Fatalf("the page after the code: %d", resp.StatusCode)
	}
	byCode := h.cookie
	h.cookie = nil

	// One time: the code is used up, the way is shut again — with the same
	// answer.
	if st, body := refusal(code); st != noCodeStatus || body != noCodeBody {
		t.Errorf("the used code again: %d %q", st, body)
	}
	// The link's session and the code's live side by side.
	if h.status("GET", "/", saved) != http.StatusOK || h.status("GET", "/", byCode) != http.StatusOK {
		t.Error("a session was lost")
	}
}

// A code is burned by a few wrong tries and goes stale after a few
// minutes; a new request replaces the old code. Burning one does not shut
// the way for good: the person asks again.
func TestLogin_CodeBurnsAndExpires(t *testing.T) {
	h := newHarness(t, Options{CodeTTL: 120 * time.Millisecond})
	_, first := h.askCode(nil)
	for i := 0; i < maxCodeFails; i++ {
		if resp := h.postLogin("AAAA-AAAA", nil); resp.StatusCode != http.StatusForbidden {
			t.Fatalf("wrong try %d: %d", i+1, resp.StatusCode)
		}
	}
	if resp := h.postLogin(first, nil); resp.StatusCode != http.StatusForbidden || len(resp.Cookies()) != 0 {
		t.Fatalf("the code after %d wrong tries: %d", maxCodeFails, resp.StatusCode)
	}

	_, second := h.askCode(nil)
	_, third := h.askCode(nil)
	if second == first || third == second || !codeForm.MatchString(third) {
		t.Fatalf("codes %q %q %q", first, second, third)
	}
	if resp := h.postLogin(second, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a replaced code: %d", resp.StatusCode)
	}
	time.Sleep(200 * time.Millisecond)
	if resp := h.postLogin(third, nil); resp.StatusCode != http.StatusForbidden || len(resp.Cookies()) != 0 {
		t.Fatalf("a stale code: %d", resp.StatusCode)
	}
	// Tries while no code waits move no counter: the next code has all its
	// tries.
	for i := 0; i < 3*maxCodeFails; i++ {
		h.postLogin("AAAA-AAAA", nil)
	}

	_, fourth := h.askCode(nil)
	if resp := h.postLogin(fourth, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("a fresh code after all that: %d", resp.StatusCode)
	}
}

// POST /in refuses the types the agent keeps for itself however the field
// is spelled: the agent's reader takes "Type" for "type", and of two the
// last one.
func TestIn_InternalTypesInAnySpelling(t *testing.T) {
	h := newHarness(t, Options{})
	h.login()
	for _, body := range []string{
		`{"type":"eof"}`,
		`{"Type":"eof"}`,
		`{"TYPE":"web_task","text":"x"}`,
		`{"type":"x","Type":"eof"}`,
		`{"type":"user","text":"hi","tYpE":"web_task"}`,
		`{"Type":""}`,
		`{"type":"user","type":"eof"}`,
	} {
		if resp := h.postIn(body, nil); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d", body, resp.StatusCode)
		}
	}
	if got := h.in.String(); got != "" {
		t.Fatalf("reached the agent: %q", got)
	}
	// Spelled otherwise, an ordinary message is still one.
	if resp := h.postIn(`{"Type":"user","text":"hi"}`, nil); resp.StatusCode != http.StatusNoContent {
		t.Errorf("an ordinary message: %d", resp.StatusCode)
	}
}

// The window's page says goodbye when it is closed or reloaded (a page
// that crashed says nothing): PageLeft is true only when no page is
// connected and the last one to go said so.
func TestPageLeft(t *testing.T) {
	h := newHarness(t, Options{})
	bye := func(hdr map[string]string) int {
		all := map[string]string{"Origin": h.origin, "Content-Type": "text/plain;charset=UTF-8"}
		for k, v := range hdr {
			all[k] = v
		}
		resp := h.req("POST", "/bye", strings.NewReader(""), all)
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := bye(nil); got != http.StatusForbidden {
		t.Fatalf("a goodbye without a session: %d", got)
	}
	h.login()
	if got := bye(map[string]string{"Origin": "http://evil.example"}); got != http.StatusForbidden {
		t.Fatalf("a goodbye from another site: %d", got)
	}
	if h.s.PageLeft() {
		t.Fatal("left before anybody came")
	}
	waitViewers := func(want int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for h.s.Viewers() != want {
			if time.Now().After(deadline) {
				t.Fatalf("viewers %d, want %d", h.s.Viewers(), want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	// A crash: the stream ends without a word.
	_, closeStream := h.openEvents(nil)
	waitViewers(1)
	closeStream()
	waitViewers(0)
	if h.s.PageLeft() {
		t.Fatal("a page that vanished without a goodbye is taken for a closed window")
	}

	// A reload: goodbye, and the page is back.
	_, closeStream = h.openEvents(nil)
	waitViewers(1)
	if got := bye(nil); got != http.StatusNoContent {
		t.Fatalf("goodbye: %d", got)
	}
	if h.s.PageLeft() {
		t.Fatal("left while its stream is open")
	}
	closeStream()
	waitViewers(0)
	if !h.s.PageLeft() {
		t.Fatal("goodbye and gone, yet not left")
	}
	_, closeStream = h.openEvents(nil)
	waitViewers(1)
	if h.s.PageLeft() {
		t.Fatal("a page that came back counts as left")
	}
	// …and crashes now: its old goodbye does not speak for it.
	closeStream()
	waitViewers(0)
	if h.s.PageLeft() {
		t.Fatal("an earlier goodbye was taken for this page's")
	}
}

// When the panel stops, the pages of its app window are told to close
// themselves (a named event the panel's own handler never sees); a tab is
// told nothing, and neither is a window dropped for being slow or signed
// out by an alarm.
func TestCloseEventOnStop(t *testing.T) {
	read := func(h *harness) string {
		t.Helper()
		r, _ := http.NewRequest("GET", h.base+"/events", nil)
		r.AddCookie(h.cookie)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		got := make(chan string, 1)
		go func() {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			got <- string(b)
		}()
		deadline := time.Now().Add(5 * time.Second)
		for h.s.Viewers() == 0 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		return func() string {
			select {
			case s := <-got:
				return s
			case <-time.After(5 * time.Second):
				t.Fatal("the stream did not end")
				return ""
			}
		}()
	}
	const closeEvent = "event: execai-close\n"

	window := newHarness(t, Options{OpenExternal: func(string) error { return nil }})
	window.login()
	go func() {
		for window.s.Viewers() == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		window.s.Close()
	}()
	if got := read(window); !strings.Contains(got, closeEvent) {
		t.Errorf("the window's page was not told to close:\n%q", got)
	}

	tab := newHarness(t, Options{})
	tab.login()
	go func() {
		for tab.s.Viewers() == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		tab.s.Close()
	}()
	if got := read(tab); strings.Contains(got, closeEvent) {
		t.Errorf("a tab was told to close:\n%q", got)
	}

	alarmed := newHarness(t, Options{OpenExternal: func(string) error { return nil }})
	alarmed.login()
	go func() {
		for alarmed.s.Viewers() == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		alarmed.s.intruder(KeyUsedTwice)
	}()
	if got := read(alarmed); strings.Contains(got, closeEvent) {
		t.Errorf("a window signed out by an alarm was told to close:\n%q", got)
	}
}
