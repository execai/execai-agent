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
	"github.com/velesbsdllc/agent-vbai/internal/inbox"
	"github.com/velesbsdllc/agent-vbai/internal/serve"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

// bindHere binds dir to the project, puts this agent into the project and
// exchanges the project's .execai/. toolOK=false — bound, but the agent
// could not be added to the project (the message says so).
func (m *tuiModel) bindHere(target workspace, dir string) (msg string, toolOK bool, err error) {
	cl, err := m.projectClient()
	if err != nil {
		return "", false, err
	}
	if taken := m.folderTaken(cl, target.ID, target.Name, dir); taken != "" {
		return taken, false, nil
	}
	if err := cl.Bind(context.Background(), target.ID, dir); err != nil {
		return "", false, err
	}
	msg, toolOK = i18n.Tf("project.bound", target.Name, agentsync.HomePath(dir)), true
	// Привязка каталога — только наша половина дела. Вторая половина: агент
	// должен появиться в самом проекте, иначе в вебе его не видно и выключить
	// нельзя. Ошибку здесь не считаем фатальной — каталог уже привязан, и
	// молчать об этом хуже, чем сказать, что видна только половина.
	if agentID := m.selfAgentID(); agentID != "" {
		if err := m.addAgentToProject(target, agentID); err != nil {
			msg, toolOK = i18n.Tf("project.boundNoTool", target.Name, agentsync.HomePath(dir), err), false
		}
	}
	if svc := m.syncService(); svc != nil {
		if t := svc.SyncProject(context.Background(), target.ID, dir, target.Name); t != "" {
			msg += "\n" + t
		}
	}
	return msg, toolOK, nil
}

// folderTaken — why dir cannot be bound to the project targetID (name):
// another project is bound to that folder on this machine (one folder holds
// one project's memory); "" when it can. When the bindings cannot be read
// the binding goes ahead: sync leaves the second project of a folder out
// anyway (internal/sync, oneFolderOneProject).
func (m *tuiModel) folderTaken(cl *inbox.Client, targetID, name, dir string) string {
	agentID := m.selfAgentID()
	if agentID == "" {
		return ""
	}
	binds, err := cl.OwnBindings(context.Background(), agentID)
	if err != nil {
		return ""
	}
	other := inbox.TakenBy(binds, targetID, dir, agentsync.SameFolder)
	if other == "" {
		return ""
	}
	if wss, err := cl.Workspaces(context.Background()); err == nil {
		for _, w := range wss {
			if w.ID == other && w.Name != "" {
				other = w.Name
			}
		}
	}
	return i18n.Tf("project.folderTaken", other, name)
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
	if taken := m.folderTaken(cl, "", name, dir); taken != "" {
		return taken
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
	m.askServeStart()
	return msg
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
