// Команда /memory — импорт памяти других агентов и экспорт своей.
//
// Импорт всегда спрашивает подтверждение: содержимое чужих файлов уедет в
// память, а память потом синкается на сервер (пусть и зашифрованной). Человек
// должен понимать это ДО, а не после.
package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// handleMemoryCommand обрабатывает /memory, /memory import, /memory export [путь].
func (m *tuiModel) handleMemoryCommand(cmd string) string {
	arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/memory"))
	switch {
	case arg == "" || arg == "status":
		return m.memoryStatus()
	case arg == "import":
		return m.memoryImport()
	case arg == "find" || strings.HasPrefix(arg, "find "):
		return m.memoryFind(strings.TrimSpace(strings.TrimPrefix(arg, "find")))
	case arg == "export" || strings.HasPrefix(arg, "export "):
		target := strings.TrimSpace(strings.TrimPrefix(arg, "export"))
		return m.memoryExport(target)
	default:
		return i18n.T("memory.usage")
	}
}

// memoryStatus — что уже в памяти и что можно импортировать рядом.
func (m *tuiModel) memoryStatus() string {
	dir, err := agent.UserMemoryDir()
	if err != nil {
		return i18n.Tf("memory.error", err)
	}
	count := 0
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && e.Name() != "MEMORY.md" {
				count++
			}
		}
	}

	var b strings.Builder
	b.WriteString(i18n.Tf("memory.status", count, dir))

	// Заодно смотрим, есть ли рядом чужая память — самый частый случай, когда
	// человек не знает, что импорт вообще возможен.
	cwd, _ := os.Getwd()
	plan, _ := agent.PlanImport(agent.DiscoverAllMemorySources(cwd))
	if srcs := agent.Importable(plan); len(srcs) > 0 {
		b.WriteString("\n\n" + i18n.Tf("memory.foundNearby", len(srcs)) + "\n")
		for _, s := range srcs {
			fmt.Fprintf(&b, "  • %s (%s)\n", s.Rel, s.Label())
		}
		b.WriteString(i18n.T("memory.importHint"))
	}
	return b.String()
}

// memoryFind ищет по своей памяти, а если там пусто — по памяти других
// агентов на этой машине (Claude Code). Чужое не подмешивается к своему молча:
// найденное показывается отдельно и предлагается забрать к себе.
func (m *tuiModel) memoryFind(query string) string {
	if query == "" {
		return i18n.T("memory.findUsage")
	}
	mine, err := agent.SearchUserMemory(query)
	if err != nil {
		return i18n.Tf("memory.error", err)
	}
	var b strings.Builder
	if len(mine) > 0 {
		b.WriteString(i18n.Tf("memory.findMine", len(mine), query) + "\n")
		for _, h := range mine {
			fmt.Fprintf(&b, "  • %s — %s\n", h.Name, firstNonEmpty(h.Description, h.Line))
		}
		return b.String()
	}

	cwd, _ := os.Getwd()
	foreign := agent.SearchForeignMemory(cwd, query)
	if len(foreign) == 0 {
		return i18n.Tf("memory.findNothing", query)
	}
	b.WriteString(i18n.Tf("memory.findForeign", len(foreign), query) + "\n")
	srcs := make([]agent.ImportSource, 0, len(foreign))
	for _, h := range foreign {
		fmt.Fprintf(&b, "  • %s (%s) — %s\n", h.Name, h.Tool, firstNonEmpty(h.Description, h.Line))
		srcs = append(srcs, agent.ImportSourceFromPath(h.Path, h.Tool))
	}

	// Забирать чужое к себе — решение человека: импортированное станет частью
	// памяти, а она синкается. Поэтому пикер, а не автоматический импорт.
	yes := i18n.T("memory.importYes")
	m.asking = true
	m.askFocus = 0
	m.askQuestion = b.String() + "\n" + i18n.Tf("memory.findImportQuestion", len(foreign))
	m.askOptions = []tools.AskOption{
		{Label: yes, Description: i18n.T("memory.importYesDesc")},
		{Label: i18n.T("memory.importNo"), Description: i18n.T("memory.importNoDesc")},
	}
	m.askPending = func(answer string) string {
		if answer != yes {
			return i18n.T("memory.importCancelled")
		}
		return m.runImport(srcs)
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// memoryImport looks for other agents' files here and in the home folder,
// shows what goes where and asks before copying anything.
func (m *tuiModel) memoryImport() string {
	cwd, _ := os.Getwd()
	plan, err := agent.PlanImport(agent.DiscoverAllMemorySources(cwd))
	if err != nil {
		return i18n.Tf("memory.error", err)
	}
	if len(plan) == 0 {
		return i18n.Tf("memory.nothingToImport", cwd)
	}
	srcs := agent.Importable(plan)
	if len(srcs) == 0 {
		return agent.DescribeImportPlan(plan) + i18n.T("import.cmd.nothingNew")
	}

	// Подтверждение обязательно: импортированное станет частью памяти, а она
	// потом синкается на сервер (пусть и зашифрованной). Спрашиваем ДО.
	//
	// Блокировать здесь нельзя: обработчик команды крутится в том же цикле
	// bubbletea, который доставит ответ. Поэтому поднимаем пикер и вешаем
	// отложенное действие — оно выполнится в replyAsk.
	yes := i18n.T("memory.importYes")
	m.asking = true
	m.askFocus = 0
	m.askQuestion = i18n.Tf("memory.importQuestion", len(srcs)) + "\n\n" + agent.DescribeImportPlan(plan)
	m.askOptions = []tools.AskOption{
		{Label: yes, Description: i18n.T("memory.importYesDesc")},
		{Label: i18n.T("memory.importNo"), Description: i18n.T("memory.importNoDesc")},
	}
	m.askPending = func(answer string) string {
		if answer != yes {
			return i18n.T("memory.importCancelled")
		}
		return m.runImport(srcs)
	}
	return ""
}

// runImport copies the sources into memory, makes the new memory visible to
// the next turn and reports what happened.
func (m *tuiModel) runImport(srcs []agent.ImportSource) string {
	res, err := agent.ImportSources(srcs)
	if err != nil {
		return i18n.Tf("memory.error", err)
	}
	if len(res.Imported) > 0 {
		m.reloadMemory()
	}
	var b strings.Builder
	b.WriteString(i18n.Tf("memory.imported", len(res.Imported)))
	for _, n := range res.Imported {
		b.WriteString("\n  + " + n)
	}
	for _, s := range res.Skipped {
		b.WriteString("\n  ~ " + s)
	}
	return b.String()
}

// reloadMemory rebuilds the system prompt so memory imported mid-session is
// seen from the next turn on, not after a restart.
func (m *tuiModel) reloadMemory() {
	if m.registry == nil {
		return
	}
	cwd := getCWDForBoot()
	m.system = agent.SystemPrompt(cwd, m.registry.Names(), agent.LoadMemory(cwd))
}

// memoryExport выгружает память обычными markdown-файлами.
func (m *tuiModel) memoryExport(target string) string {
	if target == "" {
		cwd, _ := os.Getwd()
		target = filepath.Join(cwd, "execai-memory-export")
	}
	written, err := agent.ExportMemory(target)
	if err != nil {
		return i18n.Tf("memory.error", err)
	}
	return i18n.Tf("memory.exported", len(written), target)
}
