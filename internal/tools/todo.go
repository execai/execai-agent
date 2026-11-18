package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// TodoWrite is the agent's internal tool for tracking its own tasks (like
// Claude Code's). The list lives only in the execai process memory; it is not
// written to a file.
type TodoWriteTool struct {
	mu    sync.Mutex
	items []TodoItem
}

type TodoItem struct {
	Content    string `json:"content"`
	ActiveForm string `json:"activeForm,omitempty"`
	Status     string `json:"status"` // pending | in_progress | completed
}

func (*TodoWriteTool) Spec() Spec {
	return Spec{
		Name:        "TodoWrite",
		Description: "Управление встроенным to-do списком агента. Используй чтобы планировать многошаговые задачи: запиши все шаги, отмечай in_progress перед началом и completed сразу после завершения. Каждый вызов ПОЛНОСТЬЮ заменяет список — передавай актуальное состояние.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"todos": map[string]any{
					"type":        "array",
					"description": "Массив задач. Каждая: {content, activeForm, status}.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"content":    map[string]any{"type": "string"},
							"activeForm": map[string]any{"type": "string", "description": "Форма с -ing/-ще, для отображения когда статус in_progress."},
							"status":     map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
						},
						"required": []string{"content", "status"},
					},
				},
			},
			"required":             []string{"todos"},
			"additionalProperties": false,
		},
	}
}

func (*TodoWriteTool) RequiresApproval(json.RawMessage) bool { return false }

func (t *TodoWriteTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Todos []TodoItem `json:"todos"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	t.mu.Lock()
	t.items = p.Todos
	items := append([]TodoItem(nil), t.items...)
	t.mu.Unlock()

	var b strings.Builder
	b.WriteString("Текущий список задач:\n")
	for i, it := range items {
		mark := "[ ]"
		switch it.Status {
		case "in_progress":
			mark = "[~]"
		case "completed":
			mark = "[✓]"
		}
		fmt.Fprintf(&b, "%s %d. %s\n", mark, i+1, it.Content)
	}
	return b.String(), nil
}

// Items returns a copy of the current tasks — for rendering in the TUI.
func (t *TodoWriteTool) Items() []TodoItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]TodoItem(nil), t.items...)
}
