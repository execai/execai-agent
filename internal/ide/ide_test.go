package ide

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
)

// Ответ из редактора обязан значить РОВНО то же, что кнопка TUI и веб-чата:
// словарь один (once/session/exact/always/deny), незнакомое — отказ.
func TestIdeApprover_Vocabulary(t *testing.T) {
	cases := []struct {
		answer string
		want   agent.ApproveDecision
	}{
		{"once", agent.ApproveOnce},
		{"session", agent.ApproveOnce}, // + запоминает инструмент, см. ниже
		{"exact", agent.ApproveOnce},   // + запоминает точный вызов
		{"deny", agent.ApproveDeny},
		{"", agent.ApproveDeny},
		{"мусор", agent.ApproveDeny},
	}
	for _, c := range cases {
		t.Run(c.answer, func(t *testing.T) {
			c0 := newTestChat()
			ap := &ideApprover{c: c0, sessionTools: map[string]bool{}, sessionExact: map[string]bool{}}
			go answerFirstAsk(c0.srv, c.answer)
			got := ap.AskApprove("Bash", json.RawMessage(`{"command":"ls"}`), "ls")
			if got != c.want {
				t.Errorf("ответ %q дал %v, ожидалось %v", c.answer, got, c.want)
			}
		})
	}
}

// «Весь инструмент в сессии» и «эту команду в сессии» живут ДО new_chat и
// не переспрашивают; description не рвёт точный ключ (канонизация).
func TestIdeApprover_SessionScopes(t *testing.T) {
	c := newTestChat()
	s := c.srv
	ap := &ideApprover{c: c, sessionTools: map[string]bool{}, sessionExact: map[string]bool{}}

	go answerFirstAsk(s, "exact")
	if d := ap.AskApprove("Bash", json.RawMessage(`{"command":"ls","description":"a"}`), "ls"); d != agent.ApproveOnce {
		t.Fatalf("exact: %v", d)
	}
	// Та же команда с другой подписью — БЕЗ вопроса (иначе тест повиснет:
	// отвечать некому).
	if d := ap.AskApprove("Bash", json.RawMessage(`{"command":"ls","description":"b"}`), "ls"); d != agent.ApproveOnce {
		t.Fatalf("повтор exact: %v", d)
	}

	go answerFirstAsk(s, "session")
	if d := ap.AskApprove("Write", json.RawMessage(`{"path":"x"}`), "x"); d != agent.ApproveOnce {
		t.Fatalf("session: %v", d)
	}
	if d := ap.AskApprove("Write", json.RawMessage(`{"path":"ДРУГОЙ"}`), "y"); d != agent.ApproveOnce {
		t.Fatalf("session покрывает весь инструмент: %v", d)
	}
}

// newTestChat — чат на сервере, пишущем в никуда.
func newTestChat() *chatSession {
	srv := newServer(json.NewEncoder(&bytes.Buffer{}), "")
	return srv.newChat("m", "p")
}

