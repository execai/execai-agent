// Package uiserve serves the agent's chat panel to a browser on 127.0.0.1
// (execai ui): the same page as the VS Code panel, over another transport.
// The agent side is the ordinary ide server (internal/ide) running in this
// process: every line it writes goes to the open tabs as one Server-Sent
// Event, and every POST /in becomes one line of its input.
//
// Routes:
//
//	GET  /?t=<token>  the one-time token → session cookie and a page that goes on to /
//	GET  /            the page (cookie); without the cookie — the sign-in page
//	GET  /events      SSE: the last ready/state/inbox/chats/agents first, then live events
//	POST /in          one ide In message as JSON → 204
//	POST /upload      multipart file → {path, name}, saved in <cwd>/.execai/uploads/
//	POST /login       {"code": the one-time code} → session cookie, 204 (login.go)
//	POST /code        the control secret in X-Execai-Control → a one-time code (execai ui --code)
//	POST /bye         the window's page says it is being closed (webapp.go)
//	POST /open        {"url": a link} → the agent opens it in the person's browser (app window only)
//	POST /stop        the control secret in X-Execai-Control → the panel ends (execai ui --stop; webapp.go)
//	GET  /manifest.webmanifest, /icon.png   what makes the page an installable web app (webapp.go)
//
// Security: the server listens on loopback only; the address printed in the
// terminal carries a random 32-byte token that works once and is swapped for
// an HttpOnly SameSite=Strict cookie (another random value), so the token
// left in the terminal or the browser history opens nothing. Every request
// must name this server in Host (no DNS rebinding); POST and SSE requests
// must come from this origin; the page runs under a nonce CSP that lets it
// talk to this origin only. The page never receives the account token.
//
// The address without the token is stable (a port of the project's own),
// so a shortcut, a web app in the Dock or a second run can come back to it.
// What answers there without the cookie is the sign-in page, the manifest
// and the icon: nothing of the project. The second way in is a one-time
// code that exists only after the person asks for it in a terminal
// (execai ui --code) — login.go says what guards it.
package uiserve

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/ide"
)

// Options — what the server needs to know.
type Options struct {
	// Port — the loopback port the server is reached on (Host and Origin
	// are checked against it). For Run: 0 picks a free one.
	Port int
	// Cwd — the project directory (absolute): shown to the page, uploads go
	// under it.
	Cwd     string
	Version string
	// Locale — the configured language (config.json); empty = not set.
	Locale string
	// TryPorts — for Run with Port 0: the ports to try in order before a
	// free one picked by the system. A Port the person asked for is the only
	// one tried.
	TryPorts []int
	// OpenExternal — set when the page is shown in an app window: links of
	// the panel are opened with it (the person's own browser) instead of
	// inside that window's separate profile, and a window that comes up
	// closes the older ones of the same panel (webapp.go, windowScript).
	// Nil: a tab — the browser opens links and keeps its tabs.
	OpenExternal func(url string) error
	// CodeTTL — how long a one-time code waits to be typed. 0 — 5 minutes.
	CodeTTL time.Duration
	// OnStop — called once when a request that knows the control secret
	// (Server.Control) asks the panel to end. Nil: there is no such request.
	OnStop func()
	// OnSignIn — called each time a browser comes in with the one-time key
	// (before it is answered): the panel removes what carried the key.
	OnSignIn func()
	// OnIntruder — called after the server has decided that the one-time
	// key reached somebody else (Intrusion), has dropped every session,
	// issued a new key, stopped the running turns and refused the waiting
	// permission requests: the panel says so and shows its page anew.
	OnIntruder func(Intrusion)
	// ConnectWait — how long after the key was used a page may take to
	// open its event stream before the user of the key is taken for
	// something other than a panel. 0 — 15 s.
	ConnectWait time.Duration
}

// Intrusion — why the server thinks the one-time key was read by somebody
// it was not meant for.
type Intrusion int

