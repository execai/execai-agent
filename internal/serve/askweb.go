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
// Ответы и их смысл описаны ОДНИМ словарём на сервисе (agents-vbai
// internal/ask) — здесь только повторены константы, потому что тащить чужой
// модуль в CLI ради четырёх строк неоправданно. Значения обязаны совпадать:
// разъедутся — человек нажмёт «Разово», а получит «Всегда».
package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// Ответы. Должны совпадать с agents-vbai/internal/ask.
const (
	answerOnce    = "once"
	answerSession = "session" // весь инструмент до конца задачи
	answerExact   = "exact"   // точно этот вызов до конца задачи
	answerAlways  = "always"
	answerDeny    = "deny"
)

// webApprover спрашивает разрешение в веб-чате вместо немого отказа.
//
// Живёт на одну задачу: sessionAllow — это «Сессия» в терминах TUI, и
// логично, что она не переживает переход к следующей задаче из веба.
type webApprover struct {
	cfg    *config.Config
	token  string
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
		cfg: cfg, token: token, taskID: taskID, audit: audit,
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
	answer, err := a.ask(toolName, summary, string(args))
	if err != nil {
		// Спросить не вышло — отказ. Это безопасный исход: невозможность
		// задать вопрос не должна означать разрешение.
		a.audit.record("deny", toolName, summary, "не удалось спросить: "+err.Error())
		fmt.Printf("  ✗ %s: %v\n", toolName, err)
		return agent.ApproveDeny
	}

	switch answer {
	case answerOnce:
		a.audit.record("allow", toolName, summary, "ответ из чата: разово")
		return agent.ApproveOnce
	case answerSession:
		a.sessionAllow[toolName] = true
		a.audit.record("allow", toolName, summary, "ответ из чата: весь инструмент до конца задачи")
		return agent.ApproveOnce
	case answerExact:
		// Точно этот вызов: ключ tool+args, повтор той же команды в этой
		// задаче вопроса не поднимет, а любая другая — поднимет.
		a.sessionAllow[key] = true
		a.audit.record("allow", toolName, summary, "ответ из чата: эту команду до конца задачи")
		return agent.ApproveOnce
	case answerAlways:
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

// ask задаёт вопрос и ждёт ответа. Сервис держит запрос открытым, пока человек
// думает, поэтому таймаут клиента здесь заведомо больше серверного.
func (a *webApprover) ask(tool, summary, args string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"tool":    tool,
		"summary": summary,
		"args":    args,
	})
	data, err := apiRequestTimeout(a.ctx, a.cfg, a.token, http.MethodPost,
		"/agents-vbai/tasks/"+a.taskID+"/ask", body, 5*time.Minute)
	if err != nil {
		return "", err
	}
	var resp struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("не разобрать ответ: %w", err)
	}
	return resp.Answer, nil
}

// apiRequestTimeout — тот же запрос, что apiRequest, но со своим таймаутом:
// ожидание человека длиннее обычного вызова.
func apiRequestTimeout(ctx context.Context, cfg *config.Config, token, method, path string,
	body []byte, timeout time.Duration) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, trimBase(cfg.APIBase)+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, buf.String())
	}
	return buf.Bytes(), nil
}
