package uiserve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuf — a fake agent input that just records lines.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *lockedBuf) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedBuf) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

type harness struct {
	t      *testing.T
	s      *Server
	ts     *httptest.Server
	in     *lockedBuf
	base   string // http://127.0.0.1:<port>
	origin string
	cookie *http.Cookie
}

// newHarness — a Server on a real loopback port (Host and Origin are checked
// against it) behind httptest.
func newHarness(t *testing.T, opts Options) *harness {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	opts.Port = ln.Addr().(*net.TCPAddr).Port
	if opts.Cwd == "" {
		opts.Cwd = t.TempDir()
	}
	in := &lockedBuf{}
	s, err := New(opts, in)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(s.Handler())
	_ = ts.Listener.Close()
	ts.Listener = ln
	ts.Start()
	t.Cleanup(func() {
		s.Close()
		ts.Close()
	})
	base := "http://127.0.0.1:" + strconv.Itoa(opts.Port)
	return &harness{t: t, s: s, ts: ts, in: in, base: base, origin: base}
}

var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func (h *harness) req(method, path string, body io.Reader, hdr map[string]string) *http.Response {
	h.t.Helper()
	r, err := http.NewRequest(method, h.base+path, body)
	if err != nil {
		h.t.Fatal(err)
	}
	for k, v := range hdr {
		if k == "Host" {
			r.Host = v
			continue
		}
		r.Header.Set(k, v)
	}
	if h.cookie != nil && hdr["Cookie"] == "" {
		r.AddCookie(h.cookie)
	}
	resp, err := noRedirect.Do(r)
	if err != nil {
		h.t.Fatal(err)
	}
	return resp
}

func (h *harness) token() string {
	u, _ := url.Parse(h.s.URL())
	return u.Query().Get("t")
}

// login trades the token for the cookie, as the browser does.
func (h *harness) login() {
	h.t.Helper()
	resp := h.req("GET", "/?t="+h.token(), nil, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		h.t.Fatalf("token exchange: %d", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == h.s.cookieName() {
			h.cookie = c
		}
	}
	if h.cookie == nil {
		h.t.Fatal("no session cookie")
	}
}

func (h *harness) postIn(body string, hdr map[string]string) *http.Response {
	all := map[string]string{"Content-Type": "application/json", "Origin": h.origin}
	for k, v := range hdr {
		all[k] = v
	}
	resp := h.req("POST", "/in", strings.NewReader(body), all)
	resp.Body.Close()
	return resp
}

func TestToken_CookieRedirectOnce(t *testing.T) {
	h := newHarness(t, Options{Version: "6.98-test", Locale: "en"})
	if !strings.HasPrefix(h.s.URL(), h.base+"/?t=") || len(h.token()) != 64 {
		t.Fatalf("URL %s", h.s.URL())
	}

	// A link preview (HEAD) and a wrong token open nothing and do not burn
	// the right one.
	resp := h.req("HEAD", "/?t="+h.token(), nil, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("HEAD: %d", resp.StatusCode)
	}
	resp = h.req("GET", "/?t="+strings.Repeat("0", 64), nil, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong token: %d", resp.StatusCode)
	}

	resp = h.req("GET", "/?t="+h.token(), nil, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("exchange: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	var c *http.Cookie
	for _, x := range resp.Cookies() {
		if x.Name == "execai_ui_"+strconv.Itoa(h.s.opts.Port) {
			c = x
		}
	}
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatalf("cookie %+v", c)
	}
	if c.Value == h.token() || len(c.Value) != 64 {
		t.Fatal("the cookie must be a separate secret, not the token")
	}
	if resp.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Error("the token page must not leak a referrer")
	}

	// The token works once: a second browser without the cookie is refused…
	resp = h.req("GET", "/?t="+h.token(), nil, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("second use: %d", resp.StatusCode)
	}
	// …the same browser (cookie) opening the link again just goes on.
	h.cookie = c
	resp = h.req("GET", "/?t="+h.token(), nil, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("second use with the cookie: %d", resp.StatusCode)
	}

	// The page: placeholders filled, the nonce of the CSP header in the page.
	resp = h.req("GET", "/", nil, nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("page: %d", resp.StatusCode)
	}
	csp := resp.Header.Get("Content-Security-Policy")
	m := regexp.MustCompile(`script-src 'nonce-([0-9a-f]+)'`).FindStringSubmatch(csp)
	if m == nil || !strings.Contains(csp, "connect-src 'self'") || !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("CSP %q", csp)
	}
	if !strings.Contains(string(body), `nonce="`+m[1]+`"`) {
		t.Error("the page's script nonce differs from the CSP")
	}
	if strings.Contains(string(body), "{{NONCE}}") || strings.Contains(string(body), "{{CONFIG}}") {
		t.Error("placeholders left in the page")
	}
	if !strings.Contains(string(body), `"version":"6.98-test"`) || !strings.Contains(string(body), `"upload":"/upload"`) {
		t.Error("CONFIG not in the page")
	}
}

