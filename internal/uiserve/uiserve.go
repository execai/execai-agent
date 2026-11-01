// Package uiserve serves the agent's chat panel to a browser on 127.0.0.1
// (execai ui): the same page as the VS Code panel, over another transport.
// The agent side is the ordinary ide server (internal/ide) running in this
// process: every line it writes goes to the open tabs as one Server-Sent
// Event, and every POST /in becomes one line of its input.
//
// Routes:
//
//	GET  /?t=<token>  the one-time token from the terminal → session cookie, 303 to /
//	GET  /            the page (cookie)
//	GET  /events      SSE: the last ready/state/inbox/chats/agents first, then live events
//	POST /in          one ide In message as JSON → 204
//	POST /upload      multipart file → {path, name}, saved in <cwd>/.execai/uploads/
//
// Security: the server listens on loopback only; the address printed in the
// terminal carries a random 32-byte token that works once and is swapped for
// an HttpOnly SameSite=Strict cookie (another random value), so the token
// left in the terminal or the browser history opens nothing. Every request
// must name this server in Host (no DNS rebinding); POST and SSE requests
// must come from this origin; the page runs under a nonce CSP that lets it
// talk to this origin only. The page never receives the account token.
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
}

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
	session   string // the cookie value, once the token is used
}

// New prepares a server for opts.Port; in is where POST /in lines go.
func New(opts Options, in io.Writer) (*Server, error) {
	tok, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	return &Server{opts: opts, hub: newHub(), in: in, token: tok,
		envLocale: os.Getenv("EXECAI_LOCALE")}, nil
}

// Out — the writer the ide server writes its protocol to.
func (s *Server) Out() io.Writer { return s.hub }

// URL — the address to open: it carries the one-time token.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/?t=%s", s.opts.Port, s.token)
}

// Close ends every event stream.
func (s *Server) Close() { s.hub.close() }

// Run listens on 127.0.0.1, runs ide on a pipe that stays open while the
// process lives (closing a tab is not the end of the agent), calls started
// with the address to open and returns when ctx is done (Ctrl+C) or ide
// stops on its own.
func Run(ctx context.Context, opts Options, ide IDE, started func(url string)) error {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(opts.Port)))
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
		started(s.URL())
	}
	err = <-errc
	s.Close()
	sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = hs.Shutdown(sctx)
	return err
}

// Handler — all routes behind the Host check.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/in", s.handleIn)
	mux.HandleFunc("/upload", s.handleUpload)
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

// authed — the request carries this run's session cookie.
func (s *Server) authed(r *http.Request) bool {
	c, err := r.Cookie(s.cookieName())
	if err != nil || c.Value == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session != "" && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.session)) == 1
}

// consume trades the one-time token for the session secret; a wrong or a
// second try gets "".
func (s *Server) consume(tok string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tokenUsed || subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) != 1 {
		return ""
	}
	sess, err := randomHex(32)
	if err != nil {
		return ""
	}
	s.tokenUsed, s.session = true, sess
	return sess
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
		if sess := s.consume(tok); sess != "" {
			http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: sess, Path: "/",
				MaxAge: cookieMaxAge, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		} else if !s.authed(r) {
			http.Error(w, i18n.T("ui.http.linkUsed"), http.StatusForbidden)
			return
		}
		// The address bar loses the token either way.
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !s.authed(r) {
		http.Error(w, i18n.T("ui.http.noSession"), http.StatusForbidden)
		return
	}
	s.servePage(w, r)
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
				return // dropped as too slow, or the server is stopping
			}
			writeEvent(&buf, l)
			// Take what else is already waiting: one flush per burst.
			for more := true; more && buf.Len() < 64<<10; {
				select {
				case l, ok := <-c.ch:
					if !ok {
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
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "want one JSON object", http.StatusBadRequest)
		return
	}
	var typ string
	if err := json.Unmarshal(msg["type"], &typ); err != nil || typ == "" || internalTypes[typ] {
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
