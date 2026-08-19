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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// agentToolID — tool_id, под которым агент живёт в workspace_tools.
// Должен совпадать с записью в каталоге integrations-vbai.
const agentToolID = "agent"

// workspaceTool — запись инструмента в проекте.
type workspaceTool struct {
	ID      int    `json:"id"`
	ToolID  string `json:"tool_id"`
	Profile string `json:"profile"`
	Enabled bool   `json:"enabled"`
}

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
	for _, t := range w.Tools {
		if t.ToolID == agentToolID && t.Profile == agentID {
			return workspaceTool{ID: t.ID, ToolID: t.ToolID, Profile: t.Profile, Enabled: t.Enabled}, true
		}
	}
	return workspaceTool{}, false
}

// addAgentToProject добавляет агента в проект инструментом.
//
// Идемпотентность на нашей стороне: POST в integrations-vbai не проверяет
// дубликаты и на каждый вызов создаёт новую строку. Без этой проверки
// повторный /project bind оставлял бы в карточке проекта одного и того же
// агента столько раз, сколько его привязывали.
func (m *tuiModel) addAgentToProject(w workspace, agentID string) error {
	if _, ok := findAgentTool(w, agentID); ok {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"tool_id": agentToolID,
		"profile": agentID,
		"enabled": true,
	})
	_, err := m.apiRequest(http.MethodPost,
		"/integrations-vbai/workspaces/"+w.ID+"/tools", body)
	return err
}

// removeAgentFromProject убирает агента из проекта. Отсутствие записи — не
// ошибка: пользователь мог удалить её из веб-интерфейса.
func (m *tuiModel) removeAgentFromProject(w workspace, agentID string) error {
	t, ok := findAgentTool(w, agentID)
	if !ok {
		return nil
	}
	_, err := m.apiRequest(http.MethodDelete,
		fmt.Sprintf("/integrations-vbai/workspaces/%s/tools/%d", w.ID, t.ID), nil)
	return err
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
	body, _ := json.Marshal(map[string]any{"enabled": enabled})
	_, err := m.apiRequest(http.MethodPut,
		fmt.Sprintf("/integrations-vbai/workspaces/%s/tools/%d", w.ID, t.ID), body)
	return err
}