func TestRefusals(t *testing.T) {
	h := newHarness(t, Options{})
	// Nothing without the cookie.
	for _, p := range []string{"/", "/events"} {
		resp := h.req("GET", p, nil, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s without the cookie: %d", p, resp.StatusCode)
		}
	}
	if resp := h.postIn(`{"type":"ping"}`, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("POST /in without the cookie: %d", resp.StatusCode)
	}

	h.login()
	cases := []struct {
		name   string
		method string
		path   string
		hdr    map[string]string
	}{
		{"foreign Host (DNS rebinding)", "GET", "/", map[string]string{"Host": "evil.example:" + strconv.Itoa(h.s.opts.Port)}},
		{"foreign Host on the token", "GET", "/?t=" + h.token(), map[string]string{"Host": "evil.example"}},
		{"SSE from another origin", "GET", "/events", map[string]string{"Origin": "http://evil.example"}},
		{"SSE cross-site without Origin", "GET", "/events", map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{"POST /in from another origin", "POST", "/in", map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}},
		{"POST /in from another port", "POST", "/in", map[string]string{"Origin": "http://127.0.0.1:1", "Content-Type": "application/json"}},
		{"POST /in without Origin", "POST", "/in", map[string]string{"Content-Type": "application/json"}},
		{"upload from another origin", "POST", "/upload", map[string]string{"Origin": "http://evil.example", "Content-Type": "multipart/form-data; boundary=x"}},
		{"a forged cookie", "GET", "/", map[string]string{"Cookie": h.s.cookieName() + "=" + strings.Repeat("a", 64)}},
	}
	for _, c := range cases {
		resp := h.req(c.method, c.path, strings.NewReader(`{"type":"ping"}`), c.hdr)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d", c.name, resp.StatusCode)
		}
	}
	if got := h.in.String(); got != "" {
		t.Errorf("a refused request reached the agent: %q", got)
	}

	// Bad messages never reach the agent either.
	bad := []struct {
		body, ctype string
		want        int
	}{
		{`{"type":"ping"}`, "text/plain", http.StatusUnsupportedMediaType},
		{`[1,2]`, "application/json", http.StatusBadRequest},
		{`{"text":"no type"}`, "application/json", http.StatusBadRequest},
		{`{"type":"eof"}`, "application/json", http.StatusBadRequest},
		{`{"type":"web_task","text":"x"}`, "application/json", http.StatusBadRequest},
		{`{"type":"user","text":"` + strings.Repeat("x", 1<<20) + `"}`, "application/json", http.StatusRequestEntityTooLarge},
	}
	for _, b := range bad {
		if resp := h.postIn(b.body, map[string]string{"Content-Type": b.ctype}); resp.StatusCode != b.want {
			t.Errorf("%.40s: %d, want %d", b.body, resp.StatusCode, b.want)
		}
	}
	if got := h.in.String(); got != "" {
		t.Errorf("a bad message reached the agent: %q", got)
	}

	// Same request from the right origin: one compact line.
	if resp := h.postIn("{\n  \"type\": \"user\",\n  \"text\": \"a\\nb\"\n}", map[string]string{"Content-Type": "application/json; charset=utf-8"}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /in: %d", resp.StatusCode)
	}
	if got := h.in.String(); got != `{"type":"user","text":"a\nb"}`+"\n" {
		t.Errorf("line %q", got)
	}
}

