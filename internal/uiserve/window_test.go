package uiserve

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/brand"
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
)

func (h *harness) body(method, path string, body io.Reader, hdr map[string]string) (*http.Response, string) {
	h.t.Helper()
	resp := h.req(method, path, body, hdr)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

func (h *harness) postLogin(code string, hdr map[string]string) *http.Response {
	all := map[string]string{"Content-Type": "application/json", "Origin": h.origin}
	for k, v := range hdr {
		if v == "" {
			delete(all, k)
			continue
		}
		all[k] = v
	}
	b, _ := json.Marshal(map[string]string{"code": code})
	resp := h.req("POST", "/login", bytes.NewReader(b), all)
	resp.Body.Close()
	return resp
}

// The manifest and the icon are what a browser needs to make the page an
// app of its own ("Add to Dock"); they carry nothing of the project and are
// served without the session, but only to this host name.
func TestStatic_ManifestAndIcon(t *testing.T) {
	h := newHarness(t, Options{Cwd: "/secret/project", Version: "9.9-secret"})
	resp, body := h.body("GET", "/manifest.webmanifest", nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/manifest+json") {
		t.Fatalf("manifest: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	var m struct {
		Name       string `json:"name"`
		ShortName  string `json:"short_name"`
		StartURL   string `json:"start_url"`
		Scope      string `json:"scope"`
		Display    string `json:"display"`
		ThemeColor string `json:"theme_color"`
		Background string `json:"background_color"`
		Icons      []struct {
			Src, Sizes, Type string
		} `json:"icons"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("manifest %q: %v", body, err)
	}
	if m.Name != "ExecAI" || m.ShortName != "ExecAI" || m.StartURL != "/" || m.Scope != "/" || m.Display != "standalone" ||
		m.ThemeColor == "" || m.Background == "" {
		t.Errorf("manifest %+v", m)
	}
	// The window an installed page opens in is painted with these before the
	// page is there: dark, as the page will be.
	if m.ThemeColor != "#15171c" || m.Background != "#15171c" {
		t.Errorf("the window opens in %s with a %s frame, want the dark #15171c for both", m.Background, m.ThemeColor)
	}
	if len(m.Icons) == 0 || m.Icons[0].Src != "/icon.png" || m.Icons[0].Sizes != "256x256" || m.Icons[0].Type != "image/png" {
		t.Errorf("icons %+v", m.Icons)
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "127.0.0.1") {
		t.Errorf("the manifest tells about the project or the address: %s", body)
	}

	resp, body = h.body("GET", "/icon.png", nil, nil)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" || body != string(brand.IconPNG) {
		t.Fatalf("icon: %d %s, %d bytes", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
	}

	// Another site must not learn from them that a panel runs here: a
	// browser says where a request comes from, and only the page itself
	// (same-origin) and the browser's own fetches (none) get an answer.
	for _, p := range []string{"/manifest.webmanifest", "/icon.png"} {
		for site, want := range map[string]int{
			"same-origin": http.StatusOK, "none": http.StatusOK, "": http.StatusOK,
			"cross-site": http.StatusForbidden, "same-site": http.StatusForbidden, "weird": http.StatusForbidden,
		} {
			hdr := map[string]string{}
			if site != "" {
				hdr["Sec-Fetch-Site"] = site
			}
			resp := h.req("GET", p, nil, hdr)
			resp.Body.Close()
			if resp.StatusCode != want {
				t.Errorf("%s with Sec-Fetch-Site %q: %d, want %d", p, site, resp.StatusCode, want)
			}
		}
	}
	for _, p := range []string{"/manifest.webmanifest", "/icon.png"} {
		resp := h.req("GET", p, nil, map[string]string{"Host": "evil.example:" + strconv.Itoa(h.s.opts.Port)})
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s for a foreign Host: %d", p, resp.StatusCode)
		}
		resp = h.req("POST", p, nil, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("POST %s: %d", p, resp.StatusCode)
		}
	}
}

var metaCSP = regexp.MustCompile(`(?s)<meta http-equiv="Content-Security-Policy"\s+content="([^"]*)">`)

// The page says what an installed web app needs: the manifest, the touch
// icon, the colour of the frame (dark, whatever the system is set to: the
// window is dark until the person picks a theme), a title without the address —
// and both policies (the header and the page's own) let the browser fetch
// them.
func TestPage_HeadForWebApp(t *testing.T) {
	check := func(t *testing.T, resp *http.Response, body string) {
		t.Helper()
		head, _, ok := strings.Cut(body, "</head>")
		if !ok {
			t.Fatal("no head")
		}
		for _, want := range []string{
			`<link rel="manifest" href="/manifest.webmanifest">`,
			`<link rel="icon" type="image/png" href="/icon.png">`,
			`<link rel="apple-touch-icon" href="/icon.png">`,
			`<meta name="theme-color" content="#15171c">`,
			`<meta name="apple-mobile-web-app-title" content="ExecAI">`,
		} {
			if strings.Count(head, want) != 1 {
				t.Errorf("the head has %d of %s", strings.Count(head, want), want)
			}
		}
		title := regexp.MustCompile(`<title>([^<]*)</title>`).FindStringSubmatch(head)
		if title == nil || title[1] != "ExecAI" {
			t.Errorf("title %q", title)
		}
		for name, csp := range map[string]string{"header": resp.Header.Get("Content-Security-Policy"), "meta": ""} {
			if name == "meta" {
				m := metaCSP.FindStringSubmatch(head)
				if m == nil {
					continue // a page without its own policy has the header's
				}
				csp = m[1]
			}
			for _, want := range []string{"default-src 'none'", "manifest-src 'self'", "img-src 'self' data: blob:", "connect-src 'self'", "script-src 'nonce-"} {
				if !strings.Contains(csp, want) {
					t.Errorf("%s policy lacks %s: %s", name, want, csp)
				}
			}
			if strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "*") || strings.Contains(csp, "script-src 'self'") {
				t.Errorf("%s policy got wider: %s", name, csp)
			}
		}
		if n := len(metaCSP.FindAllString(body, -1)); n > 1 {
			t.Errorf("%d policies in the page", n)
		}
	}
	h := newHarness(t, Options{Locale: "ru"})
	h.login()
	resp, body := h.body("GET", "/", nil, nil)
	check(t, resp, body)

	// Every embedded page takes the head: both languages of the panel and
	// the stand-in.
	entries, err := assets.ReadDir("assets")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		raw, _ := assets.ReadFile("assets/" + e.Name())
		out := string(prepare(raw, "abc", []byte("{}"), false))
		t.Run(e.Name(), func(t *testing.T) {
			check(t, &http.Response{Header: http.Header{"Content-Security-Policy": {pagePolicy("abc", true)}}}, out)
			if !strings.Contains(out, `nonce="abc"`) || strings.Contains(out, "{{NONCE}}") {
				t.Error("nonce not filled")
			}
		})
	}
}

