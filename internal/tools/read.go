package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ReadTool struct{ cwd string }

func (*ReadTool) Spec() Spec {
	return Spec{
		Name:        "Read",
		Description: "Читает файл с диска. Возвращает содержимое с номерами строк (формат: 'NNNN\\tcontent'). Поддерживает чтение части файла через offset/limit (по умолчанию первые 2000 строк).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Абсолютный или относительный (от cwd) путь к файлу."},
				"offset": map[string]any{"type": "integer", "description": "Номер первой строки (1-based). Опционально."},
				"limit":  map[string]any{"type": "integer", "description": "Максимум строк к возврату. По умолчанию 2000."},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
	}
}

func (r *ReadTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	if p.Path == "" {
		return "", fmt.Errorf("path обязателен")
	}
	abs := p.Path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(r.cwd, abs)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if !isProbablyText(data) {
		return fmt.Sprintf("(%s — бинарный файл, %d байт; Read возвращает только текст)", abs, len(data)), nil
	}
	lines := strings.Split(string(data), "\n")
	start := p.Offset
	if start <= 0 {
		start = 1
	}
	end := start + p.Limit - 1
	if p.Limit <= 0 {
		end = start + 2000 - 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) {
		return fmt.Sprintf("(файл имеет %d строк; offset=%d за пределами)", len(lines), start), nil
	}

	var b strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%5d\t%s\n", i, lines[i-1])
	}
	if end < len(lines) {
		fmt.Fprintf(&b, "(...показано %d-%d из %d строк, есть ещё)\n", start, end, len(lines))
	}
	return b.String(), nil
}

func isProbablyText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	// Heuristic: count NUL bytes and non-printables. If >5% — binary.
	bad := 0
	check := len(b)
	if check > 4096 {
		check = 4096
	}
	for i := 0; i < check; i++ {
		c := b[i]
		if c == 0 {
			return false
		}
		if c < 0x09 || (c > 0x0d && c < 0x20) {
			bad++
		}
	}
	return bad*20 < check
}
