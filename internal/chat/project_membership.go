// Членство агента в проекте.
//
// Агент — такой же инструмент проекта, как ssh-профиль: запись в
// workspace_tools с tool_id="agent". Отдельной сущности мы не заводим
// намеренно — тогда бесплатно достаются и тумблер enabled, и отображение в
// карточке проекта, и единый способ «добавить в проект» для всего.
//
// В profile лежит agent_id, а не alias. auth-vbai переиспользует сессию по
// device_id, поэтому agent_id переживает повторный login, а alias
// пользователь может переименовать в настройках — привязка бы порвалась.
// Показываем при этом всё равно alias: uuid в интерфейсе бесполезен.
package chat

import (
	"context"
	"fmt"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
)

// agentToolID — tool_id, под которым агент живёт в workspace_tools.
// Должен совпадать с записью в каталоге integrations-vbai.
const agentToolID = inbox.AgentToolID

// workspaceTool — запись инструмента в проекте.
type workspaceTool = inbox.WorkspaceTool

// selfAgentID — id этой машины. Пустая строка означает, что мы залогинены не
// как агент (например, токеном из браузера) — тогда в проект добавлять нечего.
func (m *tuiModel) selfAgentID() string {
	if m.creds == nil {
		return ""
	}
	return m.creds.AgentID
}

// selfAlias — человекочитаемое имя машины для вывода.
func (m *tuiModel) selfAlias() string {
	if m.creds == nil {
		return ""
	}
	if m.creds.Alias != "" {
		return m.creds.Alias
	}
	return m.creds.AgentID
}

// findAgentTool ищет запись этого агента среди инструментов проекта.
// Второе значение — найдена ли: сама запись может быть выключена, и nil-проверки
// для этого недостаточно.
func findAgentTool(w workspace, agentID string) (workspaceTool, bool) {
	return inbox.FindAgentTool(w, agentID)
}

// addAgentToProject adds the agent to the project as a tool (idempotent:
// integrations-vbai does not deduplicate).
func (m *tuiModel) addAgentToProject(w workspace, agentID string) error {
	cl, err := m.projectClient()
	if err != nil {
		return err
	}
	return cl.AddAgent(context.Background(), w, agentID)
}

// removeAgentFromProject убирает агента из проекта. Отсутствие записи — не
// ошибка: пользователь мог удалить её из веб-интерфейса.
func (m *tuiModel) removeAgentFromProject(w workspace, agentID string) error {
	cl, err := m.projectClient()
	if err != nil {
		return err
	}
	return cl.RemoveAgent(context.Background(), w, agentID)
}

// setAgentEnabled включает или выключает агента в проекте — тот же тумблер,
// что и в веб-интерфейсе, потому что запись одна и та же.
func (m *tuiModel) setAgentEnabled(w workspace, agentID string, enabled bool) error {
	t, ok := findAgentTool(w, agentID)
	if !ok {
		return fmt.Errorf("%s", i18n.Tf("project.notInProject", w.Name))
	}
	if t.Enabled == enabled {
		return nil
	}
	cl, err := m.projectClient()
	if err != nil {
		return err
	}
	return cl.SetToolEnabled(context.Background(), w.ID, t.ID, enabled)
}