// sseReader reads data lines of an event stream.
type sseReader struct {
	t     *testing.T
	lines chan string
}

func (h *harness) openEvents(hdr map[string]string) (*sseReader, func()) {
	h.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r, _ := http.NewRequestWithContext(ctx, "GET", h.base+"/events", nil)
	r.AddCookie(h.cookie)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		h.t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		h.t.Fatalf("events: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	sr := &sseReader{t: h.t, lines: make(chan string, 256)}
	go func() {
		defer close(sr.lines)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
		for sc.Scan() {
			if d, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				sr.lines <- d
			}
		}
	}()
	return sr, func() { cancel(); resp.Body.Close() }
}

func (sr *sseReader) next() string {
	sr.t.Helper()
	select {
	case l, ok := <-sr.lines:
		if !ok {
			sr.t.Fatal("stream closed")
		}
		return l
	case <-time.After(5 * time.Second):
		sr.t.Fatal("no event")
	}
	return ""
}

func typeOf(line string) string {
	var o struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(line), &o)
	if o.Text != "" {
		return o.Type + ":" + o.Text
	}
	return o.Type
}

func TestEvents_ReplayThenLive(t *testing.T) {
	h := newHarness(t, Options{})
	h.login()
	out := h.s.Out()
	enc := json.NewEncoder(out)
	for _, o := range []map[string]string{
		{"type": "notice", "text": "before"},
		{"type": "state", "text": "old"},
		{"type": "ready"},
		{"type": "text_delta", "text": "x", "chat": "c1"},
		{"type": "chats"},
		{"type": "agents"},
		{"type": "state", "text": "new"},
		{"type": "inbox"},
	} {
		_ = enc.Encode(o)
	}
	// A line written in two pieces is still one event.
	_, _ = out.Write([]byte(`{"type":"tool_`))

	sr, stop := h.openEvents(map[string]string{"Origin": h.origin})
	defer stop()
	var got []string
	for i := 0; i < 5; i++ {
		got = append(got, typeOf(sr.next()))
	}
	if strings.Join(got, " ") != "ready state:new inbox chats agents" {
		t.Fatalf("replay %v", got)
	}
	_, _ = out.Write([]byte(`call","text":"live"}` + "\n"))
	if l := typeOf(sr.next()); l != "tool_call:live" {
		t.Fatalf("live %q", l)
	}

	// A second tab (EventSource without Origin, same-origin fetch metadata)
	// gets the same snapshots and the same live events.
	sr2, stop2 := h.openEvents(map[string]string{"Sec-Fetch-Site": "same-origin"})
	for i := 0; i < 5; i++ {
		sr2.next()
	}
	_ = enc.Encode(map[string]string{"type": "done"})
	if typeOf(sr.next()) != "done" || typeOf(sr2.next()) != "done" {
		t.Fatal("fan-out")
	}
	stop2()
	deadline := time.Now().Add(5 * time.Second)
	for h.s.hub.count() != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("a closed tab is still subscribed: %d", h.s.hub.count())
		}
		_ = enc.Encode(map[string]string{"type": "pong"}) // a write notices the closed connection
		time.Sleep(20 * time.Millisecond)
	}
}

