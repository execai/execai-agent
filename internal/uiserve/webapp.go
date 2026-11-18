package uiserve

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/brand"
)

// What makes the page a web app a browser can keep as a window of its own
// (Safari: File → Add to Dock; Chromium: Install): a manifest, an icon, the
// colours of the window frame, a title that is a name and not an address.
// The manifest and the icon are the same for everyone and say nothing of
// the project, so they are served without the session — the browser fetches
// them without cookies — but, like everything, only under this host name.

// frameDark — the colour of the window before anything is drawn in it and
// of its frame: the background of the look the window starts in (the
// workbench, dark). The window is dark until the person picks a theme,
// whatever the system is set to — so nothing here follows the system either,
// or the window would open as a light rectangle and then turn dark.
const frameDark = "#15171c"

// webAppHead — added to the <head> of every page served. The colour of the
// frame is the dark one; a page that knows better — the person picked a
// light theme, or another look — puts its own background there as soon as
// its script runs.
const webAppHead = `<link rel="manifest" href="/manifest.webmanifest">
<link rel="icon" type="image/png" href="/icon.png">
<link rel="apple-touch-icon" href="/icon.png">
<meta name="theme-color" content="` + frameDark + `">
<meta name="apple-mobile-web-app-title" content="ExecAI">
<meta name="application-name" content="ExecAI">
`

// manifest — the web app manifest. start_url is the stable address: the
// page for a browser with the session, the sign-in page for one without.
var manifest = func() []byte {
	type icon struct {
		Src     string `json:"src"`
		Sizes   string `json:"sizes"`
		Type    string `json:"type"`
		Purpose string `json:"purpose"`
	}
	b, _ := json.Marshal(struct {
		Name       string `json:"name"`
		ShortName  string `json:"short_name"`
		StartURL   string `json:"start_url"`
		Scope      string `json:"scope"`
		Display    string `json:"display"`
		Background string `json:"background_color"`
		Theme      string `json:"theme_color"`
		Icons      []icon `json:"icons"`
	}{"ExecAI", "ExecAI", "/", "/", "standalone", frameDark, frameDark,
		[]icon{{"/icon.png", "256x256", "image/png", "any"}}})
	return b
}()

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request, ctype string, body []byte) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// An image that loads tells the site that asked for it that a panel
	// runs on this computer. A browser says where a request comes from:
	// this page (same-origin) and the browser itself (none) are answered,
	// another site — on any port of this computer too — is not.
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	// The same bytes on every run: the browser may keep them.
	h.Set("Cache-Control", "public, max-age=86400")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	s.serveStatic(w, r, "application/manifest+json; charset=utf-8", manifest)
}

func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	s.serveStatic(w, r, "image/png", brand.IconPNG)
}

// pagePolicy — the Content-Security-Policy of the panel page: the webview's
// policy plus connect-src 'self' for /events, /in and /upload, and what the
// web app head needs — the manifest and the icon from this origin. Nothing
// may frame the page or post a form elsewhere. header=false is the copy in
// the page's own <meta>, where frame-ancestors has no meaning.
func pagePolicy(nonce string, header bool) string {
	p := "default-src 'none'; img-src 'self' data: blob:; style-src 'unsafe-inline'; " +
		"script-src 'nonce-" + nonce + "'; connect-src 'self'; manifest-src 'self'; base-uri 'none'; form-action 'none'"
	if header {
		p += "; frame-ancestors 'none'"
	}
	return p
}

// metaPolicy — the policy the panel build carries in its own <meta>. A
// browser applies both it and the header, so it is rewritten to the header's
// policy: left as built, it would refuse the manifest and the icon.
var metaPolicy = regexp.MustCompile(`<meta http-equiv="Content-Security-Policy"\s+content="[^"]*">`)

