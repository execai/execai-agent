package uiserve

import (
	"bytes"
	"embed"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// assets — the browser build of the editor panel, copied from vscode-vbai by
// scripts/sync-panel.sh (panel-browser.<lang>.html), and stub.html, a minimal
// page of the same contract used until (or unless) the build is there.
//
//go:embed assets/*.html
var assets embed.FS

// pageLangs — the languages the panel speaks (its STRINGS); others get "en".
var pageLangs = map[string]bool{"en": true, "ru": true}

// pageConfig — {{CONFIG}}: what the page needs from the agent. The account
// token is never part of it.
type pageConfig struct {
	Lang    string `json:"lang"`
	Cwd     string `json:"cwd"`
	Version string `json:"version"`
	Upload  string `json:"upload"`
}

// page — the HTML for a language: the panel build for it, the English build,
// or the stub.
func page(lang string) []byte {
	for _, name := range []string{"assets/panel-browser." + lang + ".html", "assets/panel-browser.en.html", "assets/stub.html"} {
		if b, err := assets.ReadFile(name); err == nil {
			return b
		}
	}
	return nil
}

func (s *Server) servePage(w http.ResponseWriter, r *http.Request) {
	lang := s.lang(r)
	nonce, err := randomHex(16)
	if err != nil {
		http.Error(w, "no randomness", http.StatusInternalServerError)
		return
	}
	cfg, _ := json.Marshal(pageConfig{Lang: lang, Cwd: s.opts.Cwd, Version: s.opts.Version, Upload: "/upload"})
	// json.Marshal already writes <, > and & as \u escapes, so the value is
	// safe inside a <script>; a quote is escaped too, for an attribute.
	cfg = bytes.ReplaceAll(cfg, []byte("'"), []byte(`'`))
	body := page(lang)
	body = bytes.ReplaceAll(body, []byte("{{NONCE}}"), []byte(nonce))
	body = bytes.ReplaceAll(body, []byte("{{CONFIG}}"), cfg)

	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	// The webview's policy plus connect-src 'self' for /events, /in and
	// /upload; nothing may frame the page or post a form elsewhere.
	h.Set("Content-Security-Policy", "default-src 'none'; img-src data: blob:; style-src 'unsafe-inline'; "+
		"script-src 'nonce-"+nonce+"'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	_, _ = w.Write(body)
}

// lang — the page language: the configured locale, then EXECAI_LOCALE, then
// the browser's Accept-Language; a language the panel does not speak is
// English.
func (s *Server) lang(r *http.Request) string {
	for _, l := range []string{s.opts.Locale, s.envLocale} {
		if l = baseLang(l); l != "" {
			if pageLangs[l] {
				return l
			}
			return "en"
		}
	}
	if l := acceptLang(r.Header.Get("Accept-Language")); l != "" {
		return l
	}
	return "en"
}

// baseLang reduces "ru_RU.UTF-8", "en-US", "zh-Hans" to "ru", "en", "zh".
func baseLang(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, "._-@"); i >= 0 {
		s = s[:i]
	}
	if s == "c" || s == "posix" {
		return ""
	}
	return s
}

// acceptLang — the panel language the browser prefers most, "" if it names
// none of them.
func acceptLang(header string) string {
	type pref struct {
		lang string
		q    float64
	}
	var prefs []pref
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		l := baseLang(fields[0])
		if !pageLangs[l] {
			continue
		}
		q := 1.0
		for _, f := range fields[1:] {
			if v, ok := strings.CutPrefix(strings.TrimSpace(f), "q="); ok {
				if x, err := strconv.ParseFloat(v, 64); err == nil {
					q = x
				}
			}
		}
		if q > 0 {
			prefs = append(prefs, pref{l, q})
		}
	}
	if len(prefs) == 0 {
		return ""
	}
	sort.SliceStable(prefs, func(a, b int) bool { return prefs[a].q > prefs[b].q })
	return prefs[0].lang
}
