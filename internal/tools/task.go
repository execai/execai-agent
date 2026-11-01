package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// TaskTool delegates a self-contained piece of work to a subagent: its own
// tool-use loop, its own (isolated) context, one block of text back.
//
// A subagent may only read (mode "read", the default: the ReadOnly set) or
// also act (mode "write": the full set without Task, so there is no
// recursion). A write subagent asks the person through the main agent's
// approver, marked with its title; a read subagent is refused everything
// that needs consent.
//
// Several Task calls in one model message run at the same time (the loop
// treats Task as a ParallelTool). With background=true the call returns the
// subagent's id at once; the result comes back later as an automatic message
// or through TaskWait.
type TaskTool struct{}

// SubagentRequest is a Task call as the runner receives it.
type SubagentRequest struct {
	Description   string // the title shown to the person
	Prompt        string
	Mode          string // "read" | "write"
	Model         string // "same" | "fast" | a model id of the active source
	Cwd           string // relative to the parent's directory, never above it
	MaxIterations int    // 0 = the default
	Background    bool
	// ToolCall — the key of the parent's tool call (see WithToolCallID).
	ToolCall string
}

// SubagentRunner runs subagents for one turn of a parent agent. The host
// (terminal, editor panel, execai serve) installs it in the turn's context
// with WithSubagentRunner, built from that turn's snapshot of model, source,
// directory and approver.
type SubagentRunner interface {
	// Run starts a subagent. In the foreground it returns the result; in
	// the background it returns at once with a note carrying the id.
	Run(ctx context.Context, req SubagentRequest) (string, error)
	// Wait blocks until the given background subagent ("all" or empty —
	// every unfinished or unread one of this parent) is done and returns
	// what it found.
	Wait(ctx context.Context, id string) (string, error)
}

type subagentRunnerKey struct{}

// WithSubagentRunner installs the runner for Task/TaskWait calls made under ctx.
func WithSubagentRunner(ctx context.Context, r SubagentRunner) context.Context {
	return context.WithValue(ctx, subagentRunnerKey{}, r)
}

// SubagentRunnerFrom returns the runner installed in ctx, or nil.
func SubagentRunnerFrom(ctx context.Context) SubagentRunner {
	r, _ := ctx.Value(subagentRunnerKey{}).(SubagentRunner)
	return r
}

type toolCallKey struct{}

// WithToolCallID tags ctx with the key of the tool call being executed; the
// agent loop sets it for every call. A subagent records it as the parent's
// tool call, so an interface can link the Task card to the subagent.
func WithToolCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, toolCallKey{}, id)
}

// ToolCallID returns the key set by WithToolCallID, or "".
func ToolCallID(ctx context.Context) string {
	id, _ := ctx.Value(toolCallKey{}).(string)
	return id
}

// ParallelTool marks tools whose calls may run alongside each other in one
// assistant message.
type ParallelTool interface {
	Parallel() bool
}

// subagentFunc is the legacy process-wide runner (SetSubagentRunner), used
// only when the turn has no runner in its context.
var subagentFunc func(ctx context.Context, description, prompt string) (string, error)

// SetSubagentRunner wires a process-wide fallback runner. Hosts install a
// per-turn runner with WithSubagentRunner; this one only serves callers that
// do not.
func SetSubagentRunner(f func(ctx context.Context, description, prompt string) (string, error)) {
	subagentFunc = f
}

// ErrSubagentUnavailable — no runner installed (plain REPL, or we are already
// inside a subagent).
var ErrSubagentUnavailable = errors.New("субагенты недоступны в этом режиме")

// Subagent limits shared by every host.
const (
	SubagentDefaultIterations = 15
	SubagentMaxIterations     = 50
)

