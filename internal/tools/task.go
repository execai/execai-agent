package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// TaskTool delegates a self-contained piece of work to a subagent: its own
// tool-use loop, its own (isolated) context, one block of text back. Useful for
// searches that would otherwise flood the main context — "find every place X is
// used and tell me what actually matters".
//
// Two deliberate limits in this first version:
//
//   - Subagents get a READ-ONLY toolset (see ReadOnly). They investigate; the
//     main agent, whose actions the user approves, writes. That also removes the
//     approval-modal problem entirely — read-only tools never ask.
//   - Subagents cannot spawn subagents: their registry has no Task. Recursion
//     here burns the user's provider quota in a way that is hard to notice.
//
// Subagent calls run sequentially, because the agent loop executes tool calls in
// order. Parallelism can come later; the sequencing is what keeps the streaming
// UI readable today.
type TaskTool struct{}

// subagentFunc is installed by the chat layer (SetSubagentRunner): it knows the
// current LLM client, working directory and system prompt.
var subagentFunc func(ctx context.Context, description, prompt string) (string, error)

// SetSubagentRunner wires the implementation that actually runs a subagent.
func SetSubagentRunner(f func(ctx context.Context, description, prompt string) (string, error)) {
	subagentFunc = f
}

// ErrSubagentUnavailable — no runner installed (plain REPL, or we are already
// inside a subagent).
var ErrSubagentUnavailable = errors.New("субагенты недоступны в этом режиме")

func (*TaskTool) Spec() Spec {
	return Spec{
		Name: "Task",
		Description: "Запустить субагента на отдельную подзадачу. У него свой цикл " +
			"работы и свой контекст, назад возвращается только итог — поэтому это " +
			"выгодно, когда изучение вопроса засорило бы основной диалог: обойти " +
			"много файлов, собрать факты по кодовой базе, проверить несколько мест. " +
			"Субагент умеет ТОЛЬКО читать (Read, Grep, Glob, LS, Tree, WebFetch, " +
			"WebSearch) — писать файлы и запускать команды он не может, это делаешь " +
			"ты сам по его выводам. Формулируй задачу самодостаточно: он не видит " +
			"нашу переписку.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{
					"type":        "string",
					"description": "Краткое название задачи (3–5 слов) — показывается пользователю.",
				},
				"prompt": map[string]any{
					"type": "string",
					"description": "Полная постановка задачи со всем нужным контекстом: " +
						"что искать, где, и что именно вернуть в ответе.",
				},
			},
			"required":             []string{"description", "prompt"},
			"additionalProperties": false,
		},
	}
}

// The subagent is read-only, so there is nothing to confirm.
func (*TaskTool) RequiresApproval(json.RawMessage) bool { return false }

func (t *TaskTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	p.Prompt = strings.TrimSpace(p.Prompt)
	if p.Prompt == "" {
		return "", fmt.Errorf("prompt обязателен")
	}
	if strings.TrimSpace(p.Description) == "" {
		p.Description = "подзадача"
	}
	if subagentFunc == nil {
		return "", ErrSubagentUnavailable
	}
	return subagentFunc(ctx, p.Description, p.Prompt)
}
