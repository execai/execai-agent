package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

func TestExtractPrompt(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{"обычный", `{"prompt":"покажи версию ОС"}`, "покажи версию ОС"},
		{"с пробелами", `{"prompt":"  собери проект  "}`, "собери проект"},
		{"лишние поля", `{"prompt":"тест","model":"k3"}`, "тест"},
		// payload может прийти голой строкой — это тоже задача, терять её нельзя.
		{"голая строка", `просто текст`, "просто текст"},
		{"пустой prompt", `{"prompt":""}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractPrompt(c.payload); got != c.want {
				t.Errorf("extractPrompt(%q) = %q, ожидалось %q", c.payload, got, c.want)
			}
		})
	}
}

// Задача выполняется в каталоге СВОЕГО проекта. Ошибиться здесь — значит
// выполнить «поправь тесты» не в том репозитории.
func TestWorkDirFor(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"bindings": []map[string]string{
			{"workspace_id": "ws-есть", "local_path": dir},
			{"workspace_id": "ws-пусто", "local_path": ""},
			{"workspace_id": "ws-переехал", "local_path": "/несуществующий/путь/xyz"},
		}})
	}))
	defer srv.Close()
	cfg := &config.Config{APIBase: srv.URL}

	got, err := workDirFor(context.Background(), cfg, "tok", "ws-есть")
	if err != nil || got != dir {
		t.Errorf("привязанный каталог: (%q, %v), ожидался %q", got, err, dir)
	}

	// Каталог мог переехать. Работать «где-нибудь» вместо него нельзя.
	if _, err := workDirFor(context.Background(), cfg, "tok", "ws-переехал"); err == nil {
		t.Error("несуществующий каталог не вызвал ошибку")
	}
	if _, err := workDirFor(context.Background(), cfg, "tok", "ws-пусто"); err == nil {
		t.Error("пустой local_path не вызвал ошибку")
	}
	if _, err := workDirFor(context.Background(), cfg, "tok", "ws-чужой"); err == nil {
		t.Error("непривязанный проект не вызвал ошибку")
	}

	// Задача вне проекта выполняется там, где запущен демон.
	cwd, _ := os.Getwd()
	got, err = workDirFor(context.Background(), cfg, "tok", "")
	if err != nil || got != cwd {
		t.Errorf("задача вне проекта: (%q, %v), ожидался текущий каталог %q", got, err, cwd)
	}
}

func TestPollInbox(t *testing.T) {
	var gotAuth, gotQuery, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotQuery, gotMethod = r.Header.Get("Authorization"), r.URL.RawQuery, r.Method
		_, _ = w.Write([]byte(`{"tasks":[{"id":"t1","workspace_id":"ws1","payload":"{\"prompt\":\"ok\"}"}]}`))
	}))
	defer srv.Close()

	tasks, err := pollInbox(context.Background(), &config.Config{APIBase: srv.URL}, "tok", 45*time.Second)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "t1" || tasks[0].WorkspaceID != "ws1" {
		t.Errorf("разобрано %+v", tasks)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("метод %s, ожидался POST", gotMethod)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	// Без wait сервер применит свой максимум, и агент будет опрашивать чаще,
	// чем нужно.
	if !strings.Contains(gotQuery, "wait=45") {
		t.Errorf("query = %q, ожидался wait=45", gotQuery)
	}
}

// Результат должен уходить даже когда основной контекст уже отменён (Ctrl+C):
// иначе в чате останется висеть таймаут вместо ответа.
func TestPostResult_SurvivesCancelledContext(t *testing.T) {
	done := make(chan map[string]string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["_path"] = r.URL.Path
		done <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // имитируем Ctrl+C

	postResult(ctx, &config.Config{APIBase: srv.URL}, "tok", "task-42", "final", "готово")

	select {
	case body := <-done:
		if body["chunk_type"] != "final" || body["data"] != "готово" {
			t.Errorf("тело запроса: %+v", body)
		}
		if body["_path"] != "/agents-vbai/tasks/task-42/result" {
			t.Errorf("путь %q", body["_path"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("результат не отправлен при отменённом контексте")
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("первая\nвторая"); got != "первая" {
		t.Errorf("firstLine = %q", got)
	}
	// Обрезка по рунам, а не по байтам: иначе кириллица рвётся посередине.
	long := strings.Repeat("я", 100)
	got := firstLine(long)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("длинная строка не обрезана: %q", got)
	}
	if len([]rune(got)) != 81 {
		t.Errorf("длина %d рун, ожидалось 81 (80 + многоточие)", len([]rune(got)))
	}
	if strings.Contains(got, "�") {
		t.Error("обрезка сломала кодировку")
	}
}

// Выдача из инбокса — предложение; выполнять можно только подтверждённое.
// Иначе задача, «выданная» в мёртвое соединение, выполнялась бы дважды.
func TestAckTasks_ExecutesOnlyConfirmed(t *testing.T) {
	var gotIDs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TaskIDs []string `json:"task_ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotIDs = req.TaskIDs
		// Сервер подтвердил только первую: вторую успел забрать таймаут/чужой ack.
		_ = json.NewEncoder(w).Encode(map[string]any{"acked": []string{"t1"}})
	}))
	defer srv.Close()

	got, err := ackTasks(context.Background(), &config.Config{APIBase: srv.URL}, "tok",
		[]Task{{ID: "t1"}, {ID: "t2"}})
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(gotIDs) != 2 {
		t.Errorf("на ack отправлено %v, ожидались оба id", gotIDs)
	}
	if len(got) != 1 || got[0].ID != "t1" {
		t.Errorf("к выполнению принято %v, ожидалась только подтверждённая t1", got)
	}
}

// Ack не прошёл (сеть) → не выполнять ничего: задачи остались pending и
// придут следующим poll'ом. Выполнение без подтверждения = риск дубля.
func TestAckTasks_NetworkFailureMeansNoTasks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := ackTasks(context.Background(), &config.Config{APIBase: srv.URL}, "tok",
		[]Task{{ID: "t1"}}); err == nil {
		t.Error("ошибка ack не всплыла — задачи выполнялись бы без подтверждения")
	}
}