// windowScript — what the page does differently in an app window.
//
// Links: the panel's window.open goes to the agent (POST /open), which opens
// the link in the person's own browser. Left to the window, a link would
// open in the window's separate profile — a browser without their logins —
// and keep its process alive after the panel window is closed.
//
// One window: a second `execai ui` asks the running browser for the same
// address, and Chromium answers that with one more window, never by raising
// the first. So a window that comes up says so to the others of this
// address, and they close: the panel stays one window, and it is the one in
// front. Only an app window closes itself (display-mode: standalone, asked
// when the message comes: while the page is still loading the browser has
// not told it its mode yet) — a tab is the person's to keep or close.
//
// The panel's event stream is listened to as well — the browser's own
// EventSource, with two listeners added to the one the panel opens. A named
// event from the server (sayClose) closes the window when the panel stops.
// The turn events tell whether a turn is running: closing the window ends
// the panel, so while one runs the browser asks first (beforeunload) —
// unless it is the script itself that closes.
//
// Goodbye: a page that is closed or reloaded says so (POST /bye). Where the
// end of the browser cannot tell that the window was closed, this tells a
// closed window from a page that crashed.
const windowScript = `<script nonce="{{NONCE}}">(function(){var open=window.open;window.open=function(u){var s=String(u||""),a=arguments;` +
	`if(!/^(https?:|mailto:)/i.test(s)){return open.apply(window,a)}` +
	`fetch("/open",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({url:s})})` +
	`.then(function(r){if(!r.ok){open.apply(window,a)}},function(){open.apply(window,a)});return null};` +
	`var closing=false,busy={};function standalone(){return matchMedia("(display-mode: standalone)").matches}` +
	`function shut(){closing=true;window.close()}` +
	`try{var ch=new BroadcastChannel("execai-ui-window");` +
	`ch.onmessage=function(e){if(e.data==="shown"&&standalone()){shut()}};` +
	`ch.postMessage("shown")}catch(e){}` +
	`var ES=window.EventSource;if(ES){var W=function(u,o){var es=new ES(u,o);` +
	`es.addEventListener("execai-close",function(){if(standalone()){shut()}});` +
	`es.addEventListener("message",function(e){try{var m=JSON.parse(e.data);` +
	`if(m.type==="turn_start"){busy[m.chat||""]=1}else if(m.type==="done"){delete busy[m.chat||""]}` +
	`else if(m.type==="chats"){busy={};(m.chats||[]).forEach(function(c){if(c.busy){busy[c.id]=1}})}}catch(x){}});` +
	`return es};W.prototype=ES.prototype;W.CONNECTING=0;W.OPEN=1;W.CLOSED=2;window.EventSource=W}` +
	`addEventListener("beforeunload",function(e){if(!closing&&Object.keys(busy).length){e.preventDefault();e.returnValue=""}});` +
	`addEventListener("pagehide",function(){try{navigator.sendBeacon("/bye")}catch(x){}})})();</script>
`

// prepare fills a page: the policy, the web app head, the nonce and CONFIG.
// window — the page is shown in an app window (windowScript).
func prepare(raw []byte, nonce string, cfg []byte, window bool) []byte {
	body := metaPolicy.ReplaceAllLiteral(raw,
		[]byte(`<meta http-equiv="Content-Security-Policy" content="`+pagePolicy("{{NONCE}}", false)+`">`))
	head := webAppHead
	if window {
		head += windowScript
	}
	if i := bytes.Index(body, []byte("</head>")); i >= 0 {
		body = append(append(append(make([]byte, 0, len(body)+len(head)), body[:i]...), head...), body[i:]...)
	}
	body = bytes.ReplaceAll(body, []byte("{{NONCE}}"), []byte(nonce))
	return bytes.ReplaceAll(body, []byte("{{CONFIG}}"), cfg)
}

const (
	// maxOpenURL — a link longer than this is not a link.
	maxOpenURL = 4096
	// openBurst links per openWindow: the panel opens one on a click.
	openBurst  = 10
	openWindow = 10 * time.Second
)

