// Реестр локальных инструментов агента (как у Claude Code: Read, Write, Edit,
// Bash, Grep, Glob, LS, WebFetch). Каждый инструмент — отдельный файл в этом
// пакете, регистрируется в Default(). LLM получает массив Spec() в формате
// OpenAI tool definitions.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Tool — исполнимый инструмент. Ответственность реализации:
// - Spec возвращает JSON-Schema описание для отправки в LLM.
// - Execute получает аргументы как json.RawMessage (то что отдала модель в
//   tool_call.arguments — обычно валидный JSON, но иногда модель может прислать
//   битый, тогда Execute возвращает ошибку которую агент покажет модели).
// - RequiresApproval возвращает true если перед выполнением нужно спросить
//   пользователя. Read-only инструменты возвращают false; Bash зависит от
//   команды (см. tool с динамической проверкой).
type Tool interface {
	Spec() Spec
	Execute(ctx context.Context, args json.RawMessage) (string, error)
	RequiresApproval(args json.RawMessage) bool
}

// StreamingTool — опциональный интерфейс для tools которые могут отдавать вывод
// постепенно (Bash). Если tool его не имплементит, агент использует обычный
// Execute и UI получит весь результат разом по завершении.
type StreamingTool interface {
	Tool
	ExecuteStream(ctx context.Context, args json.RawMessage, onChunk func(string)) (string, error)
}

type Spec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON-Schema (Draft-07 совместимая)
}

// ToolDefinition — формат, который ждёт OpenAI Chat Completions API в поле
// tools: [{type:"function", function:{name, description, parameters}}].
type ToolDefinition struct {
	Type     string `json:"type"`
	Function Spec   `json:"function"`
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Spec().Name] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Definitions — список tool definitions в формате OpenAI/Anthropic для
// отправки в LLM. Сортируется по имени для стабильности промпта (cache hits).
func (r *Registry) Definitions() []ToolDefinition {
	out := make([]ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, ToolDefinition{Type: "function", Function: t.Spec()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Function.Name < out[j].Function.Name })
	return out
}

func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.tools))
	for n := range r.tools {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Default — стандартный набор как у Claude Code, без браузера.
// CWD передаётся в инструменты которые работают с рабочей директорией.
func Default(cwd string) *Registry {
	r := NewRegistry()
	r.Register(&ReadTool{cwd: cwd})
	r.Register(&WriteTool{cwd: cwd})
	r.Register(&EditTool{cwd: cwd})
	r.Register(&BashTool{cwd: cwd})
	r.Register(&GrepTool{cwd: cwd})
	r.Register(&GlobTool{cwd: cwd})
	r.Register(&LSTool{cwd: cwd})
	r.Register(&TreeTool{cwd: cwd})
	r.Register(&WebFetchTool{})
	r.Register(&TodoWriteTool{})
	r.Register(&ScheduleWakeupTool{})
	return r
}

// resolveJSON удобный хелпер для tools: парсит args в указанную структуру.
func resolveJSON[T any](args json.RawMessage, dst *T) error {
	if len(args) == 0 || string(args) == "null" {
		return nil
	}
	if err := json.Unmarshal(args, dst); err != nil {
		return fmt.Errorf("неверный JSON в аргументах: %w", err)
	}
	return nil
}
