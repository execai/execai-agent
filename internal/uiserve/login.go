package uiserve

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"html"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// The second way in: a one-time code.
//
// A window that cannot be handed the link — a web app the person added to
// the Dock from Safari, which opens the stable address in a cookie store of
// its own; another browser — gets the sign-in page and asks for a code.
//
// There is no code until the person asks for one: `execai ui --code` in the
// project folder asks the running panel (POST /code, with the control
// secret of the run from the state file — 0600 in the 0700 config
// directory, like credentials.json), and the panel makes a code that is
// good for a few minutes and for one sign-in. Until then, and after, the
// way is shut: there is nothing to guess, and nothing a stranger's requests
// could use up.
//
// What guards the open minutes:
//
//   - without the code nothing is served but the sign-in page, the manifest
//     and the icon — none of them tells anything of the project;
//   - the code is 8 symbols of a 32-letter alphabet (40 bits); maxCodeFails
//     wrong tries burn it, and the person asks again — a guesser has at most
//     that many tries per code the person asked for. A wrong try and a try
//     when no code waits get the same answer, so the moment a code is issued
//     does not show from outside. (A program of this computer that keeps
//     trying all the time still burns every code: that costs the person the
//     sign-in by code, never lets the program in.)
//   - a try counts only when it comes from this page: Host and Origin are
//     checked first and the body must be JSON, so another site can neither
//     guess nor burn the code (a form cannot send JSON, and a cross-origin
//     fetch carries its own Origin);
//   - the session it gives is the same HttpOnly SameSite=Strict cookie.
//
// What it cannot do is tell the person which page they type the code into:
// a program of another account on this computer could listen on another
// port and show a look-alike. It can no longer make a code appear, though:
// the code exists only after the person's own command, is printed together
// with the address it is for, and goes stale in minutes.

const (
	// codeAlphabet — no I, O, 0, 1: the code is read off a terminal.
	codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	codeLen      = 8
	// maxCodeFails — wrong tries that burn a code.
	maxCodeFails = 5
	// codeTTL — how long a code the person asked for waits to be typed.
	codeTTL = 5 * time.Minute
)

// newCode — 8 random symbols, "ABCD-EFGH".
func newCode() (string, error) {
	b := make([]byte, codeLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)] // 256 is a multiple of 32: no bias
	}
	return string(b[:codeLen/2]) + "-" + string(b[codeLen/2:]), nil
}

// normCode — the code as typed: any case, spaces or dashes anywhere.
func normCode(s string) string {
	s = strings.ToUpper(s)
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' {
			return -1
		}
		return r
	}, s)
}

// IssueCode opens the way in by a code: a new code, good for the returned
// time and for one sign-in. A code issued before is replaced.
func (s *Server) IssueCode() (string, time.Duration, error) {
	code, err := newCode()
	if err != nil {
		return "", 0, err
	}
	ttl := orDefault(s.opts.CodeTTL, codeTTL)
	s.mu.Lock()
	s.code, s.codeFails, s.codeUntil = code, 0, time.Now().Add(ttl)
	s.mu.Unlock()
	return code, ttl, nil
}

type redeemed int

const (
	codeWrong redeemed = iota
	codeOK
	codeClosed // no code is waiting: none was asked for, or it is used, burned or stale
)

// redeem trades the code for a session. Every call is one try.
func (s *Server) redeem(typed string) (string, redeemed) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.code != "" && time.Now().After(s.codeUntil) {
		s.code = ""
	}
	if s.code == "" {
		return "", codeClosed
	}
	want := normCode(s.code)
	got := normCode(typed)
	if len(got) != len(want) || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		if s.codeFails++; s.codeFails >= maxCodeFails {
			s.code = ""
		}
		return "", codeWrong
	}
	sess := s.newSessionLocked()
	if sess == "" {
		return "", codeWrong
	}
	s.code = ""
	return sess, codeOK
}

// controlled — the request knows the control secret of this run and does
// not come from a page (a page has no business with the panel's controls,
// with or without the secret). The secret is read from a header only:
// nothing a link or a form can carry.
func (s *Server) controlled(r *http.Request) bool {
	got := r.Header.Get("X-Execai-Control")
	return r.Header.Get("Origin") == "" && s.control != "" && len(got) == len(s.control) &&
		subtle.ConstantTimeCompare([]byte(got), []byte(s.control)) == 1
}

