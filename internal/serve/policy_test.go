package serve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
)

// Курированные разрешения проверяет сам agent.Agent ДО approver'а, поэтому
// сюда доходит только то, чего в permissions.json нет. Значит при непустом
// списке правильный ответ — отказ.
func TestPolicyApprover(t *testing.T) {
	cases := []struct {
		name   string
		strict bool
		want   agent.ApproveDecision
		why    string
	}{
		{"есть курированный список", true, agent.ApproveDeny,
			"пользователь свой выбор уже сделал, а этого инструмента там нет"},
		{"список пуст", false, agent.ApproveOnce,
			"иначе фоновый режим не заработал бы из коробки"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := policyApprover{strict: c.strict, audit: &auditLog{}}
			if got := p.AskApprove("Bash", json.RawMessage(`{"command":"rm -rf /"}`), "rm -rf /"); got != c.want {
				t.Errorf("решение %v, ожидалось %v — %s", got, c.want, c.why)
			}
		})
	}
}

// Журнал — единственный способ узнать постфактум, что агент делал ночью.
// Он обязан писаться независимо от политики.
func TestAuditLog_RecordsCallsAndVerdicts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	a := newAuditLog()
	if a.f == nil {
		t.Fatal("журнал не открылся")
	}
	a.setTask("task-1234-abcd")
	a.record("call", "Bash", `{"command":"go test ./..."}`, "")
	p := policyApprover{strict: true, audit: a}
	p.AskApprove("Write", json.RawMessage(`{"path":"/etc/passwd"}`), "Write /etc/passwd")
	a.close()

	data, err := os.ReadFile(filepath.Join(dir, "execai", "serve-audit.log"))
	if err != nil {
		t.Fatalf("журнал не найден: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "call Bash") || !strings.Contains(text, "go test") {
		t.Errorf("вызов не записан: %q", text)
	}
	if !strings.Contains(text, "deny Write") {
		t.Errorf("отказ не записан: %q", text)
	}
	// Без id задачи журнал бесполезен: непонятно, что чем вызвано.
	if !strings.Contains(text, "task=task-123") {
		t.Errorf("нет привязки к задаче: %q", text)
	}
	// Права: в аргументах бывают пути и куски кода.
	st, _ := os.Stat(filepath.Join(dir, "execai", "serve-audit.log"))
	if st.Mode().Perm() != 0o600 {
		t.Errorf("права журнала %v, ожидалось 0600", st.Mode().Perm())
	}
}

// Журнал не должен ронять демона, если каталог недоступен.
func TestAuditLog_NilSafe(t *testing.T) {
	var a *auditLog
	a.setTask("x")
	a.record("call", "Bash", "ls", "")
	a.close()
	empty := &auditLog{}
	empty.record("call", "Bash", "ls", "")
	// Дошли сюда без паники — достаточно.
}

// Длинные аргументы обрезаются по рунам: журнал читается глазами, а кириллица
// не должна рваться посередине.
func TestOneLine(t *testing.T) {
	long := strings.Repeat("я", 300)
	got := oneLine(long)
	if len([]rune(got)) != 201 {
		t.Errorf("длина %d рун, ожидалось 201 (200 + многоточие)", len([]rune(got)))
	}
	if strings.Contains(got, "�") {
		t.Error("обрезка сломала кодировку")
	}
}
