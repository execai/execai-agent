// Tool-use loop. Architecturally mirrors Claude Code:
//
//	user → LLM → (text and/or tool_calls)
//	                        │
//	                        ▼
//	     (if tool_calls) execute locally via Registry
//	                        │
//	                        ▼
//	               tool results as role:"tool" messages
//	                        │
//	                        ▼
//	                    LLM → ...
//
// The loop ends when the model returns a response without tool_calls (finish_reason=stop).
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// ApproveDecision is the user's decision on a tool call.
type ApproveDecision int

const (
	ApproveDeny      ApproveDecision = iota // Deny this call
	ApproveOnce                             // Allow only this call
	ApproveTool                             // Allow ALL calls of this tool in the CURRENT session
	ApproveExactArgs                        // Allow exactly this command in the current session
	ApproveAlways                           // Allow forever (persisted in permissions.json)
)

// Approver prompts the user to confirm a tool call.
type Approver interface {
	AskApprove(toolName string, args json.RawMessage, summary string) ApproveDecision
}

// Streamer provides UI callbacks for displaying progress.
type Streamer interface {
	OnText(delta string)
	OnReasoning(delta string)
	OnToolCall(name string, args json.RawMessage)
	OnToolChunk(name string, chunk string) // incremental output from a StreamingTool (e.g. Bash)
	OnToolResult(name string, result string, err error)
	OnIterationStart(n int)
}

type Agent struct {
	Client   llm.StreamingLLM // ExecAI (AICoreClient) or an external subscription (GLMClient etc.)
	Tools    *tools.Registry
	Approver Approver
	Streamer Streamer

	// System prompt — assembled once and sent as the first system message.
	System string

	// Maximum number of tool-use loop iterations, so the model does not loop forever.
	MaxIterations int

	// allowedTools — tools for which the user chose "always allow in this
	// session". Reset when the CLI restarts.
	allowedTools     map[string]bool
	allowedExactArgs map[string]bool // key = name + "|" + json(args)

	// Persistent permissions (stored in ~/.config/execai/permissions.json).
	// Loaded when the Agent is created, updated on ApproveAlways.
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
		MaxIterations:    50,
		allowedTools:     map[string]bool{},
		allowedExactArgs: map[string]bool{},
		Permissions:      perms,
	}
}

// Run executes a single user request and returns the updated history.
// history contains the previous user/assistant/tool messages; a new user
// message is appended (if userMessage is non-empty), followed by everything
// received from the model plus tool results.
func (a *Agent) Run(ctx context.Context, history []llm.AIMessage, userMessage string) ([]llm.AIMessage, error) {
	return a.RunWithFiles(ctx, history, userMessage, nil)
}

// RunWithFiles is Run with attachments the caller already knows about. The
// terminal finds image paths inside the message text; the editor panel knows
// exactly what was attached and says so — no round trip through a regex that
// has to guess what a path looks like on this OS.
func (a *Agent) RunWithFiles(ctx context.Context, history []llm.AIMessage, userMessage string, files []string) ([]llm.AIMessage, error) {
	// Ensure the system message comes first.
	out := ensureSystem(history, a.System)
	if userMessage != "" {
		out = append(out, llm.AIMessage{
			Role:    "user",
			Content: llm.BuildUserContentWithFiles(userMessage, files),
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
			cb.OnToolCall = func(name string) {} // name announced before args accumulate; full args are fetched after Stream
		}
		res, err := a.Client.Stream(ctx, out, defsRaw, cb)
		if err != nil {
			return out, err
		}

		// Append the assistant message to the history as-is.
		assistantMsg := llm.AIMessage{
			Role:      "assistant",
			Content:   res.Content,
			ToolCalls: res.ToolCalls,
		}
		out = append(out, assistantMsg)

		// No tool_calls → final answer.
		if len(res.ToolCalls) == 0 {
			return out, nil
		}

		// Execute each tool_call in order.
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
	// Soft stop: instead of failing the chat with an error, insert an assistant hint.
	// The user sees the context of the last state and can say 'continue'
	// to let the task proceed (next 40 iterations).
	out = append(out, llm.AIMessage{
		Role:    "assistant",
		Content: i18n.Tf("loop.iterationLimit", a.MaxIterations, a.MaxIterations),
	})
	return out, nil
}

func (a *Agent) runTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	t, ok := a.Tools.Get(name)
	if !ok {
		// The model often hallucinates names ("grepbash", "BashBash"). Return
		// a detailed tool_result message with the list of available tools so the
		// agent corrects itself on the next iteration instead of looping.
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
		// Канонический ключ: description и порядок полей не должны превращать
		// повтор той же команды в «новый» вопрос (см. exactkey.go).
		exactKey := ExactKey(name, args)
		// Инструмент может задать СВОЙ масштаб разрешения: чтение — каталог,
		// сеть — домен. Без этого «навсегда» покрывало бы ровно один файл или
		// один URL, и человек отвечал бы на тот же по смыслу вопрос без конца.
		scope := ""
		if sc, ok := t.(tools.Scoped); ok {
			if k := sc.PermissionKey(args); k != "" {
				exactKey = k
				scope = sc.PermissionScope(args)
			}
		}
		// If already allowed — persisted, session-wide, or for this exact command — skip the prompt.
		persistAllow := a.Permissions != nil && (a.Permissions.HasTool(name) || a.Permissions.HasExact(exactKey))
		if !persistAllow && !a.allowedTools[name] && !a.allowedExactArgs[exactKey] {
			summary := summaryFor(name, args)
			// Человек должен видеть, что именно он открывает: «каталог /var/log»,
			// а не только имя файла, по которому пришёл вопрос.
			if scope != "" {
				summary += "\n→ разрешение распространится на: " + scope
			}
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
					// У инструмента со своим масштабом «навсегда» означает
					// каталог или домен, а не весь инструмент: разрешить
					// «весь Read» ради одного каталога — отдать всю машину.
					if scope != "" {
						a.Permissions.AddExact(exactKey)
					} else {
						a.Permissions.AddTool(name)
					}
					if err := a.Permissions.Save(); err != nil {
						// Молча потерянное «навсегда» = вопрос снова при следующем
						// запуске; человек решит, что кнопка не работает.
						fmt.Fprintln(os.Stderr, "не сохранить разрешение:", err)
					}
				}
			case ApproveOnce:
				// just proceed
			}
		}
	}
	// If the tool supports streaming and a Streamer is present, forward chunks to the UI.
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
		// Refresh the system message if present — in case the model/cwd changed.
		history[0].Content = sys
		return history
	}
	out := make([]llm.AIMessage, 0, len(history)+1)
	out = append(out, llm.AIMessage{Role: "system", Content: sys})
	out = append(out, history...)
	return out
}

// summaryFor returns a short human-readable description of the planned call,
// shown to the user in the approval prompt.
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

// SummaryFor — публичная обёртка над summaryFor: короткая подпись вызова для
// интерфейсов (веб, IDE), чтобы каждый не изобретал свою и подписи совпадали.
func SummaryFor(name string, args json.RawMessage) string { return summaryFor(name, args) }
