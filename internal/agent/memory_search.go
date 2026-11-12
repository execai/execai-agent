package agent

// Searching memory — ours first, other agents' second.
//
// The agent's own memory is a folder of small markdown files, so "search" is a
// grep over them: the frontmatter description says what a file is about, the
// body holds the fact. What matters is the second half: when our memory has
// nothing on a subject, the answer may still exist — in the memory another
// agent on this machine has been keeping. Claude Code stores it per project in
// ~/.claude/projects/<encoded-cwd>/memory/ plus a global CLAUDE.md.
//
// We never read from there behind the user's back at answer time: finding it is
// one thing, adopting it is another. The find is reported, the import is
// offered, and the user decides.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MemoryHit — one matching memory file.
type MemoryHit struct {
	Path        string // absolute
	Name        string // file name without .md
	Description string // frontmatter description, when present
	Line        string // the matching line of the body
	Foreign     bool   // true when it belongs to another agent (Claude Code…)
	Tool        string // which agent it belongs to, for foreign hits
}

// maxSearchFileSize — memory files are notes, not dumps. Anything bigger is
// something else that happened to land in the folder.
const maxSearchFileSize = 512 * 1024

// SearchUserMemory greps the agent's own memory. An empty query lists everything.
func SearchUserMemory(query string) ([]MemoryHit, error) {
	dir, err := UserMemoryDir()
	if err != nil {
		return nil, err
	}
	return searchDir(dir, query, false, ""), nil
}

// ClaudeProjectMemoryDir returns Claude Code's memory folder for a working
// directory. Claude Code encodes the project path into the folder name by
// replacing every separator with a dash: /home/yz/work → -home-yz-work.
func ClaudeProjectMemoryDir(cwd string) string {
	home, err := os.UserHomeDir()
	if err != nil || cwd == "" {
		return ""
	}
	enc := strings.ReplaceAll(filepath.Clean(cwd), string(filepath.Separator), "-")
	enc = strings.ReplaceAll(enc, ":", "-") // windows drive letters
	return filepath.Join(home, ".claude", "projects", enc, "memory")
}

// SearchForeignMemory greps the memory other agents keep on this machine:
// Claude Code's memory for this project, its global folder and CLAUDE.md.
// Returns hits marked Foreign so the caller never confuses them with ours.
func SearchForeignMemory(cwd, query string) []MemoryHit {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var out []MemoryHit
	if d := ClaudeProjectMemoryDir(cwd); d != "" {
		out = append(out, searchDir(d, query, true, "Claude Code")...)
	}
	out = append(out, searchDir(filepath.Join(home, ".claude", "memory"), query, true, "Claude Code")...)
	for _, f := range []string{
		filepath.Join(home, ".claude", "CLAUDE.md"),
		filepath.Join(cwd, "CLAUDE.md"),
		filepath.Join(cwd, ".claude", "CLAUDE.md"),
	} {
		if hit, ok := searchFile(f, query, true, "Claude Code"); ok {
			out = append(out, hit)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func searchDir(dir, query string, foreign bool, tool string) []MemoryHit {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []MemoryHit
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if hit, ok := searchFile(filepath.Join(dir, e.Name()), query, foreign, tool); ok {
			out = append(out, hit)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func searchFile(path, query string, foreign bool, tool string) (MemoryHit, bool) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() || st.Size() > maxSearchFileSize {
		return MemoryHit{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return MemoryHit{}, false
	}
	text := string(data)
	hit := MemoryHit{
		Path:        path,
		Name:        strings.TrimSuffix(filepath.Base(path), ".md"),
		Description: descriptionOf(text),
		Foreign:     foreign,
		Tool:        tool,
	}
	if strings.TrimSpace(query) == "" {
		return hit, true
	}
	q := strings.ToLower(query)
	// Every word must appear somewhere in the file — a two-word query should
	// not match a file that only knows one of them.
	words := strings.Fields(q)
	low := strings.ToLower(text)
	for _, w := range words {
		if !strings.Contains(low, w) {
			return MemoryHit{}, false
		}
	}
	for _, line := range strings.Split(text, "\n") {
		l := strings.ToLower(line)
		matched := true
		for _, w := range words {
			if !strings.Contains(l, w) {
				matched = false
				break
			}
		}
		if matched && strings.TrimSpace(line) != "" {
			hit.Line = strings.TrimSpace(line)
			break
		}
	}
	return hit, true
}

// descriptionOf pulls `description:` out of the frontmatter, if there is one.
func descriptionOf(text string) string {
	if !strings.HasPrefix(text, "---") {
		return ""
	}
	end := strings.Index(text[3:], "\n---")
	if end < 0 {
		return ""
	}
	for _, line := range strings.Split(text[3:3+end], "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "description:"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}
