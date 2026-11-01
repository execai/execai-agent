// Команда /project — привязка агента к проекту (тот самый селектор «ПРОЕКТ»
// в веб-чате).
//
// Проект = workspace в integrations-vbai: набор инструментов с профилями.
// Мы его не дублируем, только читаем список и запоминаем, какой каталог на
// этой машине какому проекту соответствует. Это нужно, чтобы задача, пришедшая
// из веба вместе с workspace_id, выполнялась в правильном месте.
package chat

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	"github.com/velesbsdllc/agent-vbai/internal/serve"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// workspace is a project from integrations-vbai. The project API is shared
// with the editor panel and lives in internal/inbox.
type workspace = inbox.Workspace

// binding ties a project to a directory of this machine (agents-vbai).
type binding = inbox.Binding

func (m *tuiModel) handleProjectCommand(cmd string) string {
	arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/project"))
	switch {
	case arg == "" || arg == "list":
		return m.projectList()
	case strings.HasPrefix(arg, "bind"):
		return m.projectBind(strings.TrimSpace(strings.TrimPrefix(arg, "bind")))
	case arg == "publish" || strings.HasPrefix(arg, "publish "):
		cwd, _ := os.Getwd()
		return m.projectPublish(strings.TrimSpace(strings.TrimPrefix(arg, "publish")), cwd, true)
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
	bound, toolOK, err := m.bindHere(*target, cwd)
	if err != nil {
		return i18n.Tf("project.error", err)
	}
	if !toolOK {
		return bound
	}

	// Привязка без работающего слушателя бесполезна: задача из веба уйдёт в
	// очередь, а в чате будет «агент не отвечает». Предлагаем поднять его
	// сразу — но именно предлагаем: фоновый процесс, выполняющий инструменты
	// без подтверждения, не должен появляться молча.
	if running, pid := serve.AlreadyRunning(); running {
		return bound + "\n" + i18n.Tf("project.serveAlready", pid)
	}
	m.askServeStart(bound)
	return ""
}

// askServeStart предлагает запустить фоновый слушатель после привязки.
//
// Блокировать обработчик нельзя: он крутится в том же цикле bubbletea, который
// доставит ответ. Поэтому поднимаем пикер и вешаем отложенное действие — оно
// выполнится в replyAsk.
func (m *tuiModel) askServeStart(prefix string) {
	yes := i18n.T("project.serveYes")
	readOnly := i18n.T("project.serveReadOnly")
	m.asking = true
	m.askFocus = 0
	m.askQuestion = prefix + "\n\n" + i18n.T("project.serveQuestion")
	m.askOptions = []tools.AskOption{
		{Label: yes, Description: i18n.T("project.serveYesDesc")},
		{Label: readOnly, Description: i18n.T("project.serveReadOnlyDesc")},
		{Label: i18n.T("project.serveNo"), Description: i18n.T("project.serveNoDesc")},
	}
	m.askPending = func(answer string) string {
		var args []string
		switch answer {
		case yes:
		case readOnly:
			args = append(args, "--read-only")
		default:
			return i18n.T("project.serveSkipped")
		}
		pid, err := serve.Spawn(args...)
		if err != nil {
			return i18n.Tf("project.serveFailed", err)
		}
		out := i18n.Tf("project.serveStarted", pid)
		if p := serve.SpawnLogPath(); p != "" {
			out += "\n" + i18n.Tf("project.serveLog", p)
		}
		return out
	}
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
			cl, err := m.projectClient()
			if err != nil {
				return i18n.Tf("project.error", err)
			}
			if err := cl.Unbind(context.Background(), b.WorkspaceID); err != nil {
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
	cl, err := m.projectClient()
	if err != nil {
		return nil, err
	}
	return cl.Workspaces(context.Background())
}

func (m *tuiModel) fetchBindings() ([]binding, error) {
	cl, err := m.projectClient()
	if err != nil {
		return nil, err
	}
	return cl.Bindings(context.Background())
}

// projectClient is the projects-and-bindings client with the person's token.
func (m *tuiModel) projectClient() (*inbox.Client, error) {
	token := m.credsToken()
	if token == "" {
		return nil, fmt.Errorf("%s", i18n.T("project.needLogin"))
	}
	base := "https://api.execai.ru"
	if m.cfg != nil && m.cfg.APIBase != "" {
		base = m.cfg.APIBase
	}
	return inbox.NewClient(base, token), nil
}
