package uiserve

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// The window is dark until the person picks a theme, whatever the system is
// set to — and so is everything shown before the window's own page: the
// sign-in page and the colour a browser paints an installed page's window
// with. A light rectangle that turns dark a moment later is what this keeps
// away.
func TestLook_DarkBeforeTheWindowDraws(t *testing.T) {
	h := newHarness(t, Options{Locale: "en"})
	resp, body := h.body("GET", "/", nil, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("sign-in page: %d", resp.StatusCode)
	}
	head, rest, ok := strings.Cut(body, "</head>")
	if !ok {
		t.Fatal("no head")
	}

	// Dark is the rule of the page itself: it holds without the script and
	// with the storage closed. The background is the frame's colour.
	if !strings.Contains(head, `:root { color-scheme: dark; --bg: `+frameDark+`;`) {
		t.Errorf("the sign-in page is not dark by default:\n%s", head)
	}
	if n := strings.Count(head, `<meta name="theme-color" content="`+frameDark+`">`); n != 1 {
		t.Errorf("%d dark frame colours in the head, want one", n)
	}
	if strings.Contains(head, `media="(prefers-color-scheme`) {
		t.Error("the frame colour follows the system")
	}
	// Nothing on the page follows a dark system on its own: light comes only
	// with the person's pick — "light", or "as the system" under a light one.
	if strings.Contains(body, "prefers-color-scheme: dark") {
		t.Error("the page still has a rule for a dark system: dark is the default now")
	}
	for _, want := range []string{
		`:root[data-theme="light"] { color-scheme: light; --bg: ` + loginLight + `;`,
		`@media (prefers-color-scheme: light) { :root[data-theme="system"] { color-scheme: light; --bg: ` + loginLight + `;`,
	} {
		if !strings.Contains(head, want) {
			t.Errorf("the head lacks %s", want)
		}
	}

	// The pick is read where the window keeps it, before anything is drawn:
	// the script stands in the head, ahead of the stylesheet, under the
	// nonce the policy allows.
	nonce := regexp.MustCompile(`script-src 'nonce-([0-9a-f]+)'`).FindStringSubmatch(resp.Header.Get("Content-Security-Policy"))
	if nonce == nil {
		t.Fatal("no nonce in the policy")
	}
	script := strings.Index(head, `<script nonce="`+nonce[1]+`">`)
	style := strings.Index(head, "<style>")
	if script < 0 || style < 0 || script > style {
		t.Fatalf("the theme script at %d, the stylesheet at %d: the script must come first, in the head", script, style)
	}
	theme := head[script:style]
	for _, want := range []string{
		`var t = 'dark';`,
		`localStorage.getItem('execai:ui:theme')`,
		`b === 'light' || b === 'system'`,
		`setAttribute('data-theme', t)`,
		`'` + loginLight + `'`,
	} {
		if !strings.Contains(theme, want) {
			t.Errorf("the theme script lacks %s:\n%s", want, theme)
		}
	}
	// A storage that throws leaves the page dark and working.
	if !strings.Contains(theme, "try {") || !strings.Contains(theme, "} catch (e) {}") {
		t.Errorf("the theme script does not survive a closed storage:\n%s", theme)
	}
	if strings.Contains(body, "{{") {
		t.Errorf("a placeholder is left in the page:\n%s", body)
	}
	if !strings.Contains(rest, `id="code"`) {
		t.Error("the page lost its form")
	}

	// The window's own page gets the same frame colour, once, without a
	// media query; the page corrects it for the look and theme picked.
	h.login()
	_, page := h.body("GET", "/", nil, nil)
	phead, _, _ := strings.Cut(page, "</head>")
	if n := strings.Count(phead, `<meta name="theme-color" content="`+frameDark+`">`); n != 1 {
		t.Errorf("%d dark frame colours in the window's head, want one", n)
	}
	if strings.Contains(phead, `name="theme-color" media=`) {
		t.Error("the window's frame colour follows the system")
	}
}

// The page the window shows is the one built for this default: dark until a
// theme is picked, "as the system" kept as a pick, the looks in plain sight.
func TestLook_EmbeddedPageDefaultsToDark(t *testing.T) {
	entries, err := assets.ReadDir("assets")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "panel-browser.") {
			continue
		}
		seen++
		raw, _ := assets.ReadFile("assets/" + e.Name())
		page := string(raw)
		for _, want := range []string{
			`t = 'dark', own = false`,                         // the theme before any pick
			`b === 'light' || b === 'dark' || b === 'system'`, // the three picks, the system among them
			`fresh: !own`,                                       // nothing picked yet: the looks are shown
			`id="look-tabs"`, `id="look-head"`, `id="look-top"`, // the button in every header
			`id="lookIntro"`, `id="sideLook"`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: the embedded page lacks %s — rebuild it (scripts/sync-panel.sh)", e.Name(), want)
			}
		}
	}
	if seen != 2 {
		t.Errorf("%d embedded panel pages, want the two languages", seen)
	}
}
