// Команда /project — привязка агента к проекту (тот самый селектор «ПРОЕКТ»
// в веб-чате).
//
// Проект = workspace в integrations-vbai: набор инструментов с профилями.
// Мы его не дублируем, только читаем список и запоминаем, какой каталог на
// этой машине какому проекту соответствует. Это нужно, чтобы задача, пришедшая
// из веба вместе с workspace_id, выполнялась в правильном месте.
package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/version"
)

// workspace — проект из integrations-vbai.
type workspace struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	Tools     []struct {
		// ID нужен, чтобы включать/выключать и удалять запись: PUT и DELETE
		// в integrations-vbai адресуются по id строки, а не по tool_id.
		ID      int    `json:"id"`
		ToolID  string `json:"tool_id"`
		Profile string `json:"profile"`
		Enabled bool   `json:"enabled"`
	} `json:"tools"`
}

// binding — привязка агента к проекту из agents-vbai.
type binding struct {
	AgentID     string `json:"agent_id"`
	WorkspaceID string `json:"workspace_id"`
	LocalPath   string `json:"local_path"`
}

func (m *tuiModel) handleProjectCommand(cmd string) string {
	arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/project"))
	switch {
	case arg == "" || arg == "list":
		return m.projectList()
	case strings.HasPrefix(arg, "bind"):
		return m.projectBind(strings.TrimSpace(strings.TrimPrefix(arg, "bind")))
	case arg == "unbind":
		return m.projectUnbind()
	case arg == "on":
		return m.projectToggle(true)
	case arg == "off":
		return m.projectToggle(false)
	default:
		return i18n.T("project.usage")
	}
}

// projectList показывает проекты пользователя и отмечает, какой привязан
// к текущему каталогу.
func (m *tuiModel) projectList() string {
	wss, err := m.fetchWorkspaces()
	if err != nil {
		return i18n.Tf("project.error", err)
	}
	if len(wss) == 0 {
		return i18n.T("project.none")
	}
	binds, _ := m.fetchBindings() // без привязок список всё равно полезен
	byWS := map[string]binding{}
	for _, b := range binds {
		byWS[b.WorkspaceID] = b
	}
	cwd, _ := os.Getwd()

	var b strings.Builder
	b.WriteString(i18n.T("project.listHeader") + "\n")
	for _, w := range wss {
		mark := "  "
		if bd, ok := byWS[w.ID]; ok {
			// Отмечаем именно текущий каталог: у агента может быть несколько
			// проектов, и важно, где мы находимся прямо сейчас.
			if bd.LocalPath == cwd {
				mark = "● "
			} else {
				mark = "○ "
			}
		}
		name := w.Name
		if w.IsDefault {
			name += " " + i18n.T("project.defaultTag")
		}
		// Состояние тумблера показываем прямо в списке: привязанный, но
		// выключенный агент выглядит как рабочий и молча не берёт задачи —
		// это первое, куда стоит посмотреть.
		if agentID := m.selfAgentID(); agentID != "" {
			if t, ok := findAgentTool(w, agentID); ok {
				if t.Enabled {
					name += " " + i18n.T("project.agentOn")
				} else {
					name += " " + i18n.T("project.agentOff")
				}
			}
		}
		fmt.Fprintf(&b, "%s%-16s %s\n", mark, name, toolsSummary(w))
		if bd, ok := byWS[w.ID]; ok && bd.LocalPath != "" {
			fmt.Fprintf(&b, "     ↳ %s\n", bd.LocalPath)
		}
	}
	b.WriteString("\n" + i18n.T("project.listHint"))
	return b.String()
}

func toolsSummary(w workspace) string {
	var names []string
	agents := 0
	for _, t := range w.Tools {
		if !t.Enabled {
			continue
		}
		// У агентов в profile лежит uuid — в списке он ничего не сообщает,
		// поэтому показываем только количество машин на проекте.
		//
		// Запись без профиля означает «инструмент добавлен, машина не
		// выбрана» — так делает веб-интерфейс при добавлении из каталога.
		// Считать её машиной нельзя: agent×1 без единой машины вводит в
		// заблуждение.
		if t.ToolID == agentToolID {
			if t.Profile != "" {
				agents++
			}
			continue
		}
		n := t.ToolID
		if t.Profile != "" {
			n += "/" + t.Profile
		}
		names = append(names, n)
	}
	if agents > 0 {
		names = append([]string{fmt.Sprintf("%s×%d", agentToolID, agents)}, names...)
	}
	if len(names) == 0 {
		return "—"
	}
	if len(names) > 5 {
		return strings.Join(names[:5], ", ") + fmt.Sprintf(" +%d", len(names)-5)
	}
	return strings.Join(names, ", ")
}

