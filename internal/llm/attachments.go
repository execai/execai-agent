package llm

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// imageQuotedRE — a path in single or double quotes. Inside there may be
// anything (spaces, Cyrillic) except the quote itself. Drag&drop in most
// terminals wraps the path in single quotes.
// A path starts with ~, / (POSIX), a drive letter (C:\… or C:/…) or \\ (UNC).
// Windows was missing here: the panel attaches C:\Users\…\clipboard.png, nothing
// matched, and the model honestly answered that it cannot see any image.
const pathStart = `(?:[~/]|[A-Za-z]:[\\/]|\\\\)`

var imageQuotedRE = regexp.MustCompile(`(?i)'(` + pathStart + `[^']+\.(?:png|jpe?g|gif|webp))'|"(` + pathStart + `[^"]+\.(?:png|jpe?g|gif|webp))"`)

// imageUnquotedRE — an unquoted path, terminated by a space/quote/tab.
var imageUnquotedRE = regexp.MustCompile(`(?i)(?:^|\s)(` + pathStart + `[^\s'"]+\.(?:png|jpe?g|gif|webp))(?:\s|$|[,.;:!?])`)

// imageExtAnyRE — any occurrence of .png/.jpg/etc in the string (for the fallback
// heuristic of finding unquoted paths with spaces).
var imageExtAnyRE = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|webp)(?:\s|$|[,.;:!?'"]|$)`)

// imageExt — all supported extensions (lowercase).
var imageExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// imageMatch is an internal type for accumulating matches from the two regexes.
type imageMatch struct {
	matchStart, matchEnd int    // positions in the text for excision
	path                 string // parsed path (raw)
}

// ExtractImageAttachments scans the text for image paths, reads the files
// and returns (a) the cleaned text (without paths), (b) []ContentBlock with image_urls.
// If no image is found or no file is readable — images=nil.
func ExtractImageAttachments(text string) (clean string, images []ContentBlock) {
	var all []imageMatch
	// First look for quoted paths — they may contain spaces/Cyrillic.
	for _, m := range imageQuotedRE.FindAllStringSubmatchIndex(text, -1) {
		// m: [matchStart matchEnd g1Start g1End g2Start g2End]
		// g1 — single quotes, g2 — double. One of them is -1.
		var raw string
		switch {
		case m[2] != -1:
			raw = text[m[2]:m[3]]
		case m[4] != -1:
			raw = text[m[4]:m[5]]
		default:
			continue
		}
		all = append(all, imageMatch{m[0], m[1], raw})
	}
	// Fallback for unquoted paths WITH SPACES (like "/home/.../Снимок экрана.png"):
	// the regexes do not find them, but we can try to "greedily" take from the start
	// of the string or from the first '/' up to the extension position and check os.Stat.
	// We only do this if the whole string starts with '/' or ' /' — otherwise there are too many false positives.
	if len(all) == 0 && (strings.HasPrefix(text, "/") || strings.HasPrefix(text, "~/")) {
		extMatches := imageExtAnyRE.FindAllStringIndex(text, -1)
		// Walk from the end — take the longest possible match.
		for i := len(extMatches) - 1; i >= 0; i-- {
			em := extMatches[i]
			// em[1] is the end including the trailing char (\s|punct); we need the end of the extension.
			// Find the actual end of .ext (without the trailing delimiter).
			end := em[1]
			if end > 0 && (text[end-1] == ' ' || text[end-1] == '\n' || text[end-1] == '\t' ||
				text[end-1] == ',' || text[end-1] == '.' || text[end-1] == ';' ||
				text[end-1] == ':' || text[end-1] == '!' || text[end-1] == '?' ||
				text[end-1] == '\'' || text[end-1] == '"') {
				end--
			}
			candidate := strings.TrimSpace(text[:end])
			candidate = expandHome(candidate)
			if _, err := os.Stat(candidate); err == nil {
				all = append(all, imageMatch{0, end, candidate})
				break
			}
		}
	}

	// Then unquoted — without capturing quoted regions.
	for _, m := range imageUnquotedRE.FindAllStringSubmatchIndex(text, -1) {
		// Skip if this range is already covered by a quoted match.
		overlap := false
		for _, q := range all {
			if m[2] < q.matchEnd && m[3] > q.matchStart {
				overlap = true
				break
			}
		}
		if overlap {
			continue
		}
		all = append(all, imageMatch{m[0], m[1], text[m[2]:m[3]]})
	}
	if len(all) == 0 {
		return text, nil
	}
	// Sort by match start — for correct excision.
	sortByStart(all)

	var keep []string
	last := 0
	seen := map[string]bool{}
	for _, im := range all {
		path := expandHome(im.path)
		if seen[path] {
			continue
		}
		ext := strings.ToLower(filepath.Ext(path))
		mime, ok := imageExt[ext]
		if !ok {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		images = append(images, ContentBlock{
			Type:     "image_url",
			ImageURL: fmt.Sprintf("data:%s;base64,%s", mime, b64),
		})
		seen[path] = true
		keep = append(keep, text[last:im.matchStart])
		last = im.matchEnd
	}
	keep = append(keep, text[last:])
	clean = strings.TrimSpace(strings.Join(keep, " "))
	return clean, images
}

func sortByStart(s []imageMatch) {
	// Simple insertion sort — there are few ranges.
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j-1].matchStart > s[j].matchStart {
			s[j], s[j-1] = s[j-1], s[j]
			j--
		}
	}
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// BuildUserContent creates Content for a user message: either a plain string
// (if there are no images) or []ContentBlock with text + images.
func BuildUserContent(text string) interface{} {
	clean, images := ExtractImageAttachments(text)
	if len(images) == 0 {
		return text
	}
	blocks := make([]ContentBlock, 0, 1+len(images))
	if clean != "" {
		blocks = append(blocks, ContentBlock{Type: "text", Text: clean})
	}
	blocks = append(blocks, images...)
	return blocks
}

// BuildUserContentWithFiles is the same as BuildUserContent, but also attaches
// images the caller already knows about — the editor panel passes the list of
// attachments explicitly, and guessing them back out of the text is a detour
// that broke on Windows paths. Text parsing stays for the terminal, where a
// path really does arrive inside the message.
func BuildUserContentWithFiles(text string, files []string) interface{} {
	clean, images := ExtractImageAttachments(text)
	seen := map[string]bool{}
	for _, b := range images {
		seen[b.ImageURL] = true
	}
	for _, f := range files {
		blk, ok := imageBlock(f)
		if !ok || seen[blk.ImageURL] {
			continue
		}
		seen[blk.ImageURL] = true
		images = append(images, blk)
	}
	if len(images) == 0 {
		return text
	}
	blocks := make([]ContentBlock, 0, 1+len(images))
	if clean != "" {
		blocks = append(blocks, ContentBlock{Type: "text", Text: clean})
	}
	return append(blocks, images...)
}

// imageBlock reads one image file into a content block.
func imageBlock(path string) (ContentBlock, bool) {
	mime, ok := imageExt[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return ContentBlock{}, false
	}
	data, err := os.ReadFile(expandHome(path))
	if err != nil {
		return ContentBlock{}, false
	}
	return ContentBlock{
		Type:     "image_url",
		ImageURL: fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data)),
	}, true
}