// In the app window links leave for the person's own browser: the page asks
// the agent to open them.
func TestOpenExternal(t *testing.T) {
	var mu sync.Mutex
	var opened []string
	h := newHarness(t, Options{OpenExternal: func(u string) error {
		mu.Lock()
		opened = append(opened, u)
		mu.Unlock()
		return nil
	}})
	post := func(u string, hdr map[string]string) int {
		all := map[string]string{"Content-Type": "application/json", "Origin": h.origin}
		for k, v := range hdr {
			all[k] = v
		}
		b, _ := json.Marshal(map[string]string{"url": u})
		resp := h.req("POST", "/open", bytes.NewReader(b), all)
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := post("https://execai.ru/", nil); got != http.StatusForbidden {
		t.Fatalf("without the session: %d", got)
	}
	h.login()
	if got := post("https://execai.ru/", map[string]string{"Origin": "http://evil.example"}); got != http.StatusForbidden {
		t.Fatalf("from another origin: %d", got)
	}
	for _, bad := range []string{"https://user:pass@execai.ru/", "https://user@execai.ru/", "https://execai.ru/a b", "https://execai.ru/\nx",
		"https://execai.ru/\tx", "mailto:", "mailto:?attach=/etc/passwd", "mailto:%0d%0a", "mailto:%zz@b.c",
		"file:///etc/passwd", "file://localhost/etc/passwd", "ftp://example.com/x", "javascript:alert(1)", "chrome://settings", "ms-settings:", "//execai.ru", "execai.ru",
		"", " https://execai.ru", "https://", "http://exa mple.com", "vscode://file/x", "https://execai.ru/" + strings.Repeat("a", 5000)} {
		if got := post(bad, nil); got != http.StatusBadRequest {
			t.Errorf("%.40q: %d", bad, got)
		}
	}
	if len(opened) != 0 {
		t.Fatalf("opened %q", opened)
	}
	for _, good := range []string{"https://execai.ru/agents/connect/ABCD", "http://localhost:3000/", "mailto:support@execai.ru"} {
		if got := post(good, nil); got != http.StatusNoContent {
			t.Errorf("%s: %d", good, got)
		}
	}
	mu.Lock()
	if len(opened) != 3 || opened[0] != "https://execai.ru/agents/connect/ABCD" {
		t.Errorf("opened %q", opened)
	}
	opened = nil
	mu.Unlock()

	// (Each part of this test starts with the limit on links untouched.)
	fresh := func() {
		h.s.openMu.Lock()
		h.s.openTimes = nil
		h.s.openMu.Unlock()
	}
	fresh()

	// What the opener gets is the address as parsed and written out again,
	// and of a mailto only who, about what and what — nothing that attaches
	// a file or names a program's own option.
	for in, want := range map[string]string{
		"HTTPS://ExecAI.ru/Path?q=1#frag": "https://ExecAI.ru/Path?q=1#frag",
		"mailto:support@execai.ru?subject=Hi&attach=/etc/passwd&body=text&Attachment=x&cc=a@b.c&bcc=d@e.f&x-evil=1": "mailto:support@execai.ru?bcc=d%40e.f&body=text&cc=a%40b.c&subject=Hi",
		"mailto:a@b.c,d@e.f": "mailto:a@b.c,d@e.f",
		// The address of a mailto is written out anew as well: no line
		// breaks or other control characters however they are spelled (a
		// line break there is how a header is slipped into a letter), a
		// quote encoded.
		"mailto:a@b.c%0D%0ABcc:evil@x.y":                     "mailto:a@b.cBcc:evil@x.y",
		"mailto:a@b.c%0a%09%00%7F":                           "mailto:a@b.c",
		`mailto:"quoted"@b.c`:                                "mailto:%22quoted%22@b.c",
		"mailto:a%22b@c.d":                                   "mailto:a%22b@c.d",
		"mailto:a@b.c?subject=Hi%0d%0aBcc:x@y.z&cc=c@d.e%0A": "mailto:a@b.c?cc=c%40d.e&subject=HiBcc%3Ax%40y.z",
		"mailto:a@b.c?body=line1%0Aline2":                    "mailto:a@b.c?body=line1%0Aline2",
	} {
		fresh()
		if got := post(in, nil); got != http.StatusNoContent {
			t.Errorf("%s: %d", in, got)
			continue
		}
		mu.Lock()
		if len(opened) == 0 || opened[len(opened)-1] != want {
			t.Errorf("%s was opened as %q, want %q", in, opened, want)
		}
		mu.Unlock()
	}

	// A page gone wrong cannot flood the desktop with windows.
	fresh()
	mu.Lock()
	opened = nil
	mu.Unlock()
	limited := 0
	for i := 0; i < 3*openBurst; i++ {
		switch got := post("https://execai.ru/"+strconv.Itoa(i), nil); got {
		case http.StatusNoContent:
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Fatalf("link %d: %d", i, got)
		}
	}
	mu.Lock()
	if len(opened) > openBurst || limited < 2*openBurst {
		t.Errorf("%d links opened in a burst (the limit is %d), %d refused", len(opened), openBurst, limited)
	}
	mu.Unlock()

	// The page of the window routes window.open there; a tab keeps the
	// browser's own way.
	// …and a window that comes up closes the older windows of the panel
	// (a second execai ui shows the panel by opening it again). A tab does
	// neither: the browser opens its links and the person keeps their tabs.
	const oneWindow = `var ch=new BroadcastChannel("execai-ui-window");`
	_, body := h.body("GET", "/", nil, nil)
	if !strings.Contains(body, `fetch("/open"`) {
		t.Error("the window's page does not route links to the agent")
	}
	if !strings.Contains(body, oneWindow) || !strings.Contains(body, `if(e.data==="shown"&&standalone()){shut()}`) {
		t.Error("the window's page does not close the older windows of the panel")
	}
	for what, want := range map[string]string{
		"closes itself when the panel stops":    `es.addEventListener("execai-close",function(){if(standalone()){shut()}})`,
		"asks before a close that cuts a turn":  `addEventListener("beforeunload",function(e){if(!closing&&Object.keys(busy).length){e.preventDefault();e.returnValue=""}})`,
		"says goodbye when it is closed":        `addEventListener("pagehide",function(){try{navigator.sendBeacon("/bye")}catch(x){}})`,
		"follows the turns of the event stream": `if(m.type==="turn_start"){busy[m.chat||""]=1}else if(m.type==="done"){delete busy[m.chat||""]}`,
		"keeps the browser's own event stream":  `W.prototype=ES.prototype`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the window's page no longer %s", what)
		}
	}
	tab := newHarness(t, Options{})
	tab.login()
	_, body = tab.body("GET", "/", nil, nil)
	if strings.Contains(body, `fetch("/open"`) || strings.Contains(body, "BroadcastChannel(\"execai-ui-window\")") ||
		strings.Contains(body, "window.close()") || strings.Contains(body, "beforeunload") || strings.Contains(body, "/bye") {
		t.Error("a tab's page routes links to the agent, closes windows or reports its closing")
	}
	b, _ := json.Marshal(map[string]string{"url": "https://execai.ru/"})
	resp := tab.req("POST", "/open", bytes.NewReader(b), map[string]string{"Content-Type": "application/json", "Origin": tab.origin})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /open in a tab: %d", resp.StatusCode)
	}
}