// answerFirstAsk ждёт появления вопроса и отвечает на него.
func answerFirstAsk(s *server, value string) {
	for i := 0; i < 200; i++ {
		s.answersMu.Lock()
		for id := range s.answers {
			s.answersMu.Unlock()
			s.deliver(id, value)
			return
		}
		s.answersMu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
}

// files_changed — только то, что реально легло на диск: отказ пользователя
// возвращается инструментом БЕЗ ошибки, и без проверки диска отклонённый
// Write выглядел бы как правка (поймано E2E 09.08).
func TestStreamer_FilesChangedIsFactual(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")

	st := &streamer{c: newTestChat(),
		changed: map[string]bool{}, turnStart: time.Now()}

	// Отклонённый Write: файл не появился → в changed его нет.
	st.OnToolCall("Write", json.RawMessage(`{"path":"`+jsonEscape(filepath.Join(dir, "denied.txt"))+`"}`))
	st.OnToolResult("Write", "Пользователь отклонил выполнение этого инструмента.", nil)

	// Реальный Write: файл существует и тронут в этом ходу.
	st.OnToolCall("Write", json.RawMessage(`{"path":"`+jsonEscape(real)+`"}`))
	if err := os.WriteFile(real, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	st.OnToolResult("Write", "создан", nil)

	got := st.changedList()
	if len(got) != 1 || got[0] != real {
		t.Errorf("changed = %v, ожидался только %s", got, real)
	}
}

// Контекст редактора приклеивается так, чтобы модель отличала слова человека
// от места, где он стоит; без контекста текст уходит нетронутым.
func TestBuildPrompt(t *testing.T) {
	if got := buildPrompt(In{Text: "привет"}); got != "привет" {
		t.Errorf("без контекста текст изменился: %q", got)
	}
	got := buildPrompt(In{Text: "поясни", Context: &EditorCtx{
		Path: "main.go", Language: "go", Selection: "func main() {}",
	}})
	for _, want := range []string{"поясни", "main.go", "go", "func main() {}", "контекст редактора"} {
		if !strings.Contains(got, want) {
			t.Errorf("в промте нет %q:\n%s", want, got)
		}
	}

	// Файлы кнопкой «+»: в промт уходят ПУТИ, не содержимое — агент в том же
	// каталоге и прочитает Read'ом ровно то, что нужно.
	got = buildPrompt(In{Text: "сравни", Context: &EditorCtx{Files: []string{"a.go", "b/c.ts"}}})
	for _, want := range []string{"сравни", "a.go", "b/c.ts", "Read"} {
		if !strings.Contains(got, want) {
			t.Errorf("в промте с файлами нет %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "```") {
		t.Error("файлы не должны раскрываться содержимым в промте")
	}

	// Картинки — в текст В КАВЫЧКАХ: их подхватит BuildUserContent и приложит
	// vision-блоками; обычные файлы остаются списком для Read.
	got = buildPrompt(In{Text: "глянь", Context: &EditorCtx{Files: []string{"shot.png", "a.go"}}})
	if !strings.Contains(got, `"shot.png"`) {
		t.Errorf("картинка не в кавычках — vision её не подхватит:\n%s", got)
	}
	if !strings.Contains(got, "a.go") || strings.Contains(got, `"a.go"`) {
		t.Errorf("обычный файл должен остаться списком без кавычек:\n%s", got)
	}
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

// Новый человек: плагин поставлен, вход ещё не сделан. LoadCredentials в этом
// случае отдаёт (nil, nil) — проверки одной ошибки мало, и Run падал паникой
// на первом же запуске (поймано прогоном 14.08 с чистым XDG_CONFIG_HOME).
func TestRun_NoCredentialsDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir) // на macOS конфиг ищется в HOME

	cr, err := config.LoadCredentials()
	if err != nil || cr != nil {
		t.Skipf("окружение отдало креды (%v, %v) — сценарий не воспроизводится", cr, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// stdin сразу закрыт: цикл обязан подняться и штатно выйти, а не упасть.
	old := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.Close()
	defer func() { os.Stdin = old }()

	done := make(chan error, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- fmt.Errorf("паника без кредов: %v", p)
			}
		}()
		done <- Run(ctx, config.Defaults(), Options{Cwd: dir})
	}()
	select {
	case err := <-done:
		if err != nil && strings.Contains(err.Error(), "паника") {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("Run не завершился на закрытом stdin")
	}
}

// Регрессия: finishTurn звал cancel() ДО проверки ctx.Err(), поэтому ЛЮБАЯ
// ошибка провайдера (неверный ключ, лимиты, кончился контекст, сеть)
// уезжала в панель как done:"stopped" — без текста. Пользователь видел
// молча остановившийся ход и не понимал, что случилось.
func TestFinishTurn_ProviderErrorIsReported(t *testing.T) {
	buf := &bytes.Buffer{}
	srv := newServer(json.NewEncoder(buf), "")
	c := srv.newChat("m", "p")
	ctx, cancel := context.WithCancel(context.Background())
	c.finishTurn(cancel, nil, fmt.Errorf("401 от Anthropic API — неверный ключ"), ctx,
		time.Now(), nil, turnDeps{})
	out := buf.String()
	if !strings.Contains(out, "неверный ключ") {
		t.Errorf("текст ошибки не дошёл до панели: %s", out)
	}
	if !strings.Contains(out, `"error"`) || strings.Contains(out, `"stopped"`) {
		t.Errorf("ошибка провайдера ушла как stopped: %s", out)
	}
}

// А настоящая остановка (человек нажал стоп) остаётся «stopped» — без
// пугающего текста ошибки.
func TestFinishTurn_UserStopIsStopped(t *testing.T) {
	buf := &bytes.Buffer{}
	srv := newServer(json.NewEncoder(buf), "")
	c := srv.newChat("m", "p")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // стоп пришёл ДО завершения хода
	c.finishTurn(cancel, nil, context.Canceled, ctx, time.Now(), nil, turnDeps{})
	out := buf.String()
	if !strings.Contains(out, `"stopped"`) {
		t.Errorf("остановка не помечена stopped: %s", out)
	}
	if strings.Contains(out, `"error"`) {
		t.Errorf("остановка человека показана как ошибка: %s", out)
	}
}

type fakeSummarizer struct{ reply string }

func (f *fakeSummarizer) Stream(_ context.Context, _ []llm.AIMessage, _ []map[string]any,
	_ llm.StreamCallbacks) (*llm.StreamResult, error) {
	return &llm.StreamResult{Content: f.reply}, nil
}

// В панели нет ручного /compact, поэтому за потолком окна следит сам ход:
// перевалили порог — история сжимается до system+пересказ+хвост; ниже
// порога — не трогается вовсе.
func TestMaybeAutoCompact_IDE(t *testing.T) {
	buf := &bytes.Buffer{}
	srv := newServer(json.NewEncoder(buf), "")
	c := srv.newChat("m", "p")
	deps := turnDeps{cli: &fakeSummarizer{reply: "пересказ"}, modelID: "claude-sonnet-4-6",
		autoCompactPct: 80}

	hist := []llm.AIMessage{{Role: "system", Content: "SYS"}}
	for i := 0; i < 12; i++ {
		hist = append(hist, llm.AIMessage{Role: "user", Content: "x"})
	}

	// 100k из 200k окна — ниже порога, история нетронута.
	if got := c.maybeAutoCompact(context.Background(), hist, deps, 100_000); len(got) != len(hist) {
		t.Errorf("ниже порога, но история изменилась: %d → %d", len(hist), len(got))
	}

	// 180k из 200k — за порогом, ждём сжатие.
	got := c.maybeAutoCompact(context.Background(), hist, deps, 180_000)
	if len(got) >= len(hist) {
		t.Fatalf("за порогом, но история не сжалась: %d → %d", len(hist), len(got))
	}
	if got[0].Role != "system" {
		t.Errorf("системное сообщение потеряно: %+v", got[0])
	}
	if !strings.Contains(llm.ContentText(got[1].Content), "пересказ") {
		t.Errorf("пересказ не вписан: %+v", got[1])
	}
	if !strings.Contains(buf.String(), "notice") {
		t.Error("панель не уведомлена о сжатии")
	}

	// Порог 0 = выключено.
	deps.autoCompactPct = 0
	if got := c.maybeAutoCompact(context.Background(), hist, deps, 999_999); len(got) != len(hist) {
		t.Error("автосжатие выключено, но сработало")
	}
}