// externalURL — raw as a link the agent may hand to the system's opener:
// http, https or mailto, nothing that starts a program or reads a file. The
// opener gets the address as parsed and written out again, not as it came:
// no user name in it (a link that shows one host and goes to another), no
// white space or control characters; of a mailto only the address, the
// subject, the body and the copies — mail programs take more (attach=…),
// and a page has no business attaching files.
func externalURL(raw string) (string, bool) {
	if raw == "" || len(raw) > maxOpenURL {
		return "", false
	}
	for _, r := range raw {
		if r <= ' ' || r == 0x7f {
			return "", false
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.User != nil || u.Hostname() == "" {
			return "", false
		}
		u.Scheme = strings.ToLower(u.Scheme)
		return u.String(), true
	case "mailto":
		// The address is decoded, cleaned and written out anew like the
		// rest: a line break in it, spelled %0d%0a, is how a header is
		// slipped into the letter a mail program composes.
		to, err := url.PathUnescape(u.Opaque)
		if err != nil {
			return "", false
		}
		if to = noControls(to, false); to == "" {
			return "", false
		}
		keep := url.Values{}
		for k, v := range u.Query() {
			switch k = strings.ToLower(k); k {
			case "subject", "cc", "bcc":
				if len(v) > 0 {
					keep.Set(k, noControls(v[0], false))
				}
			case "body":
				if len(v) > 0 {
					keep.Set(k, noControls(v[0], true)) // a letter has lines
				}
			}
		}
		// Escaped as a path segment: a quote, a space and the like are
		// encoded, an address keeps its @. The comma between addresses stays.
		out := "mailto:" + strings.ReplaceAll(url.PathEscape(to), "%2C", ",")
		if len(keep) > 0 {
			out += "?" + keep.Encode()
		}
		return out, true
	}
	return "", false
}

// noControls — s without control characters; lines — keep line feeds.
func noControls(s string, lines bool) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' && lines {
			return r
		}
		if r < ' ' || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029 {
			return -1
		}
		return r
	}, s)
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	if s.opts.OpenExternal == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authed(r) || !s.originOK(r, true) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		http.Error(w, "want application/json", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2*maxOpenURL))
	var msg struct {
		URL string `json:"url"`
	}
	if err != nil || json.Unmarshal(body, &msg) != nil {
		http.Error(w, "bad url", http.StatusBadRequest)
		return
	}
	link, ok := externalURL(msg.URL)
	if !ok {
		http.Error(w, "bad url", http.StatusBadRequest)
		return
	}
	if !s.openAllowed(time.Now()) {
		http.Error(w, "too many links", http.StatusTooManyRequests)
		return
	}
	if err := s.opts.OpenExternal(link); err != nil {
		http.Error(w, "could not open", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// openAllowed — no more than openBurst links per openWindow.
func (s *Server) openAllowed(now time.Time) bool {
	s.openMu.Lock()
	defer s.openMu.Unlock()
	keep := s.openTimes[:0]
	for _, t := range s.openTimes {
		if now.Sub(t) < openWindow {
			keep = append(keep, t)
		}
	}
	s.openTimes = keep
	if len(s.openTimes) >= openBurst {
		return false
	}
	s.openTimes = append(s.openTimes, now)
	return true
}

// handleStop — POST /stop: `execai ui --stop` asks the panel of a folder to
// end as Ctrl+C in its terminal would. The asker proves it may by the
// control secret of this run — 32 random bytes kept in the state file next
// to the one-time code, which only the person's own programs can read. A
// page has no business here, so a request that names an Origin is refused
// even with the secret, and the secret is taken from a header only: nothing
// a link or a form can carry.
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if s.opts.OnStop == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.controlled(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	// The answer first: the server's shutdown waits for this handler.
	s.stopOnce.Do(func() { go s.opts.OnStop() })
}

// handleBye — POST /bye: the window's page is being closed or reloaded
// (sendBeacon on pagehide). With no page connected afterwards, that is a
// closed window (PageLeft); a page that only vanished has crashed.
func (s *Server) handleBye(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authed(r) || !s.originOK(r, true) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.mu.Lock()
	s.lastBye = time.Now()
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
