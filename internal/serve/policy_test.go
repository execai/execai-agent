package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/config"
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

// Ответ «Сессия» действует до конца задачи, но не дольше: следующая задача из
// веба — это новое решение человека.
func TestWebApprover_SessionScope(t *testing.T) {
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked++
		_ = json.NewEncoder(w).Encode(map[string]string{"answer": "session"})
	}))
	defer srv.Close()

	a := newWebApprover(context.Background(), &config.Config{APIBase: srv.URL},
		"tok", "task-1", nil, &auditLog{})

	if d := a.AskApprove("Bash", json.RawMessage(`{"command":"ls"}`), "ls"); d != agent.ApproveOnce {
		t.Fatalf("первый вызов: %v", d)
	}
	// Второй раз тот же инструмент — уже без вопроса.
	if d := a.AskApprove("Bash", json.RawMessage(`{"command":"pwd"}`), "pwd"); d != agent.ApproveOnce {
		t.Fatalf("второй вызов: %v", d)
	}
	if asked != 1 {
		t.Errorf("спросили %d раз, ожидался 1 — «Сессия» не запомнилась", asked)
	}
}

// «Эту команду» — узкое решение: повтор ТОЧНО такого вызова в этой задаче
// вопроса не поднимает, а другая команда того же инструмента — поднимает.
func TestWebApprover_ExactScope(t *testing.T) {
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked++
		_ = json.NewEncoder(w).Encode(map[string]string{"answer": "exact"})
	}))
	defer srv.Close()

	a := newWebApprover(context.Background(), &config.Config{APIBase: srv.URL},
		"tok", "task-1", nil, &auditLog{})

	ls := json.RawMessage(`{"command":"ls","description":"list files"}`)
	if d := a.AskApprove("Bash", ls, "ls"); d != agent.ApproveOnce {
		t.Fatalf("первый вызов: %v", d)
	}
	if d := a.AskApprove("Bash", ls, "ls"); d != agent.ApproveOnce {
		t.Fatalf("повтор той же команды: %v", d)
	}
	// Модель между повторами меняет подпись (поймано в WA13) — это ТА ЖЕ
	// команда, вопрос подниматься не должен.
	lsOther := json.RawMessage(`{"command":"ls","description":"list files (again)"}`)
	if d := a.AskApprove("Bash", lsOther, "ls"); d != agent.ApproveOnce {
		t.Fatalf("повтор с другой подписью: %v", d)
	}
	if asked != 1 {
		t.Errorf("спросили %d раз, ожидался 1 — точный вызов не запомнился", asked)
	}
	// Другая команда того же инструмента — новый вопрос (и новый ответ exact).
	if d := a.AskApprove("Bash", json.RawMessage(`{"command":"pwd"}`), "pwd"); d != agent.ApproveOnce {
		t.Fatalf("другая команда: %v", d)
	}
	if asked != 2 {
		t.Errorf("спросили %d раз, ожидалось 2 — «эту команду» не должно "+
			"открывать инструмент целиком", asked)
	}
}

// Не смогли спросить — отказ. Невозможность задать вопрос не должна означать
// разрешение.
func TestWebApprover_AskFailureIsDeny(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := newWebApprover(context.Background(), &config.Config{APIBase: srv.URL},
		"tok", "task-1", nil, &auditLog{})
	if d := a.AskApprove("Bash", json.RawMessage(`{}`), "rm -rf /"); d != agent.ApproveDeny {
		t.Errorf("решение %v, ожидался отказ", d)
	}
}

// Отказ и незнакомый ответ одинаково означают «не выполнять».
func TestWebApprover_DenyAndGarbage(t *testing.T) {
	for _, ans := range []string{"deny", "", "ЧТО-ТО"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"answer": ans})
		}))
		a := newWebApprover(context.Background(), &config.Config{APIBase: srv.URL},
			"tok", "t", nil, &auditLog{})
		if d := a.AskApprove("Write", json.RawMessage(`{}`), "x"); d != agent.ApproveDeny {
			t.Errorf("ответ %q дал %v, ожидался отказ", ans, d)
		}
		srv.Close()
	}
}

// «Всегда» обязано попасть в permissions.json — иначе следующий запуск
// спросит снова и кнопка окажется обманом.
func TestWebApprover_AlwaysPersists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"answer": "always"})
	}))
	defer srv.Close()

	perms, err := agent.LoadPermissions()
	if err != nil {
		t.Fatal(err)
	}
	a := newWebApprover(context.Background(), &config.Config{APIBase: srv.URL},
		"tok", "t", perms, &auditLog{})
	if d := a.AskApprove("Bash", json.RawMessage(`{"command":"go test"}`), "go test"); d != agent.ApproveOnce {
		t.Fatalf("решение %v", d)
	}
	reloaded, err := agent.LoadPermissions()
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.HasTool("Bash") {
		t.Error("«Всегда» не записалось на диск — следующий запуск спросит снова")
	}
}