const (
	// KeyUsedTwice — a browser without a session came with a key another
	// one had used — a moment or a day before. The key is shown to one
	// browser; two comers mean the link leaked (where it is passed to a
	// browser as an argument, every program on the computer can read it
	// for a moment), and the server cannot tell which of the two is the
	// person. The person's own browser is told apart by the session it
	// carries: opening the link again there is no alarm.
	KeyUsedTwice Intrusion = iota + 1
	// KeyUsedNoPage — the key was used, and no page connected afterwards:
	// its user was not a panel in a browser.
	KeyUsedNoPage
)

// IDE runs the agent's ide server over the given transport until ctx ends
// (in execai: ide.Run with Options.In/Out).
type IDE func(ctx context.Context, in io.Reader, out io.Writer) error

const (
	// maxInBytes keeps one In line under the ide reader's 1 MiB line limit:
	// a longer line would end its scanner, and with it the agent.
	maxInBytes = 1<<20 - 64
	// inTimeout — how long POST /in waits for the agent to take the line.
	inTimeout = 10 * time.Second
	// writeWait — the deadline of one write to an event stream.
	writeWait = 15 * time.Second
	// cookieMaxAge — the session cookie outlives a browser restart; it is
	// worthless once this process ends (a new run makes a new secret).
	cookieMaxAge = 30 * 24 * 3600
	// maxSessions — browsers signed in at once (the link, then codes); the
	// oldest gives way.
	maxSessions = 16
)

// heartbeat — an SSE comment this often keeps proxies and the browser from
// timing the stream out and finds dead tabs. A variable for tests.
var heartbeat = 20 * time.Second

// internalTypes — In types the agent uses for itself; the page may not send
// them ("eof" would stop the agent, "web_task" fake a task from the site).
var internalTypes = map[string]bool{"eof": true, "web_task": true}

// Server — the HTTP side of execai ui.
type Server struct {
	opts      Options
	hub       *hub
	envLocale string

	in   io.Writer // the ide server's input
	inMu sync.Mutex

	mu        sync.Mutex
	token     string // one-time, from the terminal
	tokenUsed bool
	keyGen    int      // grows with every new key
	joined    bool     // a page opened an event stream since the key was issued
	stopped   bool     // Close was called
	sessions  []string // the cookie values: one per browser that signed in

	// The one-time sign-in code (login.go): none until one is asked for.
	code      string
	codeFails int // wrong tries at the current code
	codeUntil time.Time

	lastJoin time.Time // a page opened an event stream
	lastBye  time.Time // a page said it is being closed

	openMu    sync.Mutex
	openTimes []time.Time // POST /open, the last ones

	control  string // the secret of POST /stop and POST /code
	stopOnce sync.Once
}

// New prepares a server for opts.Port; in is where POST /in lines go.
func New(opts Options, in io.Writer) (*Server, error) {
	tok, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	control, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	return &Server{opts: opts, hub: newHub(), in: in, token: tok, control: control,
		envLocale: os.Getenv("EXECAI_LOCALE")}, nil
}

// Out — the writer the ide server writes its protocol to.
func (s *Server) Out() io.Writer { return s.hub }

// URL — the address to open: it carries the one-time key. The key is
// replaced after an alarm (OnIntruder), so the address is asked for anew
// each time the page is shown.
func (s *Server) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("http://127.0.0.1:%d/?t=%s", s.opts.Port, s.token)
}

// StableURL — the address without the token: the same on every run that
// gets the same port. It opens the page for a browser that holds the
// session cookie and the sign-in page for any other.
func (s *Server) StableURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/", s.opts.Port)
}

// Port — the loopback port the server is reached on.
func (s *Server) Port() int { return s.opts.Port }

// Control — the secret the panel's own controls ask for (POST /stop, POST
// /code). It goes into the state file, which the person alone can read, and
// never to a page.
func (s *Server) Control() string { return s.control }

// PageLeft — no page is connected, and the last one to go said goodbye
// (POST /bye): it was closed, not lost. A page that crashed says nothing.
func (s *Server) PageLeft() bool {
	if s.hub.count() != 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastBye.After(s.lastJoin)
}