func (*TaskTool) Spec() Spec {
	return Spec{
		Name: "Task",
		Description: "Запустить субагента на отдельную подзадачу. У него свой цикл работы и свой " +
			"контекст, назад возвращается только итог — поэтому это выгодно, когда изучение " +
			"вопроса засорило бы основной диалог: обойти много файлов, собрать факты, проверить " +
			"несколько мест. НЕСКОЛЬКО вызовов Task в одном ответе выполняются ОДНОВРЕМЕННО — " +
			"независимые подзадачи отдавай пачкой. mode=read (по умолчанию) — только чтение " +
			"(Read, Grep, Glob, LS, Tree, WebFetch, WebSearch); mode=write — ещё Write, Edit, Bash, " +
			"на действия он спросит человека с пометкой своего имени. background=true — вернуть " +
			"id сразу и продолжать работу; итог придёт отдельным сообщением или через TaskWait. " +
			"Формулируй задачу самодостаточно: он не видит нашу переписку.",
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
						"что искать или сделать, где, и что именно вернуть в ответе.",
				},
				"mode": map[string]any{
					"type":        "string",
					"enum":        []string{"read", "write"},
					"description": "read — только читать (по умолчанию); write — может менять файлы и запускать команды с разрешения человека.",
				},
				"model": map[string]any{
					"type": "string",
					"description": "same — твоя модель (по умолчанию); fast — самая дешёвая модель текущего " +
						"источника (для простого поиска); либо id модели текущего источника.",
				},
				"cwd": map[string]any{
					"type":        "string",
					"description": "Каталог субагента относительно твоего (не выше него). По умолчанию — твой.",
				},
				"max_iterations": map[string]any{
					"type":        "integer",
					"description": fmt.Sprintf("Предел шагов субагента (по умолчанию %d, максимум %d).", SubagentDefaultIterations, SubagentMaxIterations),
				},
				"background": map[string]any{
					"type":        "boolean",
					"description": "true — не ждать: вернуть id сразу, итог придёт сам или через TaskWait.",
				},
			},
			"required":             []string{"description", "prompt"},
			"additionalProperties": false,
		},
	}
}

// Task itself never asks: what the subagent does is asked (or refused) call
// by call inside it.
func (*TaskTool) RequiresApproval(json.RawMessage) bool { return false }

// Parallel: several Task calls of one message run at the same time.
func (*TaskTool) Parallel() bool { return true }

// ParseSubagentRequest validates Task arguments.
func ParseSubagentRequest(args json.RawMessage) (SubagentRequest, error) {
	var p struct {
		Description   string `json:"description"`
		Prompt        string `json:"prompt"`
		Mode          string `json:"mode"`
		Model         string `json:"model"`
		Cwd           string `json:"cwd"`
		MaxIterations int    `json:"max_iterations"`
		Background    bool   `json:"background"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return SubagentRequest{}, err
	}
	req := SubagentRequest{
		Description: strings.TrimSpace(p.Description),
		Prompt:      strings.TrimSpace(p.Prompt),
		Mode:        strings.ToLower(strings.TrimSpace(p.Mode)),
		Model:       strings.TrimSpace(p.Model),
		Cwd:         strings.TrimSpace(p.Cwd),
		Background:  p.Background,
	}
	if req.Prompt == "" {
		return req, fmt.Errorf("prompt обязателен")
	}
	if req.Description == "" {
		req.Description = "подзадача"
	}
	switch req.Mode {
	case "", "read":
		req.Mode = "read"
	case "write":
	default:
		return req, fmt.Errorf("mode: read или write, а не %q", p.Mode)
	}
	if req.Model == "" {
		req.Model = "same"
	}
	switch {
	case p.MaxIterations < 0:
		return req, fmt.Errorf("max_iterations не может быть отрицательным")
	case p.MaxIterations == 0:
		req.MaxIterations = SubagentDefaultIterations
	case p.MaxIterations > SubagentMaxIterations:
		req.MaxIterations = SubagentMaxIterations
	default:
		req.MaxIterations = p.MaxIterations
	}
	return req, nil
}

func (t *TaskTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	req, err := ParseSubagentRequest(args)
	if err != nil {
		return "", err
	}
	req.ToolCall = ToolCallID(ctx)
	if r := SubagentRunnerFrom(ctx); r != nil {
		return r.Run(ctx, req)
	}
	if subagentFunc == nil {
		return "", ErrSubagentUnavailable
	}
	return subagentFunc(ctx, req.Description, req.Prompt)
}

// TaskWaitTool waits for background subagents started with Task.
type TaskWaitTool struct{}

func (*TaskWaitTool) Spec() Spec {
	return Spec{
		Name: "TaskWait",
		Description: "Дождаться субагентов, запущенных Task с background=true, и получить их итог. " +
			"id — идентификатор из ответа Task, или all — все ещё не прочитанные. " +
			"Итог, полученный здесь, отдельным сообщением уже не придёт.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "id субагента или all (по умолчанию all).",
				},
			},
			"additionalProperties": false,
		},
	}
}

func (*TaskWaitTool) RequiresApproval(json.RawMessage) bool { return false }

func (*TaskWaitTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	r := SubagentRunnerFrom(ctx)
	if r == nil {
		return "", ErrSubagentUnavailable
	}
	id := strings.TrimSpace(p.ID)
	if id == "" {
		id = "all"
	}
	return r.Wait(ctx, id)
}
