package ide

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
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
			s := &session{enc: json.NewEncoder(&bytes.Buffer{}), answers: map[string]chan string{}}
			ap := &ideApprover{s: s, sessionTools: map[string]bool{}, sessionExact: map[string]bool{}}
			go answerFirstAsk(s, c.answer)
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
	s := &session{enc: json.NewEncoder(&bytes.Buffer{}), answers: map[string]chan string{}}
	ap := &ideApprover{s: s, sessionTools: map[string]bool{}, sessionExact: map[string]bool{}}

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

// answerFirstAsk ждёт появления вопроса и отвечает на него.
func answerFirstAsk(s *session, value string) {
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

	st := &streamer{s: &session{enc: json.NewEncoder(&bytes.Buffer{})},
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
