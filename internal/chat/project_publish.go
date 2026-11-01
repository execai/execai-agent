// /project publish and the project half of sync. A project lives in two
// places: in the web console (a workspace of integrations-vbai) and in a
// directory on each machine that is bound to it. publish turns a local
// directory into a web project; bind takes a web project into a
// directory. Either way the project's .execai/ folder is exchanged at once
// when sync is on (internal/sync: one encrypted blob per project).
package chat

import (
	"context"
	"path/filepath"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/serve"
)

// bindHere binds dir to the project, puts this agent into the project and
// exchanges the project's .execai/. toolOK=false — bound, but the agent
// could not be added to the project (the message says so).
func (m *tuiModel) bindHere(target workspace, dir string) (msg string, toolOK bool, err error) {
	cl, err := m.projectClient()
	if err != nil {
		return "", false, err
	}
	if err := cl.Bind(context.Background(), target.ID, dir); err != nil {
		return "", false, err
	}
	msg, toolOK = i18n.Tf("project.bound", target.Name, dir), true
	// Привязка каталога — только наша половина дела. Вторая половина: агент
	// должен появиться в самом проекте, иначе в вебе его не видно и выключить
	// нельзя. Ошибку здесь не считаем фатальной — каталог уже привязан, и
	// молчать об этом хуже, чем сказать, что видна только половина.
	if agentID := m.selfAgentID(); agentID != "" {
		if err := m.addAgentToProject(target, agentID); err != nil {
			msg, toolOK = i18n.Tf("project.boundNoTool", target.Name, dir, err), false
		}
	}
	if svc := m.syncService(); svc != nil {
		if t := svc.SyncProject(context.Background(), target.ID, dir, target.Name); t != "" {
			msg += "\n" + t
		}
	}
	return msg, toolOK, nil
}

// projectPublish — /project publish [name]: a web project made from this
// directory (the name defaults to the directory's), bound here, its
// .execai/ sent at once. ask — offer to start the background listener, as
// /project bind does (the chat only).
func (m *tuiModel) projectPublish(name, dir string, ask bool) string {
	if name == "" {
		name = filepath.Base(dir)
	}
	cl, err := m.projectClient()
	if err != nil {
		return i18n.Tf("project.error", err)
	}
	ws, err := cl.CreateWorkspace(context.Background(), name)
	if err != nil {
		return i18n.Tf("project.error", err)
	}
	if ws.Name == "" {
		ws.Name = name
	}
	bound, toolOK, err := m.bindHere(ws, dir)
	if err != nil {
		return i18n.Tf("project.publishedNotBound", ws.Name, err)
	}
	msg := i18n.Tf("project.published", ws.Name) + "\n" + bound
	if !ask || !toolOK {
		return msg
	}
	if running, pid := serve.AlreadyRunning(); running {
		return msg + "\n" + i18n.Tf("project.serveAlready", pid)
	}
	m.askServeStart(msg)
	return ""
}

// PublishProject — execai project publish [name] from a shell: the same as
// /project publish in the chat, without the listener question.
func PublishProject(cfg *config.Config, name, dir string) string {
	cr, _ := config.LoadCredentials()
	m := &tuiModel{cfg: cfg, creds: cr}
	if svc := m.syncService(); svc != nil {
		svc.Cmd = "execai sync"
	}
	return m.projectPublish(name, dir, false)
}