func TestHub_SlowClientDropped(t *testing.T) {
	hb := newHub()
	c, _, err := hb.subscribe()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= clientBuffer; i++ {
		_, _ = hb.Write([]byte(`{"type":"text_delta","text":"x"}` + "\n"))
	}
	if hb.count() != 0 {
		t.Fatal("a client that does not read must be dropped, not block the agent")
	}
	n := 0
	for range c.ch {
		n++
	}
	if n != clientBuffer {
		t.Fatalf("buffered %d", n)
	}
	hb.close()
	if _, _, err := hb.subscribe(); err == nil {
		t.Fatal("subscribe after close")
	}
	if _, err := hb.Write([]byte("{\"type\":\"done\"}\n")); err != nil {
		t.Fatal("writes after close must not fail")
	}
}

// The whole path through Run: POST /in reaches the agent's input as a line,
// the agent's output reaches the tab; a tab closing is not the end of the
// agent, Ctrl+C (the context) is.
func TestRun_InReachesAgent(t *testing.T) {
	agentDone := make(chan struct{})
	fake := func(ctx context.Context, in io.Reader, out io.Writer) error {
		defer close(agentDone)
		enc := json.NewEncoder(out)
		_ = enc.Encode(map[string]string{"type": "ready"})
		lines := make(chan string)
		go func() {
			sc := bufio.NewScanner(in)
			for sc.Scan() {
				lines <- sc.Text()
			}
			close(lines)
		}()
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					return io.ErrUnexpectedEOF // the input must not end while the process lives
				}
				_ = enc.Encode(map[string]string{"type": "notice", "text": "got " + l})
			case <-ctx.Done():
				return nil
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	urls := make(chan string, 1)
	runErr := make(chan error, 1)
	go func() {
		runErr <- Run(ctx, Options{Cwd: t.TempDir()}, fake, func(u string) { urls <- u })
	}()
	var u string
	select {
	case u = <-urls:
	case <-time.After(5 * time.Second):
		t.Fatal("not started")
	}
	parsed, _ := url.Parse(u)
	if parsed.Hostname() != "127.0.0.1" {
		t.Fatalf("listens on %s", parsed.Host)
	}
	base := "http://" + parsed.Host

	// A browser: follows the redirect, keeps the cookie.
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	resp, err := browser.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Request.URL.RawQuery != "" {
		t.Fatalf("landing: %d %s", resp.StatusCode, resp.Request.URL)
	}

	evCtx, evCancel := context.WithCancel(context.Background())
	r, _ := http.NewRequestWithContext(evCtx, "GET", base+"/events", nil)
	evResp, err := browser.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	events := bufio.NewScanner(evResp.Body)
	nextData := func() string {
		for events.Scan() {
			if d, ok := strings.CutPrefix(events.Text(), "data: "); ok {
				return d
			}
		}
		t.Fatal("stream ended")
		return ""
	}
	if typeOf(nextData()) != "ready" {
		t.Fatal("no ready replay")
	}
	post, _ := http.NewRequest("POST", base+"/in", strings.NewReader(`{"type":"user","text":"hi"}`))
	post.Header.Set("Content-Type", "application/json")
	post.Header.Set("Origin", base)
	resp, err = browser.Do(post)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /in: %d", resp.StatusCode)
	}
	if got := typeOf(nextData()); got != `notice:got {"type":"user","text":"hi"}` {
		t.Fatalf("echo %q", got)
	}

	// The tab closes: the agent keeps running.
	evCancel()
	evResp.Body.Close()
	select {
	case <-agentDone:
		t.Fatal("closing the tab ended the agent")
	case err := <-runErr:
		t.Fatalf("Run returned on a closed tab: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop on the context")
	}
	if _, err := http.Get(base + "/"); err == nil {
		t.Error("still listening after stop")
	}
}

func multipartBody(t *testing.T, field, name string, data []byte) (io.Reader, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	_ = mw.WriteField("note", "ignored")
	fw, err := mw.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(data)
	_ = mw.Close()
	return &b, mw.FormDataContentType()
}

