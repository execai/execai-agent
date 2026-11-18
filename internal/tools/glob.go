package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

type GlobTool struct{ cwd string }

func (*GlobTool) Spec() Spec {
	return Spec{
		Name:        "Glob",
		Description: "Возвращает пути файлов по glob-шаблону относительно cwd или указанного path. Шаблон поддерживает '*', '**', '?'. Сортируется по mtime (свежие сверху).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Например 'src/**/*.go', '*.md', '**/*test*.py'."},
				"path":    map[string]any{"type": "string", "description": "Корень обхода (по умолчанию cwd)."},
				"limit":   map[string]any{"type": "integer", "description": "Максимум путей в выводе (по умолчанию 200)."},
			},
			"required":             []string{"pattern"},
			"additionalProperties": false,
		},
	}
}

func (g *GlobTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Limit   int    `json:"limit"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	if p.Pattern == "" {
		return "", fmt.Errorf("pattern обязателен")
	}
	if p.Limit <= 0 {
		p.Limit = 200
	}
	root := p.Path
	if root == "" {
		root = g.cwd
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(g.cwd, root)
	}

	var matches []match
	_ = filepath.WalkDir(root, func(pp string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			n := d.Name()
			if n == ".git" || n == "node_modules" || n == ".venv" || n == "venv" || n == "__pycache__" || n == "dist" || n == "build" || n == "target" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, pp)
		if rel == "" {
			rel = pp
		}
		if matchGlob(p.Pattern, rel) {
			info, _ := d.Info()
			abs := pp
			if relCwd, err := filepath.Rel(g.cwd, pp); err == nil {
				abs = relCwd
			}
			matches = append(matches, match{path: abs, mtime: info.ModTime().Unix()})
		}
		return nil
	})
	sort.Slice(matches, func(i, j int) bool { return matches[i].mtime > matches[j].mtime })
	if len(matches) > p.Limit {
		matches = matches[:p.Limit]
	}
	if len(matches) == 0 {
		return "(нет совпадений)", nil
	}
	var b strings.Builder
	for _, m := range matches {
		b.WriteString(m.path)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

type match struct {
	path  string
	mtime int64
}

// matchGlob supports ** for recursion. The standard filepath.Match does not understand **.
func matchGlob(pat, path string) bool {
	if !strings.Contains(pat, "**") {
		ok, _ := filepath.Match(pat, path)
		if ok {
			return true
		}
		// Also check against the basename (the typical '*.go' case).
		ok2, _ := filepath.Match(pat, filepath.Base(path))
		return ok2
	}
	// Treat ** as an arbitrary prefix of directories.
	parts := strings.Split(pat, "**")
	idx := 0
	for i, part := range parts {
		part = strings.Trim(part, "/")
		if part == "" {
			continue
		}
		// Look for a match of the part segment after idx using filepath.Match.
		segs := strings.Split(path[idx:], "/")
		found := -1
		for j := range segs {
			candidate := strings.Join(segs[:j+1], "/")
			ok, _ := filepath.Match(part, candidate)
			if !ok {
				ok, _ = filepath.Match(part, segs[j])
			}
			if ok {
				if i == len(parts)-1 {
					return true
				}
				found = idx + len(strings.Join(segs[:j+1], "/"))
				break
			}
		}
		if found < 0 {
			return false
		}
		idx = found
	}
	return true
}
