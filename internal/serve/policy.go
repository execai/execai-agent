// Политика выполнения инструментов в фоновом режиме и журнал выполненного.
//
// Задачу ставит пользователь, и ей мы доверяем. Но по пути агент читает файлы,
// issue, страницы через WebFetch — этому содержимому доверять нельзя. В чате
// человек увидел бы «Bash: curl … | sh» и отказал; в фоне такого момента нет.
//
// Поэтому решение за отсутствующего человека принимается по ЕГО ЖЕ выбору:
// ~/.config/execai/permissions.json, который он набивал в TUI кнопкой «Всегда».
// Сверяется с ним сам agent.Agent ДО обращения к approver'у, так что сюда
// доходит только то, чего в списке нет.
//
//	список непустой → отказ (пользователь свой выбор уже сделал);
//	список пустой   → разрешаем, но громко предупреждаем при старте
//	                  (иначе фоновый режим не заработал бы «из коробки»).
package serve

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
)

// policyApprover — решение об инструменте, когда рядом нет человека.
type policyApprover struct {
	// strict — у пользователя есть курированный список разрешений, значит
	// всё, что до нас дошло, в него не входит.
	strict bool
	audit  *auditLog
}

func (p policyApprover) AskApprove(toolName string, args json.RawMessage, summary string) agent.ApproveDecision {
	if !p.strict {
		p.audit.record("allow", toolName, summary, "список разрешений пуст")
		return agent.ApproveOnce
	}
	p.audit.record("deny", toolName, summary, "нет в permissions.json")
	return agent.ApproveDeny
}

// auditLog — что агент реально делал, пока никто не смотрел.
//
// Пишем всегда, независимо от политики: единственный способ узнать постфактум,
// что происходило ночью. Файл на месте истории — ротацию делает пользователь,
// объём строчный.
// maxAuditSize — при каком размере журнал уезжает в .1. Демон живёт неделями,
// а строка пишется на каждый вызов инструмента: без предела файл растёт, пока
// не кончится место.
const maxAuditSize = 8 << 20 // 8 МБ

type auditLog struct {
	mu   sync.Mutex
	f    *os.File
	path string
	size int64
	task string
}

func newAuditLog() *auditLog {
	base, err := os.UserConfigDir()
	if err != nil {
		return &auditLog{}
	}
	dir := filepath.Join(base, "execai")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return &auditLog{}
	}
	// 0600: в аргументах инструментов бывают пути и куски кода.
	path := filepath.Join(dir, "serve-audit.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return &auditLog{}
	}
	var size int64
	if st, err := f.Stat(); err == nil {
		size = st.Size()
	}
	return &auditLog{f: f, path: path, size: size}
}

// rotate переносит журнал в .1 и начинает новый. Держим одно поколение:
// это журнал для разбора «что было ночью», а не архив.
// Вызывается под уже взятым замком.
func (a *auditLog) rotate() {
	if a.f == nil || a.path == "" {
		return
	}
	_ = a.f.Close()
	// Ошибку переименования игнорируем осознанно: если не вышло, продолжим
	// писать в тот же файл — потерять журнал хуже, чем превысить размер.
	_ = os.Rename(a.path, a.path+".1")
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		a.f = nil
		return
	}
	a.f, a.size = f, 0
}

// AuditPath — куда пишется журнал; показываем при старте, чтобы человек знал,
// где смотреть.
func AuditPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "execai", "serve-audit.log")
}

func (a *auditLog) setTask(id string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.task = id
}

func (a *auditLog) record(verdict, tool, summary, reason string) {
	if a == nil || a.f == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	line := fmt.Sprintf("%s task=%s %s %s | %s",
		time.Now().Format(time.RFC3339), short(a.task), verdict, tool,
		strings.ReplaceAll(oneLine(summary), "\n", " "))
	if reason != "" {
		line += " (" + reason + ")"
	}
	if a.size >= maxAuditSize {
		a.rotate()
		if a.f == nil {
			return
		}
	}
	n, _ := fmt.Fprintln(a.f, line)
	a.size += int64(n)
}

func (a *auditLog) close() {
	if a == nil || a.f == nil {
		return
	}
	_ = a.f.Close()
}

// oneLine обрезает длинные аргументы: журнал должен читаться глазами.
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 200 {
		return string(r[:200]) + "…"
	}
	return string(r)
}