// Viewers — how many pages hold an event stream now.
func (s *Server) Viewers() int { return s.hub.count() }

// TokenUsed reports whether a browser has come in with the one-time link.
func (s *Server) TokenUsed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokenUsed
}

// Close ends every event stream.
func (s *Server) Close() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.hub.close()
}

// Run listens on 127.0.0.1, runs ide on a pipe that stays open while the
// process lives (closing a tab is not the end of the agent), calls started
// with the server (its addresses) and returns when ctx is done (Ctrl+C, the
// app window closed) or ide stops on its own.
func Run(ctx context.Context, opts Options, ide IDE, started func(s *Server)) error {
	ln, err := listen(opts)
	if err != nil {
		return err
	}
	opts.Port = ln.Addr().(*net.TCPAddr).Port
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	s, err := New(opts, pw)
	if err != nil {
		_ = ln.Close()
		return err
	}
	hs := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	go func() { _ = hs.Serve(ln) }()

	errc := make(chan error, 1)
	go func() { errc <- ide(ctx, pr, s.Out()) }()
	if started != nil {
		started(s)
	}
	err = <-errc
	s.Close()
	sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = hs.Shutdown(sctx)
	return err
}

// listen takes the loopback port: the one asked for, or the first free one
// of opts.TryPorts, or any free one.
func listen(opts Options) (net.Listener, error) {
	if opts.Port == 0 {
		for _, p := range opts.TryPorts {
			if p <= 0 {
				continue
			}
			if ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p))); err == nil {
				return ln, nil
			}
		}
	}
	return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(opts.Port)))
}

// Handler — all routes behind the Host check.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/in", s.handleIn)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/open", s.handleOpen)
	mux.HandleFunc("/stop", s.handleStop)
	mux.HandleFunc("/code", s.handleCode)
	mux.HandleFunc("/bye", s.handleBye)
	mux.HandleFunc("/manifest.webmanifest", s.handleManifest)
	mux.HandleFunc("/icon.png", s.handleIcon)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("X-Frame-Options", "DENY")
		if !s.hostOK(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// hostOK — the request names this server: a page on another name resolving
// to 127.0.0.1 (DNS rebinding) is refused.
func (s *Server) hostOK(host string) bool {
	host = strings.ToLower(host)
	p := strconv.Itoa(s.opts.Port)
	return host == "127.0.0.1:"+p || host == "localhost:"+p
}

// originOK — the request comes from this origin. Browsers send Origin with
// every POST, so there it is required; an EventSource to its own origin may
// come without one, then Sec-Fetch-Site (when sent) must say same-origin.
func (s *Server) originOK(r *http.Request, required bool) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		if required {
			return false
		}
		sfs := r.Header.Get("Sec-Fetch-Site")
		return sfs == "" || sfs == "same-origin"
	}
	return strings.EqualFold(o, "http://"+r.Host)
}

func (s *Server) cookieName() string { return "execai_ui_" + strconv.Itoa(s.opts.Port) }

// authed — the request carries a session cookie of this run.
func (s *Server) authed(r *http.Request) bool {
	c, err := r.Cookie(s.cookieName())
	if err != nil || c.Value == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ok := 0
	for _, sess := range s.sessions {
		ok |= subtle.ConstantTimeCompare([]byte(c.Value), []byte(sess))
	}
	return ok == 1
}

// newSessionLocked — a session secret for one more browser; under s.mu.
func (s *Server) newSessionLocked() string {
	sess, err := randomHex(32)
	if err != nil {
		return ""
	}
	if len(s.sessions) >= maxSessions {
		s.sessions = s.sessions[1:]
	}
	s.sessions = append(s.sessions, sess)
	return sess
}

