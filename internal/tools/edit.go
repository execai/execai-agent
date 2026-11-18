package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/writelock"
)

type EditTool struct{ cwd string }

func (*EditTool) Spec() Spec {
	return Spec{
		Name:        "Edit",
		Description: "Точная замена в существующем файле. old_string должен встречаться в файле РОВНО ОДИН РАЗ (включая отступы), иначе Edit вернёт ошибку — уточни old_string, добавив контекст. Чтобы заменить все вхождения, используй replace_all=true.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":        map[string]any{"type": "string"},
				"old_string":  map[string]any{"type": "string", "description": "Точный текст, который нужно найти и заменить (с учётом отступов)."},
				"new_string":  map[string]any{"type": "string", "description": "Текст-замена."},
				"replace_all": map[string]any{"type": "boolean", "description": "Заменить все вхождения (по умолчанию false)."},
			},
			"required":             []string{"path", "old_string", "new_string"},
			"additionalProperties": false,
		},
	}
}

func (*EditTool) RequiresApproval(json.RawMessage) bool { return true }

func (e *EditTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	if p.Path == "" || p.OldString == "" {
		return "", fmt.Errorf("path и old_string обязательны")
	}
	if p.OldString == p.NewString {
		return "", fmt.Errorf("old_string и new_string совпадают — нечего менять")
	}
	abs := p.Path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(e.cwd, abs)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", err
	}
	// The read and the write under one hold of the lock sync's
	// replacements take in the synced folders (internal/writelock): a
	// version sync writes cannot come between them and be written over
	// with an edit of the text before it.
	count := 0
	err := writelock.Update(abs, 0o644, func(data []byte) ([]byte, error) {
		src := string(data)
		count = strings.Count(src, p.OldString)
		if count == 0 {
			return nil, fmt.Errorf("old_string не найден в файле %s", abs)
		}
		if !p.ReplaceAll && count > 1 {
			return nil, fmt.Errorf("old_string встречается %d раз в %s — добавь контекст или используй replace_all=true", count, abs)
		}
		if p.ReplaceAll {
			return []byte(strings.ReplaceAll(src, p.OldString, p.NewString)), nil
		}
		return []byte(strings.Replace(src, p.OldString, p.NewString, 1)), nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Файл %s обновлён (%d замен).", abs, count), nil
}