func (h *harness) upload(name string, data []byte) (*http.Response, map[string]string) {
	h.t.Helper()
	body, ct := multipartBody(h.t, "file", name, data)
	resp := h.req("POST", "/upload", body, map[string]string{"Content-Type": ct, "Origin": h.origin})
	defer resp.Body.Close()
	var out map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestUpload_ConfinedToProject(t *testing.T) {
	cwd := t.TempDir()
	h := newHarness(t, Options{Cwd: cwd})
	h.login()

	resp, out := h.upload("../../etc/passwd", []byte("hello"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload: %d", resp.StatusCode)
	}
	dir := filepath.Join(cwd, ".execai", "uploads")
	if filepath.Dir(out["path"]) != dir || !strings.HasSuffix(out["name"], "-passwd") || filepath.Base(out["path"]) != out["name"] {
		t.Fatalf("saved as %v", out)
	}
	if b, err := os.ReadFile(out["path"]); err != nil || string(b) != "hello" {
		t.Fatalf("content %q %v", b, err)
	}

	// Same name again: a new file, the first one untouched.
	_, out2 := h.upload("../../etc/passwd", []byte("second"))
	if out2["path"] == out["path"] {
		t.Fatal("a file was overwritten")
	}

	maxUploadBytes = 8
	defer func() { maxUploadBytes = 25 << 20 }()
	if resp, _ := h.upload("big.bin", []byte("0123456789")); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("too large: %d", resp.StatusCode)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("a refused upload left a file: %d entries", len(entries))
	}
	maxUploadBytes = 25 << 20

	// The uploads folder made a symlink out of the project: refused, nothing
	// written outside.
	outside := t.TempDir()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, dir); err != nil {
		t.Skip("no symlinks here")
	}
	if resp, _ := h.upload("x.txt", []byte("x")); resp.StatusCode != http.StatusForbidden {
		t.Errorf("symlinked uploads: %d", resp.StatusCode)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Error("a file was written outside the project")
	}
}

func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{
		"shot.png":                        "shot.png",
		"../../etc/passwd":                "passwd",
		`C:\Users\me\a b.txt`:             "a_b.txt",
		".bashrc":                         "bashrc",
		"..":                              "file",
		"":                                "file",
		"отчёт 2026.pdf":                  "отчёт_2026.pdf",
		"a/b/<script>.html":               "script_.html",
		"x" + "\x00" + "y.sh":             "x_y.sh",
		strings.Repeat("a", 300) + ".png": strings.Repeat("a", 96) + ".png",
	} {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPageLanguage(t *testing.T) {
	langOf := func(h *harness, accept string) string {
		t.Helper()
		resp := h.req("GET", "/", nil, map[string]string{"Accept-Language": accept})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		m := regexp.MustCompile(`"lang":"(\w+)"`).FindSubmatch(body)
		if m == nil {
			t.Fatalf("no lang in the page")
		}
		return string(m[1])
	}
	t.Setenv("EXECAI_LOCALE", "")
	h := newHarness(t, Options{})
	h.login()
	for accept, want := range map[string]string{
		"":                                 "en",
		"ru-RU,ru;q=0.9,en;q=0.8":          "ru",
		"de-DE,de;q=0.9,ru;q=0.8,en;q=0.5": "ru",
		"de-DE,en;q=0.9,ru;q=0.5":          "en",
		"zh-CN":                            "en",
	} {
		if got := langOf(h, accept); got != want {
			t.Errorf("Accept-Language %q: %s, want %s", accept, got, want)
		}
	}

	t.Setenv("EXECAI_LOCALE", "ru_RU.UTF-8")
	h = newHarness(t, Options{})
	h.login()
	if got := langOf(h, "en"); got != "ru" {
		t.Errorf("EXECAI_LOCALE: %s", got)
	}
	h = newHarness(t, Options{Locale: "en"})
	h.login()
	if got := langOf(h, "ru"); got != "en" {
		t.Errorf("config locale first: %s", got)
	}
	h = newHarness(t, Options{Locale: "de"})
	h.login()
	if got := langOf(h, "ru"); got != "en" {
		t.Errorf("a configured language the panel lacks: %s", got)
	}
}