// setSession gives the browser its cookie.
func (s *Server) setSession(w http.ResponseWriter, sess string) {
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: sess, Path: "/",
		MaxAge: cookieMaxAge, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

// consume trades the one-time token for the session secret; a wrong or a
// second try gets "".
func (s *Server) consume(tok string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokenUsed || subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) != 1 {
		return ""
	}
	sess := s.newSessionLocked()
	if sess == "" {
		return ""
	}
	s.tokenUsed = true
	// The one who used the key is expected to be a panel page: it opens
	// its event stream within moments.
	gen := s.keyGen
	time.AfterFunc(orDefault(s.opts.ConnectWait, 15*time.Second), func() {
		s.mu.Lock()
		quiet := s.keyGen != gen || s.joined || s.stopped
		s.mu.Unlock()
		if !quiet {
			s.intruder(KeyUsedNoPage)
		}
	})
	return sess
}

func orDefault(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

// usedAgain — tok is the current key, already used: a second comer. There
// is no "too late to matter": a link somebody kept is in other hands an hour
// later as much as in the first second. (The caller has looked for the
// session first — the person's own browser is not a second comer.)
func (s *Server) usedAgain(tok string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokenUsed && subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) == 1
}

// intruder — the key reached somebody else. Nobody can be told from
// anybody by now, so everybody is out: no session stays, open event streams
// end, and the next sign-in needs a key nobody has seen. Whoever held the
// other session may have sent the agent a message already, so what runs is
// stopped and what waits for a yes is told no (halt). Then the panel is
// told.
func (s *Server) intruder(why Intrusion) {
	s.mu.Lock()
	s.sessions = nil
	if tok, err := randomHex(32); err == nil {
		s.token, s.tokenUsed = tok, false
	} else {
		s.tokenUsed = true // no new key: the old one stays used up
	}
	s.keyGen++
	s.joined = false
	s.mu.Unlock()
	s.hub.dropAll()
	s.halt()
	if s.opts.OnIntruder != nil {
		s.opts.OnIntruder(why)
	}
}

// halt stops every running turn and refuses every permission request that
// waits, with the agent's own messages — the ones the panel sends when the
// person presses "stop" or "deny". Which turns run and which requests wait
// is read off the agent's events (hub). Refusals go first: a request must
// not be left for a late "yes" while its turn is being stopped.
func (s *Server) halt() {
	chats, asks := s.hub.pending()
	for _, id := range asks {
		line, _ := json.Marshal(ide.In{Type: "answer", ID: id, Value: "deny"})
		_ = s.send(append(line, '\n'))
	}
	for _, chat := range chats {
		line, _ := json.Marshal(ide.In{Type: "stop", Chat: chat})
		_ = s.send(append(line, '\n'))
	}
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	// GET only: a link preview probing with HEAD must not burn the token.
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if tok := r.URL.Query().Get("t"); tok != "" {
		sess := s.consume(tok)
		switch {
		case sess != "":
			if s.opts.OnSignIn != nil {
				s.opts.OnSignIn()
			}
			s.setSession(w, sess)
		case s.authed(r):
			// The browser that holds the session opened the link again.
		case s.usedAgain(tok):
			s.intruder(KeyUsedTwice)
			s.serveLogin(w, r)
			return
		default:
			http.Error(w, i18n.T("ui.http.linkUsed"), http.StatusForbidden)
			return
		}
		// The address bar loses the token either way.
		s.serveOnward(w)
		return
	}
	if !s.authed(r) {
		// No session: the sign-in page, which tells nothing of the project.
		s.serveLogin(w, r)
		return
	}
	s.servePage(w, r)
}

