// Tool-use loop. Архитектурно повторяет Claude Code:
//
//   user → LLM → (текст и/или tool_calls)
//                           │
//                           ▼
//        (если tool_calls) выполнить локально через Registry
//                           │
//                           ▼
//                  tool результаты как role:"tool" сообщения
//                           │
//                           ▼
//                       LLM → ...
//
// Цикл заканчивается когда модель вернула ответ без tool_calls (finish_reason=stop).
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// ApproveDecision — решение пользователя по tool-вызову.
type ApproveDecision int

const (
	ApproveDeny       ApproveDecision = iota // Отклонить этот вызов
	ApproveOnce                              // Разрешить только этот вызов
	ApproveTool                              // Разрешить ВСЕ вызовы этого инструмента в ТЕКУЩЕЙ сессии
	ApproveExactArgs                         // Разрешить именно эту команду в текущей сессии
	ApproveAlways                            // Разрешить навсегда (persist в permissions.json)
)

// Approver — опросчик пользователя для подтверждения tool-вызова.
type Approver interface {
	AskApprove(toolName string, args json.RawMessage, summary string) ApproveDecision
}

// Streamer — UI-обратные вызовы для отображения хода работы.
type Streamer interface {
	OnText(delta string)
	OnReasoning(delta string)
	OnToolCall(name string, args json.RawMessage)
	OnToolChunk(name string, chunk string) // постепенный вывод от StreamingTool (например Bash)
	OnToolResult(name string, result string, err error)
	OnIterationStart(n int)
}

type Agent struct {
	Client   llm.StreamingLLM // ExecAI (AICoreClient) или внешняя подписка (GLMClient и т.п.)
	Tools    *tools.Registry
	Approver Approver
	Streamer Streamer

	// Системный prompt — собирается один раз и шлётся как первое system-сообщение.
	System string

	// Максимум итераций tool-use loop, чтобы модель не зацикливалась.
	MaxIterations int

	// allowedTools — инструменты, для которых пользователь нажал "разрешить
	// всегда в этой сессии". Сбрасывается при перезапуске CLI.
	allowedTools     map[string]bool
	allowedExactArgs map[string]bool // ключ = name + "|" + json(args)

	// Persistent-разрешения (хранятся в ~/.config/execai/permissions.json).
	// Загружаются при создании Agent, обновляются при ApproveAlways.
	Permissions *Permissions
}

func New(cli llm.StreamingLLM, reg *tools.Registry, sys string, ap Approver, st Streamer) *Agent {
	perms, _ := LoadPermissions()
	return &Agent{
		Client:           cli,
		Tools:            reg,
		System:           sys,
		Approver:         ap,
		Streamer:         st,
		MaxIterations:    40,
		allowedTools:     map[string]bool{},
		allowedExactArgs: map[string]bool{},
		Permissions:      perms,
	}
}

// Run выполняет один user-запрос и возвращает обновлённую историю.
// history содержит предыдущие user/assistant/tool-сообщения; в конце добавляется
// новый user (если userMessage не пуст) и далее всё что пришло от модели + tool результаты.
func (a *Agent) Run(ctx context.Context, history []llm.AIMessage, userMessage string) ([]llm.AIMessage, error) {
	// Гарантируем что system-сообщение в начале.
	out := ensureSystem(history, a.System)
	if userMessage != "" {
		out = append(out, llm.AIMessage{
			Role:    "user",
			Content: llm.BuildUserContent(userMessage), // авто-вложение картинок по пути в тексте
		})
	}

	defs := a.Tools.Definitions()
	defsRaw := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		raw, _ := json.Marshal(d)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		defsRaw = append(defsRaw, m)
	}

	for i := 0; i < a.MaxIterations; i++ {
		if a.Streamer != nil {
			a.Streamer.OnIterationStart(i + 1)
		}
		cb := llm.StreamCallbacks{}
		if a.Streamer != nil {
			cb.OnText = a.Streamer.OnText
			cb.OnReasoning = a.Streamer.OnReasoning
			cb.OnToolCall = func(name string) {} // объявление имени до накопления args; полные args дёрнем после Stream
		}
		res, err := a.Client.Stream(ctx, out, defsRaw, cb)
		if err != nil {
			return out, err
		}

		// Добавляем assistant-сообщение как есть в историю.
		assistantMsg := llm.AIMessage{
			Role:      "assistant",
			Content:   res.Content,
			ToolCalls: res.ToolCalls,
		}
		out = append(out, assistantMsg)

		// Нет tool_calls → финал.
		if len(res.ToolCalls) == 0 {
			return out, nil
		}

		// Выполняем каждый tool_call по очереди.
		for _, tc := range res.ToolCalls {
			args := json.RawMessage(tc.Function.Arguments)
			if a.Streamer != nil {
				a.Streamer.OnToolCall(tc.Function.Name, args)
			}
			result, err := a.runTool(ctx, tc.Function.Name, args)
			if a.Streamer != nil {
				a.Streamer.OnToolResult(tc.Function.Name, result, err)
			}
			content := result
			if err != nil {
				content = fmt.Sprintf("ERROR: %v", err)
				if result != "" {
					content += "\n\n" + result
				}
			}
			out = append(out, llm.AIMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    content,
			})
		}
	}
	// Мягкая остановка: не роняем чат ошибкой, а вставляем assistant-подсказку.
	// Юзер видит контекст последнего состояния и может сказать 'продолжай'
	// чтобы задача поехала дальше (следующие 40 итераций).
	out = append(out, llm.AIMessage{
		Role: "assistant",
		Content: fmt.Sprintf(
			"⚠ Достигнут лимит %d итераций — задача не закрыта. Скажи «продолжай» чтобы дать ещё %d, или переформулируй.",
			a.MaxIterations, a.MaxIterations),
	})
	return out, nil
}