// The default port is the first free one of the list; the address without
// the token is the stable one a shortcut or the Dock can keep.
func TestRun_TryPorts(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	busyPort, freePort := busy.Addr().(*net.TCPAddr).Port, free.Addr().(*net.TCPAddr).Port
	free.Close()

	idle := func(ctx context.Context, in io.Reader, out io.Writer) error { <-ctx.Done(); return nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan *Server, 1)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Cwd: t.TempDir(), TryPorts: []int{busyPort, freePort}}, idle, func(s *Server) { got <- s })
	}()
	select {
	case s := <-got:
		if s.Port() != freePort {
			t.Errorf("port %d, want the next free one %d", s.Port(), freePort)
		}
		if s.StableURL() != "http://127.0.0.1:"+strconv.Itoa(freePort)+"/" || !strings.HasPrefix(s.URL(), s.StableURL()+"?t=") {
			t.Errorf("addresses %s %s", s.StableURL(), s.URL())
		}
		if s.TokenUsed() {
			t.Error("the token is used before anyone came")
		}
	case err := <-done:
		t.Fatalf("Run: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("not started")
	}
	cancel()
	<-done

	// A port the person asked for is the only one tried.
	err = Run(context.Background(), Options{Cwd: t.TempDir(), Port: busyPort, TryPorts: []int{freePort}}, idle, nil)
	if err == nil {
		t.Fatal("Run on a taken port the person asked for")
	}
}

