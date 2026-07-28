package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type GrepTool struct{ cwd string }

func (*GrepTool) Spec() Spec {
	return Spec{
		Name:        "Grep",
		Description: "Поиск регулярного выражения в файлах под путём (рекурсивно). Аналог `grep -rn`. Возвращает строки в формате 'path:line:content'. Поддерживает glob-фильтр и ограничение типа файла.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern":     map[string]any{"type": "string", "description": "Регулярное выражение (Go-синтаксис, RE2)."},
				"path":        map[string]any{"type": "string", "description": "Корень поиска (файл или директория). По умолчанию cwd."},
				"glob":        map[string]any{"type": "string", "description": "Доп. фильтр glob ('*.go', 'src/**/*.ts'). Опционально."},
				"case_insensitive": map[string]any{"type": "boolean", "description": "Игнорировать регистр (i)."},
				"max_matches": map[string]any{"type": "integer", "description": "Максимум совпадений в выводе (по умолчанию 200)."},
			},
			"required":             []string{"pattern"},
			"additionalProperties": false,
		},
	}
}

func (*GrepTool) RequiresApproval(json.RawMessage) bool { return false }

func (g *GrepTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Pattern         string `json:"pattern"`
		Path            string `json:"path"`
		Glob            string `json:"glob"`
		CaseInsensitive bool   `json:"case_insensitive"`
		MaxMatches      int    `json:"max_matches"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	if p.Pattern == "" {
		return "", fmt.Errorf("pattern обязателен")
	}
	if p.MaxMatches <= 0 {
		p.MaxMatches = 200
	}
	root := p.Path
	if root == "" {
		root = g.cwd
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(g.cwd, root)
	}
	pat := p.Pattern
	if p.CaseInsensitive {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return "", fmt.Errorf("плохой regexp: %w", err)
	}

	var out strings.Builder
	matches := 0
	walkErr := filepath.WalkDir(root, func(p2 string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".venv" || name == "venv" || name == "__pycache__" || name == "dist" || name == "build" || name == "target" {
				return filepath.SkipDir
			}
			return nil
		}
		if matches >= p.MaxMatches {
			return filepath.SkipAll
		}
		if p.Glob != "" {
			ok, _ := filepath.Match(p.Glob, filepath.Base(p2))
			if !ok {
				return nil
			}
		}
		if isLikelyBinaryByExt(p2) {
			return nil
		}
		f, err := os.Open(p2)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		ln := 0
		for sc.Scan() {
			ln++
			line := sc.Text()
			if re.MatchString(line) {
				rel, _ := filepath.Rel(g.cwd, p2)
				if rel == "" {
					rel = p2
				}
				if len(line) > 300 {
					line = line[:300] + "..."
				}
				fmt.Fprintf(&out, "%s:%d:%s\n", rel, ln, line)
				matches++
				if matches >= p.MaxMatches {
					break
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return out.String(), walkErr
	}
	if matches == 0 {
		return "(нет совпадений)", nil
	}
	if matches >= p.MaxMatches {
		fmt.Fprintf(&out, "...(показано %d совпадений, могут быть ещё; уточни pattern или path)\n", matches)
	}
	return out.String(), nil
}

func isLikelyBinaryByExt(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".webp", ".pdf",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar",
		".exe", ".dll", ".so", ".dylib", ".o", ".a",
		".woff", ".woff2", ".ttf", ".otf", ".eot",
		".mp3", ".mp4", ".avi", ".mov", ".mkv", ".webm",
		".db", ".sqlite", ".sqlite3":
		return true
	}
	return false
}