func (a *Agent) runTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	t, ok := a.Tools.Get(name)
	if !ok {
		// Модель часто галлюцинирует имена ("grepbash", "BashBash"). Возвращаем
		// в tool_result детальное сообщение со списком доступных, чтобы агент
		// сам поправился в следующей итерации, а не зацикливался.
		names := a.Tools.Names()
		hint := fmt.Sprintf(
			"ОШИБКА: инструмент %q не существует.\n"+
				"Доступные инструменты (используй ТОЧНОЕ имя из этого списка):\n  %s\n"+
				"Подсказки:\n"+
				"  - искать в файлах = Grep (не grepbash, не bash_grep)\n"+
				"  - выполнить shell-команду = Bash (не Bash_run, не RunBash)\n"+
				"  - читать файл = Read\n"+
				"  - писать/редактировать = Write / Edit\n"+
				"Повтори вызов с правильным именем.",
			name, strings.Join(names, ", "))
		return hint, nil
	}
	if t.RequiresApproval(args) && a.Approver != nil {
		exactKey := name + "|" + string(args)
		// Если уже разрешено: persist-всегда, в сессии, или для этой команды — пропускаем.
		persistAllow := a.Permissions != nil && (a.Permissions.HasTool(name) || a.Permissions.HasExact(exactKey))
		if !persistAllow && !a.allowedTools[name] && !a.allowedExactArgs[exactKey] {
			summary := summaryFor(name, args)
			decision := a.Approver.AskApprove(name, args, summary)
			switch decision {
			case ApproveDeny:
				return "Пользователь отклонил выполнение этого инструмента.", nil
			case ApproveTool:
				a.allowedTools[name] = true
			case ApproveExactArgs:
				a.allowedExactArgs[exactKey] = true
			case ApproveAlways:
				if a.Permissions != nil {
					a.Permissions.AddTool(name)
					_ = a.Permissions.Save()
				}
			case ApproveOnce:
				// просто пропускаем дальше
			}
		}
	}
	// Если tool поддерживает стриминг и есть Streamer — отдаём чанки в UI.
	if st, ok := t.(tools.StreamingTool); ok && a.Streamer != nil {
		return st.ExecuteStream(ctx, args, func(chunk string) {
			a.Streamer.OnToolChunk(name, chunk)
		})
	}
	return t.Execute(ctx, args)
}

func ensureSystem(history []llm.AIMessage, sys string) []llm.AIMessage {
	if sys == "" {
		return history
	}
	if len(history) > 0 && history[0].Role == "system" {
		// Обновляем system если он был — на случай если изменилась модель/cwd.
		history[0].Content = sys
		return history
	}
	out := make([]llm.AIMessage, 0, len(history)+1)
	out = append(out, llm.AIMessage{Role: "system", Content: sys})
	out = append(out, history...)
	return out
}

// summaryFor — короткое человекочитаемое описание планируемого вызова, чтобы
// показать пользователю в подтверждении.
func summaryFor(name string, args json.RawMessage) string {
	var generic map[string]any
	_ = json.Unmarshal(args, &generic)
	switch name {
	case "Bash":
		if cmd, _ := generic["command"].(string); cmd != "" {
			if desc, _ := generic["description"].(string); desc != "" {
				return fmt.Sprintf("Bash: %s — %s", cmd, desc)
			}
			return "Bash: " + cmd
		}
	case "Write":
		if p, _ := generic["path"].(string); p != "" {
			return "Write: " + p
		}
	case "Edit":
		if p, _ := generic["path"].(string); p != "" {
			return "Edit: " + p
		}
	}
	raw, _ := json.Marshal(generic)
	return name + " " + string(raw)
}
