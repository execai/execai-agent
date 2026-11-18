package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// AskUserTool lets the model ask the user a question with a fixed set of
// answers instead of guessing — the equivalent of Claude Code's
// AskUserQuestion. The UI renders a picker; the chosen label comes back as the
// tool result, so the model continues with an unambiguous answer.
//
// Wiring follows the same shape as the tool-approval prompt: the tool does not
// know about the TUI, it calls a hook the chat layer installs at startup.
type AskUserTool struct{}

// AskOption is one answer offered to the user.
type AskOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// askUserFunc is installed by the chat layer (SetAskUserFunc). It blocks until
// the user picks an option or dismisses the question.
var askUserFunc func(ctx context.Context, question string, options []AskOption) (string, error)

// SetAskUserFunc wires the UI implementation of the question picker.
func SetAskUserFunc(f func(ctx context.Context, question string, options []AskOption) (string, error)) {
	askUserFunc = f
}

// ErrAskUnavailable is returned when nothing can render the question — a plain
// REPL, a pipe, a subagent. The model is told to decide on its own instead.
var ErrAskUnavailable = errors.New("интерактивный вопрос недоступен в этом режиме")

func (*AskUserTool) Spec() Spec {
	return Spec{
		Name: "AskUser",
		Description: "Задать пользователю вопрос с вариантами ответа, когда решение " +
			"действительно за ним: развилка в подходе, выбор между несовместимыми " +
			"вариантами, неоднозначное требование. Пользователь выбирает вариант, " +
			"его текст возвращается как результат. НЕ использовать для того, что " +
			"можно выяснить самому из кода или файлов, и для вопросов с очевидным " +
			"ответом по умолчанию — в таких случаях просто действуй.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "Вопрос целиком, коротко и конкретно.",
				},
				"options": map[string]any{
					"type":        "array",
					"minItems":    2,
					"maxItems":    4,
					"description": "От 2 до 4 взаимоисключающих вариантов. Первым — рекомендуемый.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"label":       map[string]any{"type": "string", "description": "Короткий текст варианта (1–5 слов)."},
							"description": map[string]any{"type": "string", "description": "Что произойдёт при этом выборе."},
						},
						"required":             []string{"label"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"question", "options"},
			"additionalProperties": false,
		},
	}
}

// The question IS the confirmation — a second modal on top would be absurd.
func (*AskUserTool) RequiresApproval(json.RawMessage) bool { return false }

func (t *AskUserTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Question string      `json:"question"`
		Options  []AskOption `json:"options"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	p.Question = strings.TrimSpace(p.Question)
	if p.Question == "" {
		return "", fmt.Errorf("question обязателен")
	}
	// Keep the model honest about the contract instead of silently rendering
	// a one-option picker or a wall of twelve.
	if len(p.Options) < 2 {
		return "", fmt.Errorf("нужно минимум 2 варианта, получено %d", len(p.Options))
	}
	if len(p.Options) > 4 {
		p.Options = p.Options[:4]
	}
	for i, o := range p.Options {
		if strings.TrimSpace(o.Label) == "" {
			return "", fmt.Errorf("вариант %d без label", i+1)
		}
	}

	if askUserFunc == nil {
		return "", ErrAskUnavailable
	}
	answer, err := askUserFunc(ctx, p.Question, p.Options)
	if err != nil {
		return "", err
	}
	return answer, nil
}
