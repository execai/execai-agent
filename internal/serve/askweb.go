// Спрашивание разрешения через веб-чат.
//
// До этого фоновый режим отклонял всё, чего нет в permissions.json, и человек
// узнавал об этом только по остановившейся задаче. Теперь агент спрашивает, а
// владелец отвечает из чата теми же вариантами, что видит в TUI: «Разово»,
// «Весь <инструмент> в этой задаче», «Эту команду в этой задаче», «НАВСЕГДА»,
// «Отклонить». Подписи кнопок собирает сервис (agents-vbai internal/ask), и они
// называют РЕАЛЬНУЮ широту решения: session и always открывают инструмент
// целиком, exact — только точный повтор вызова.
//
// The answers and their meaning are one vocabulary on the service
// (agents-vbai internal/ask); the agent repeats it in exactly one place,
// internal/inbox (Answer*).
package serve

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
)

// webApprover спрашивает разрешение в веб-чате вместо немого отказа.
//
// Живёт на одну задачу: sessionAllow — это «Сессия» в терминах TUI, и
// логично, что она не переживает переход к следующей задаче из веба.
type webApprover struct {
	client *inbox.Client
	taskID string
	audit  *auditLog

	// perms — курированные разрешения; сюда дописывает ответ «Всегда».
	perms *agent.Permissions
	// sessionAllow — что разрешено до конца этой задачи.
	sessionAllow map[string]bool
	// ctx — контекст задачи: обрыв должен прекращать ожидание, а не висеть.
	ctx context.Context
}

func newWebApprover(ctx context.Context, cfg *config.Config, token, taskID string,
	perms *agent.Permissions, audit *auditLog) *webApprover {
	return &webApprover{
		client: inbox.NewClient(cfg.APIBase, token), taskID: taskID, audit: audit,
		perms: perms, sessionAllow: map[string]bool{}, ctx: ctx,
	}
}

func (a *webApprover) AskApprove(toolName string, args json.RawMessage, summary string) agent.ApproveDecision {
	// Канонический ключ — тот же, что в TUI-цикле: description и порядок
	// полей не делают повтор команды «новым» вызовом (agent/exactkey.go).
	key := agent.ExactKey(toolName, args)
	if a.sessionAllow[toolName] || a.sessionAllow[key] {
		a.audit.record("allow", toolName, summary, "разрешено на эту задачу")
		return agent.ApproveOnce
	}

	fmt.Printf("  ? %s — спрашиваю разрешение в чате\n", toolName)
	answer, _, err := a.client.Ask(a.ctx, a.taskID, toolName, summary, string(args))
	if err != nil {
		// Спросить не вышло — отказ. Это безопасный исход: невозможность
		// задать вопрос не должна означать разрешение.
		a.audit.record("deny", toolName, summary, "не удалось спросить: "+err.Error())
		fmt.Printf("  ✗ %s: %v\n", toolName, err)
		return agent.ApproveDeny
	}

	switch answer {
	case inbox.AnswerOnce:
		a.audit.record("allow", toolName, summary, "ответ из чата: разово")
		return agent.ApproveOnce
	case inbox.AnswerSession:
		a.sessionAllow[toolName] = true
		a.audit.record("allow", toolName, summary, "ответ из чата: весь инструмент до конца задачи")
		return agent.ApproveOnce
	case inbox.AnswerExact:
		// Точно этот вызов: ключ tool+args, повтор той же команды в этой
		// задаче вопроса не поднимет, а любая другая — поднимет.
		a.sessionAllow[key] = true
		a.audit.record("allow", toolName, summary, "ответ из чата: эту команду до конца задачи")
		return agent.ApproveOnce
	case inbox.AnswerAlways:
		// Запись делает цикл: только он знает МАСШТАБ разрешения (весь
		// инструмент, каталог, домен или один секретный файл). Интерфейс,
		// записывавший здесь AddTool сам, превращал «навсегда» на один
		// каталог в доступ ко всей файловой системе.
		a.audit.record("allow", toolName, summary, "ответ из чата: всегда (запишется в permissions.json)")
		return agent.ApproveAlways
	default:
		a.audit.record("deny", toolName, summary, "ответ из чата: отказ")
		return agent.ApproveDeny
	}
}
