package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type WriteTool struct{ cwd string }

func (*WriteTool) Spec() Spec {
	return Spec{
		Name:        "Write",
		Description: "Создаёт новый файл или перезаписывает существующий с заданным содержимым. Перед перезаписью существующего файла обычно полезно его сначала прочитать через Read.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Абсолютный или относительный (от cwd) путь."},
				"content": map[string]any{"type": "string", "description": "Полное содержимое файла."},
			},
			"required":             []string{"path", "content"},
			"additionalProperties": false,
		},
	}
}

// Write always requires approval — it writes to the user's disk.
func (*WriteTool) RequiresApproval(json.RawMessage) bool { return true }

func (w *WriteTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	if p.Path == "" {
		return "", fmt.Errorf("path обязателен")
	}
	abs := p.Path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(w.cwd, abs)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	existed := false
	if _, err := os.Stat(abs); err == nil {
		existed = true
	}
	if err := os.WriteFile(abs, []byte(p.Content), 0o644); err != nil {
		return "", err
	}
	verb := "создан"
	if existed {
		verb = "перезаписан"
	}
	return fmt.Sprintf("Файл %s %s (%d байт)", abs, verb, len(p.Content)), nil
}