// handleCode — POST /code: `execai ui --code` asks for a code.
func (s *Server) handleCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.controlled(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	code, ttl, err := s.IssueCode()
	if err != nil {
		http.Error(w, "no randomness", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "ttl": int(ttl.Seconds())})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Where the request comes from is checked before the code is looked at:
	// a request from elsewhere is not a try.
	if !s.originOK(r, true) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		http.Error(w, "want application/json", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var msg struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(body, &msg) != nil {
		http.Error(w, "want one JSON object", http.StatusBadRequest)
		return
	}
	sess, res := s.redeem(msg.Code)
	if res == codeOK {
		s.setSession(w, sess)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// One answer for "the code is wrong" and "no code is waiting": told
	// apart, they would show a program that keeps asking the very moment
	// the person has asked for a code.
	http.Error(w, i18n.T("ui.http.login.wrong"), http.StatusForbidden)
}

// loginPage — the sign-in page. {{…}} are filled by serveLogin with escaped
// text; the page asks nothing of the server but POST /login.
//
// Its theme is the window's: dark until the person picks one, then what was
// picked — the window keeps the pick in this browser under execai:ui:theme
// ("light", "dark", or "system" for "as the system"), and the script in the
// head reads it before anything is drawn. Without the script, or with the
// storage closed, the page is dark.
const loginPage = `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ExecAI</title>
{{HEAD}}<script nonce="{{NONCE}}">
(function () {
  var t = 'dark';
  try { var b = localStorage.getItem('execai:ui:theme'); if (b === 'light' || b === 'system') t = b; } catch (e) {}
  document.documentElement.setAttribute('data-theme', t);
  var light = t === 'light' || (t === 'system' && window.matchMedia && matchMedia('(prefers-color-scheme: light)').matches);
  var m = document.querySelector('meta[name="theme-color"]');
  if (m && light) m.setAttribute('content', '{{LIGHT}}');
})();
</script>
<style>
  :root { color-scheme: dark; --bg: {{DARK}}; --fg: #d7dae0; --muted: #9da3ae; --line: #3d4350; --accent: #0078d4; --err: #f48771; }
  :root[data-theme="light"] { color-scheme: light; --bg: {{LIGHT}}; --fg: #1f1f1f; --muted: #616161; --line: #cecece; --accent: #005fb8; --err: #c72e2e; }
  @media (prefers-color-scheme: light) { :root[data-theme="system"] { color-scheme: light; --bg: {{LIGHT}}; --fg: #1f1f1f; --muted: #616161; --line: #cecece; --accent: #005fb8; --err: #c72e2e; } }
  html, body { height: 100%; }
  body { margin: 0; background: var(--bg); color: var(--fg); font: 14px/1.5 system-ui, -apple-system, "Segoe UI", sans-serif;
         display: flex; align-items: center; justify-content: center; }
  main { width: min(26em, 100% - 32px); }
  h1 { font-size: 18px; margin: 0 0 12px; display: flex; align-items: center; gap: 10px; }
  h1 img { width: 28px; height: 28px; }
  p { margin: 0 0 12px; color: var(--muted); }
  code { font: 13px ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; color: var(--fg); }
  form { display: flex; gap: 8px; margin: 16px 0 8px; }
  input { flex: 1; min-width: 0; font: 16px ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; letter-spacing: .08em; text-transform: uppercase;
          padding: 7px 10px; background: transparent; color: var(--fg); border: 1px solid var(--line); border-radius: 6px; }
  input:focus { outline: 2px solid var(--accent); outline-offset: -1px; }
  button { font: inherit; padding: 7px 14px; background: var(--accent); color: #fff; border: 0; border-radius: 6px; cursor: pointer; }
  #err { min-height: 1.5em; color: var(--err); }
</style>
</head>
<body>
<main>
  <h1><img src="/icon.png" alt="">ExecAI</h1>
  <p>{{CODE}}</p>
  <form id="f" autocomplete="off">
    <input id="code" name="code" autocapitalize="characters" spellcheck="false" maxlength="12" placeholder="XXXX-XXXX" aria-label="{{LABEL}}" autofocus>
    <button type="submit">{{BUTTON}}</button>
  </form>
  <div id="err" role="alert"></div>
  <p>{{LINK}}</p>
</main>
<script nonce="{{NONCE}}">
(function () {
  var MSG = {{MSG}};
  var form = document.getElementById('f'), code = document.getElementById('code'), err = document.getElementById('err');
  form.addEventListener('submit', function (ev) {
    ev.preventDefault();
    err.textContent = '';
    fetch('/login', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
                      body: JSON.stringify({ code: code.value }) })
      .then(function (r) {
        if (r.status === 204) { location.replace('/'); return; }
        err.textContent = MSG.wrong;
        code.select();
      }, function () { err.textContent = MSG.down; });
  });
})();
</script>
</body>
</html>
`

// loginLight — the background of the sign-in page in the light theme.
const loginLight = "#f8f8f8"

// serveLogin — the answer to a browser without the session: 403 with the
// sign-in page. Nothing of the project, the version or the secrets is in it.
func (s *Server) serveLogin(w http.ResponseWriter, r *http.Request) {
	nonce, err := randomHex(16)
	if err != nil {
		http.Error(w, i18n.T("ui.http.noSession"), http.StatusForbidden)
		return
	}
	msg, _ := json.Marshal(map[string]string{
		"wrong": i18n.T("ui.http.login.wrong"),
		"down":  i18n.T("ui.http.login.down"),
	})
	code := html.EscapeString(i18n.T("ui.http.login.code"))
	code = strings.ReplaceAll(code, "execai ui --code", "<code>execai ui --code</code>")
	page := strings.NewReplacer(
		"{{HEAD}}", webAppHead,
		"{{DARK}}", frameDark,
		"{{LIGHT}}", loginLight,
		"{{LINK}}", html.EscapeString(i18n.T("ui.http.login.link")),
		"{{CODE}}", code,
		"{{LABEL}}", html.EscapeString(i18n.T("ui.http.login.label")),
		"{{BUTTON}}", html.EscapeString(i18n.T("ui.http.login.button")),
		"{{MSG}}", string(msg),
		"{{NONCE}}", nonce,
	).Replace(loginPage)
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; "+
		"script-src 'nonce-"+nonce+"'; connect-src 'self'; manifest-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.WriteHeader(http.StatusForbidden)
	_, _ = io.WriteString(w, page)
}