// serveOnward answers the request that brought the key: a page whose only
// content takes the browser to "/".
//
// Not a redirect. The session cookie is SameSite=Strict, and a browser does
// not send such a cookie on a navigation that another site started — all
// the way through its redirects. The app window comes from a file of the
// person's (uiwindow.WriteJump), which is "another site" to the browser: a
// 303 to "/" arrived there without the cookie just set, and the window
// showed the sign-in page. A navigation this page starts itself is the
// panel's own, and carries the cookie. replace keeps the address with the
// key out of the window's history.
func (s *Server) serveOnward(w http.ResponseWriter) {
	nonce, err := randomHex(16)
	if err != nil {
		http.Error(w, "no randomness", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+"'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	_, _ = io.WriteString(w, "<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>ExecAI</title>\n"+
		"<script nonce=\""+nonce+"\">location.replace(\"/\");</script>\n"+
		"<noscript><meta http-equiv=\"refresh\" content=\"0;url=/\"></noscript>\n"+
		"</head><body></body></html>\n")
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authed(r) || !s.originOK(r, false) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	c, replay, err := s.hub.subscribe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer s.hub.unsubscribe(c)
	s.mu.Lock()
	s.joined, s.lastJoin = true, time.Now()
	s.mu.Unlock()
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	var buf bytes.Buffer
	send := func() bool {
		_ = rc.SetWriteDeadline(time.Now().Add(writeWait))
		_, err := w.Write(buf.Bytes())
		buf.Reset()
		return err == nil && rc.Flush() == nil
	}
	buf.WriteString("retry: 1500\n\n")
	for _, l := range replay {
		writeEvent(&buf, l)
	}
	if !send() {
		return
	}
	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case l, ok := <-c.ch:
			if !ok {
				// Dropped as too slow or signed out, or the server is
				// stopping — then an app window is told to close.
				s.sayClose(&buf)
				_ = send()
				return
			}
			writeEvent(&buf, l)
			// Take what else is already waiting: one flush per burst.
			for more := true; more && buf.Len() < 64<<10; {
				select {
				case l, ok := <-c.ch:
					if !ok {
						s.sayClose(&buf)
						_ = send()
						return
					}
					writeEvent(&buf, l)
				default:
					more = false
				}
			}
			if !send() {
				return
			}
		case <-tick.C:
			buf.WriteString(": ping\n\n")
			if !send() {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

// sayClose — the last words to a page of an app window when the panel
// stops: a named event (the panel's own handler takes unnamed ones only)
// on which the window's script closes the window (webapp.go). This is how a
// window in a browser the panel did not start — one that took the window
// over — goes with the panel too. A tab is told nothing: it is the
// person's.
func (s *Server) sayClose(buf *bytes.Buffer) {
	s.mu.Lock()
	stopped := s.stopped
	s.mu.Unlock()
	if stopped && s.opts.OpenExternal != nil {
		buf.WriteString("event: execai-close\ndata: 1\n\n")
	}
}

// writeEvent — one Out line as one SSE event. JSON from the encoder has no
// raw newlines, so a single data field carries it.
func writeEvent(buf *bytes.Buffer, line []byte) {
	buf.WriteString("data: ")
	buf.Write(line)
	buf.WriteString("\n\n")
}

var errAgentBusy = errors.New("the agent did not take the message in time")

func (s *Server) handleIn(w http.ResponseWriter, r *http.Request) {
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
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxInBytes))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "message too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	// Read the way the agent will read it — into its own structure: there
	// "Type" is "type" too, and of two the last one counts. A filter that
	// looked at the exact key alone let {"type":"x","Type":"eof"} through.
	var msg ide.In
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "want one JSON object", http.StatusBadRequest)
		return
	}
	if msg.Type == "" || internalTypes[msg.Type] {
		http.Error(w, "bad type", http.StatusBadRequest)
		return
	}
	// One message — one line: pretty-printed JSON would be split by the
	// agent's line reader.
	var line bytes.Buffer
	if err := json.Compact(&line, body); err != nil {
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return
	}
	line.WriteByte('\n')
	if err := s.send(line.Bytes()); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if msg.Type == "answer" {
		s.hub.answered(msg.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}

// send writes one line to the agent. Lines never interleave; a request does
// not hang forever on an agent that is not reading — the line still goes in
// order once it does.
func (s *Server) send(line []byte) error {
	done := make(chan error, 1)
	go func() {
		s.inMu.Lock()
		defer s.inMu.Unlock()
		_, err := s.in.Write(line)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(inTimeout):
		return errAgentBusy
	}
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
