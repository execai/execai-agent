package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LSTool struct{ cwd string }

func (*LSTool) Spec() Spec {
	return Spec{
		Name:        "LS",
		Description: "Листинг содержимого директории. Возвращает имена файлов и директорий (директории помечаются '/' в конце). Размеры и mtime в формате 'name  size  mtime'.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Путь к директории. По умолчанию cwd."},
				"all":  map[string]any{"type": "boolean", "description": "Показывать скрытые файлы (начинающиеся с точки)."},
			},
			"additionalProperties": false,
		},
	}
}

func (*LSTool) RequiresApproval(json.RawMessage) bool { return false }

func (l *LSTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path string `json:"path"`
		All  bool   `json:"all"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	dir := p.Path
	if dir == "" {
		dir = l.cwd
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(l.cwd, dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	type row struct{ name, size, mtime string }
	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !p.All && strings.HasPrefix(name, ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		size := fmt.Sprintf("%d", info.Size())
		mt := info.ModTime().Format("2006-01-02 15:04")
		if e.IsDir() {
			name += "/"
			size = "-"
		}
		rows = append(rows, row{name, size, mt})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "%-40s  %10s  %s\n", r.name, r.size, r.mtime)
	}
	if b.Len() == 0 {
		return "(директория пуста)", nil
	}
	return b.String(), nil
}

type TreeTool struct{ cwd string }

func (*TreeTool) Spec() Spec {
	return Spec{
		Name:        "Tree",
		Description: "Дерево файлов и директорий до заданной глубины (по умолчанию 3). Пропускает .git, node_modules, __pycache__, venv, dist, build, target.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":  map[string]any{"type": "string", "description": "Корень. По умолчанию cwd."},
				"depth": map[string]any{"type": "integer", "description": "Максимальная глубина. По умолчанию 3."},
			},
			"additionalProperties": false,
		},
	}
}

func (*TreeTool) RequiresApproval(json.RawMessage) bool { return false }

func (t *TreeTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path  string `json:"path"`
		Depth int    `json:"depth"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	root := p.Path
	if root == "" {
		root = t.cwd
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(t.cwd, root)
	}
	if p.Depth <= 0 {
		p.Depth = 3
	}
	var b strings.Builder
	walkTree(&b, root, "", p.Depth)
	if b.Len() == 0 {
		return "(пусто)", nil
	}
	return b.String(), nil
}

func walkTree(b *strings.Builder, dir, prefix string, depth int) {
	if depth < 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	visible := entries[:0]
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, ".") {
			continue
		}
		if e.IsDir() {
			if n == "node_modules" || n == "__pycache__" || n == "venv" || n == ".venv" || n == "dist" || n == "build" || n == "target" {
				continue
			}
		}
		visible = append(visible, e)
	}
	for i, e := range visible {
		last := i == len(visible)-1
		branch := "├── "
		if last {
			branch = "└── "
		}
		fmt.Fprintf(b, "%s%s%s", prefix, branch, e.Name())
		if e.IsDir() {
			b.WriteString("/")
		}
		b.WriteString("\n")
		if e.IsDir() {
			subPrefix := prefix + "│   "
			if last {
				subPrefix = prefix + "    "
			}
			walkTree(b, filepath.Join(dir, e.Name()), subPrefix, depth-1)
		}
	}
}
