package llm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Which `claude` to run.
//
// The first `claude` in PATH is often not the one the person actually uses.
// A global npm install under /usr/lib is root-owned, cannot update itself and
// quietly stays months behind, while Claude Code in the editor ships its own
// fresh binary inside the extension. An old CLI resolves the `opus`/`sonnet`
// aliases to old models and rejects new IDs outright ("version 2.1.280 or
// newer is required") — from the outside it looks like the agent serves old
// models. So every known install is considered and the newest one wins.
//
// EXECAI_CLAUDE_BIN pins an exact binary and skips the search.

type claudeBin struct {
	Path    string
	Version string // "2.1.292"; empty when it could not be determined
}

// extDirRe matches an editor extension folder of Claude Code:
// anthropic.claude-code-2.1.292-linux-x64 (the platform suffix is optional).
var extDirRe = regexp.MustCompile(`^anthropic\.claude-code-(\d+\.\d+\.\d+)(?:-|$)`)

// versionRe pulls the version out of `claude --version` ("2.1.292 (Claude Code)").
var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

// findClaude returns the newest Claude Code binary on this machine.
func findClaude() (claudeBin, bool) {
	if p := strings.TrimSpace(os.Getenv("EXECAI_CLAUDE_BIN")); p != "" {
		if isExecutable(p) {
			return claudeBin{Path: p, Version: binaryVersion(p)}, true
		}
		return claudeBin{}, false
	}
	best, found := claudeBin{}, false
	for _, c := range claudeCandidates() {
		if !found || versionLess(best.Version, c.Version) {
			best, found = c, true
		}
	}
	return best, found
}

// claudeCandidates lists every install found, in order of preference when
// versions are equal: PATH first (the person's explicit choice), then the
// native installer, then binaries bundled with editor extensions.
func claudeCandidates() []claudeBin {
	var out []claudeBin
	seen := map[string]bool{}
	add := func(path, version string) {
		if path == "" || !isExecutable(path) {
			return
		}
		key := path
		if real, err := filepath.EvalSymlinks(path); err == nil {
			key = real
		}
		if seen[key] {
			return
		}
		seen[key] = true
		if version == "" {
			version = binaryVersion(path)
		}
		out = append(out, claudeBin{Path: path, Version: version})
	}

	if p, err := exec.LookPath("claude"); err == nil {
		add(p, "")
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		return out
	}
	exe := "claude"
	if runtime.GOOS == "windows" {
		exe = "claude.exe"
	}
	add(filepath.Join(home, ".local", "bin", exe), "")
	add(filepath.Join(home, ".claude", "local", exe), "")

	// Editors keep each extension version in its own folder; the version is
	// in the folder name, so no process has to be started to learn it.
	for _, editor := range []string{
		".vscode", ".vscode-insiders", ".vscode-oss", ".cursor", ".windsurf",
		".execai-studio", ".vscode-server", ".cursor-server",
	} {
		dir := filepath.Join(home, editor, "extensions")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			m := extDirRe.FindStringSubmatch(e.Name())
			if m == nil || !e.IsDir() {
				continue
			}
			add(filepath.Join(dir, e.Name(), "resources", "native-binary", exe), m[1])
		}
	}
	return out
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode()&0o111 != 0
}

// versionCache keeps `--version` answers per binary: the client is built on
// every turn, and starting a 200 MB binary each time only to read its version
// would add a noticeable pause. Keyed by path, size and mtime — an update in
// place changes at least one of them.
var versionCache sync.Map // string → string

func binaryVersion(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	key := path + "|" + strconv.FormatInt(fi.Size(), 10) + "|" + fi.ModTime().String()
	if v, ok := versionCache.Load(key); ok {
		return v.(string)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, path, "--version").Output()
	v := ""
	if err == nil {
		v = versionRe.FindString(string(raw))
	}
	versionCache.Store(key, v)
	return v
}

// versionLess compares dotted versions numerically; an unknown version loses
// to any known one.
func versionLess(a, b string) bool {
	if a == "" || b == "" {
		return a == "" && b != ""
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}