// Viewers — how many pages hold an event stream: what tells a closed window
// where its browser goes on running.
func TestViewers(t *testing.T) {
	h := newHarness(t, Options{})
	h.login()
	if n := h.s.Viewers(); n != 0 {
		t.Fatalf("viewers before anyone came: %d", n)
	}
	_, closeFirst := h.openEvents(nil)
	_, closeSecond := h.openEvents(nil)
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
	waitViewers(2)
	closeFirst()
	waitViewers(1)
	closeSecond()
	waitViewers(0)
}

// POST /stop — how `execai ui --stop` asks the panel of a folder to end. It
// obeys only who knows the control secret of this run (the state file,
// readable by the person alone), and never a page.
func TestStop(t *testing.T) {
	var stops atomic.Int32
	h := newHarness(t, Options{OnStop: func() { stops.Add(1) }})
	secret := h.s.Control()
	if len(secret) != 64 || secret == h.token() {
		t.Fatalf("control secret %q", secret)
	}
	post := func(method string, hdr map[string]string) int {
		resp := h.req(method, "/stop", nil, hdr)
		resp.Body.Close()
		return resp.StatusCode
	}
	h.login() // a signed-in page is still not allowed to stop the panel
	for name, c := range map[string]struct {
		method string
		hdr    map[string]string
		want   int
	}{
		"no secret":             {"POST", nil, http.StatusForbidden},
		"a wrong secret":        {"POST", map[string]string{"X-Execai-Control": strings.Repeat("0", 64)}, http.StatusForbidden},
		"the token as secret":   {"POST", map[string]string{"X-Execai-Control": h.token()}, http.StatusForbidden},
		"from a page":           {"POST", map[string]string{"X-Execai-Control": secret, "Origin": h.origin}, http.StatusForbidden},
		"from another site":     {"POST", map[string]string{"X-Execai-Control": secret, "Origin": "http://evil.example"}, http.StatusForbidden},
		"a foreign Host":        {"POST", map[string]string{"X-Execai-Control": secret, "Host": "evil.example:" + strconv.Itoa(h.s.opts.Port)}, http.StatusForbidden},
		"GET with the secret":   {"GET", map[string]string{"X-Execai-Control": secret}, http.StatusMethodNotAllowed},
		"the secret in a query": {"POST", map[string]string{"Referer": h.base + "/stop?control=" + secret}, http.StatusForbidden},
	} {
		if got := post(c.method, c.hdr); got != c.want {
			t.Errorf("%s: %d, want %d", name, got, c.want)
		}
	}
	if n := stops.Load(); n != 0 {
		t.Fatalf("a refused request stopped the panel (%d)", n)
	}
	if got := post("POST", map[string]string{"X-Execai-Control": secret}); got != http.StatusNoContent {
		t.Fatalf("the right secret: %d", got)
	}
	deadline := time.Now().Add(5 * time.Second)
	for stops.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := stops.Load(); n != 1 {
		t.Fatalf("OnStop called %d times", n)
	}

	// A server nobody may stop this way has no such address, whoever asks.
	plain := newHarness(t, Options{})
	resp := plain.req("POST", "/stop", nil, map[string]string{"X-Execai-Control": plain.s.Control()})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /stop without OnStop: %d", resp.StatusCode)
	}
}