// projectBind привязывает текущий каталог к проекту по имени или id.
func (m *tuiModel) projectBind(nameOrID string) string {
	if nameOrID == "" {
		return i18n.T("project.usage")
	}
	wss, err := m.fetchWorkspaces()
	if err != nil {
		return i18n.Tf("project.error", err)
	}
	var target *workspace
	for i := range wss {
		if wss[i].ID == nameOrID || strings.EqualFold(wss[i].Name, nameOrID) {
			target = &wss[i]
			break
		}
	}
	if target == nil {
		var names []string
		for _, w := range wss {
			names = append(names, w.Name)
		}
		return i18n.Tf("project.notFound", nameOrID, strings.Join(names, ", "))
	}

	cwd, _ := os.Getwd()
	body, _ := json.Marshal(map[string]string{
		"workspace_id": target.ID,
		"local_path":   cwd,
	})
	if _, err := m.agentsAPI(http.MethodPost, "/workspaces/bind", body); err != nil {
		return i18n.Tf("project.error", err)
	}

	// Привязка каталога — только наша половина дела. Вторая половина: агент
	// должен появиться в самом проекте, иначе в вебе его не видно и выключить
	// нельзя. Ошибку здесь не считаем фатальной — каталог уже привязан, и
	// молчать об этом хуже, чем сказать, что видна только половина.
	if agentID := m.selfAgentID(); agentID != "" {
		if err := m.addAgentToProject(*target, agentID); err != nil {
			return i18n.Tf("project.boundNoTool", target.Name, cwd, err)
		}
	}
	return i18n.Tf("project.bound", target.Name, cwd)
}

// projectToggle включает или выключает этого агента в привязанном проекте.
func (m *tuiModel) projectToggle(enabled bool) string {
	agentID := m.selfAgentID()
	if agentID == "" {
		return i18n.T("project.notAgent")
	}
	cwd, _ := os.Getwd()
	binds, err := m.fetchBindings()
	if err != nil {
		return i18n.Tf("project.error", err)
	}
	var wsID string
	for _, b := range binds {
		if b.LocalPath == cwd {
			wsID = b.WorkspaceID
			break
		}
	}
	if wsID == "" {
		return i18n.Tf("project.notBound", cwd)
	}

	wss, err := m.fetchWorkspaces()
	if err != nil {
		return i18n.Tf("project.error", err)
	}
	for _, w := range wss {
		if w.ID != wsID {
			continue
		}
		if err := m.setAgentEnabled(w, agentID, enabled); err != nil {
			return i18n.Tf("project.error", err)
		}
		if enabled {
			return i18n.Tf("project.enabled", m.selfAlias(), w.Name)
		}
		return i18n.Tf("project.disabled", m.selfAlias(), w.Name)
	}
	return i18n.Tf("project.notBound", cwd)
}

// projectUnbind снимает привязку проекта с текущего каталога.
func (m *tuiModel) projectUnbind() string {
	cwd, _ := os.Getwd()
	binds, err := m.fetchBindings()
	if err != nil {
		return i18n.Tf("project.error", err)
	}
	for _, b := range binds {
		if b.LocalPath == cwd {
			body, _ := json.Marshal(map[string]string{"workspace_id": b.WorkspaceID})
			if _, err := m.agentsAPI(http.MethodPost, "/workspaces/unbind", body); err != nil {
				return i18n.Tf("project.error", err)
			}
			// Убираем и из состава проекта: иначе в вебе остался бы агент,
			// который никуда не привязан и на задачи не ответит.
			if agentID := m.selfAgentID(); agentID != "" {
				if wss, err := m.fetchWorkspaces(); err == nil {
					for _, w := range wss {
						if w.ID == b.WorkspaceID {
							_ = m.removeAgentFromProject(w, agentID)
							break
						}
					}
				}
			}
			return i18n.Tf("project.unbound", cwd)
		}
	}
	return i18n.Tf("project.notBound", cwd)
}

func (m *tuiModel) fetchWorkspaces() ([]workspace, error) {
	data, err := m.apiGet("/integrations-vbai/workspaces")
	if err != nil {
		return nil, err
	}
	var list []workspace
	if err := json.Unmarshal(data, &list); err == nil {
		return list, nil
	}
	// Часть версий отдаёт объектом.
	var wrapped struct {
		Workspaces []workspace `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("не разобрать список проектов: %w", err)
	}
	return wrapped.Workspaces, nil
}

func (m *tuiModel) fetchBindings() ([]binding, error) {
	data, err := m.apiGet("/agents-vbai/workspaces/bindings")
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		Bindings []binding `json:"bindings"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("не разобрать привязки: %w", err)
	}
	return wrapped.Bindings, nil
}

// agentsAPI — POST в agents-vbai через шлюз.
func (m *tuiModel) agentsAPI(method, path string, body []byte) ([]byte, error) {
	return m.apiRequest(method, "/agents-vbai"+path, body)
}

func (m *tuiModel) apiGet(path string) ([]byte, error) {
	return m.apiRequest(http.MethodGet, path, nil)
}

// apiRequest — запрос к нашему шлюзу с токеном пользователя.
func (m *tuiModel) apiRequest(method, path string, body []byte) ([]byte, error) {
	token := m.credsToken()
	if token == "" {
		return nil, fmt.Errorf("%s", i18n.T("project.needLogin"))
	}
	base := "https://api.execai.ru"
	if m.cfg != nil && m.cfg.APIBase != "" {
		base = strings.TrimRight(m.cfg.APIBase, "/")
	}
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", version.UserAgent())

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		data = append(data, buf[:n]...)
		if err != nil {
			break
		}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}
