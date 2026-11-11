package tools

import (
	"html"
	"net/url"
	"regexp"
	"strings"
)

// HTML→text extraction for WebFetch. Deliberately regexp-based rather than a
// DOM parser: the goal is not fidelity but a readable page for the model, with
// no extra dependency in the binary.

// Go's RE2 has no backreferences, so each non-content element gets its own
// pattern instead of one `<(script|style|…)>…</\1>`.
var reDropElements = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<script\b[^>]*>.*?</\s*script\s*>`),
	regexp.MustCompile(`(?is)<style\b[^>]*>.*?</\s*style\s*>`),
	regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</\s*noscript\s*>`),
	regexp.MustCompile(`(?is)<svg\b[^>]*>.*?</\s*svg\s*>`),
	regexp.MustCompile(`(?is)<iframe\b[^>]*>.*?</\s*iframe\s*>`),
}

var (
	reComment    = regexp.MustCompile(`(?s)<!--.*?-->`)
	reHead       = regexp.MustCompile(`(?is)<head\b[^>]*>.*?</\s*head\s*>`)
	reTitle      = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</\s*title\s*>`)
	reBlockOpen  = regexp.MustCompile(`(?i)<(p|div|section|article|br|tr|li|h[1-6]|pre|blockquote)\b[^>]*>`)
	reBlockClose = regexp.MustCompile(`(?i)</\s*(p|div|section|article|tr|li|h[1-6]|pre|blockquote|table)\s*>`)
	reAnchor     = regexp.MustCompile(`(?is)<a\b[^>]*\bhref\s*=\s*["']([^"']+)["'][^>]*>(.*?)</\s*a\s*>`)
	reTag        = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpaces     = regexp.MustCompile(`[ \t]{2,}`)
	reBlankLines = regexp.MustCompile(`\n{3,}`)
)

// pageLink is a link harvested from the page so the model can follow it with a
// second WebFetch — this is what makes the local browser navigable instead of
// being a one-shot dump.
type pageLink struct {
	Text string
	URL  string
}

// htmlToText returns the page title, readable text and outgoing links with
// absolute URLs (resolved against pageURL).
func htmlToText(body, pageURL string) (title, text string, links []pageLink) {
	if m := reTitle.FindStringSubmatch(body); len(m) == 2 {
		title = strings.TrimSpace(html.UnescapeString(reTag.ReplaceAllString(m[1], "")))
	}

	base, _ := url.Parse(pageURL)
	seen := map[string]bool{}
	for _, m := range reAnchor.FindAllStringSubmatch(body, -1) {
		href := strings.TrimSpace(m[1])
		if href == "" || strings.HasPrefix(href, "#") ||
			strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") {
			continue
		}
		abs := href
		if base != nil {
			if u, err := url.Parse(href); err == nil {
				abs = base.ResolveReference(u).String()
			}
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		label := strings.TrimSpace(html.UnescapeString(reTag.ReplaceAllString(m[2], " ")))
		label = strings.Join(strings.Fields(label), " ")
		if label == "" {
			continue // navigation icons and other label-less anchors are noise
		}
		if len(label) > 80 {
			label = label[:80] + "…"
		}
		links = append(links, pageLink{Text: label, URL: abs})
	}

	s := reComment.ReplaceAllString(body, "")
	for _, re := range reDropElements {
		s = re.ReplaceAllString(s, "")
	}
	s = reHead.ReplaceAllString(s, "")
	// Prefer the content region: without this the site's navigation menu lands on
	// top of every page and the model reads "Skip to Main Content" instead of the
	// article. Links are still harvested from the whole document above, on
	// purpose — navigation is what makes the next hop possible.
	s = mainRegion(s)
	s = reBlockOpen.ReplaceAllString(s, "\n")
	s = reBlockClose.ReplaceAllString(s, "\n")
	s = reTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r", "")
	s = reSpaces.ReplaceAllString(s, " ")

	var lines []string
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			lines = append(lines, ln)
		}
	}
	text = reBlankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return title, strings.TrimSpace(text), links
}

var reMainRegions = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<article\b[^>]*>(.*?)</\s*article\s*>`),
	regexp.MustCompile(`(?is)<main\b[^>]*>(.*?)</\s*main\s*>`),
	regexp.MustCompile(`(?is)<div\b[^>]*\b(?:id|class)\s*=\s*["'][^"']*(?:content|article|post|markdown-body)[^"']*["'][^>]*>(.*?)</\s*div\s*>`),
}

// mainRegion narrows the document to its content area when one is recognisable.
// The result is only accepted if it is substantial (>500 bytes) — a tiny <main>
// usually means the markup is unusual and the whole body is the safer bet.
func mainRegion(s string) string {
	for _, re := range reMainRegions {
		if m := re.FindStringSubmatch(s); len(m) == 2 && len(m[1]) > 500 {
			return m[1]
		}
	}
	return s
}

// looksLikeHTML decides whether to run extraction: Content-Type is the primary
// signal, sniffing the body is the fallback for servers that lie.
func looksLikeHTML(contentType, body string) bool {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "text/html"), strings.Contains(ct, "application/xhtml"):
		return true
	case ct != "" && !strings.Contains(ct, "text/plain"):
		return false
	}
	head := body
	if len(head) > 1024 {
		head = head[:1024]
	}
	head = strings.ToLower(head)
	return strings.Contains(head, "<html") || strings.Contains(head, "<!doctype html")
}
