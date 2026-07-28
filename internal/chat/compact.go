package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
)

// compactDoneMsg — асинхронный результат /compact.
type compactDoneMsg struct {
	summary string
	saved   int    // сколько сообщений было сжато
	err     error
}

// compactCmd — собирает старые сообщения, отправляет в LLM с запросом на summary.
// Сохраняет system + первое user-сообщение в начале, оставляет последние K сообщений.
const compactKeepTail = 6

func (m *tuiModel) compactCmd() func() interface{} {
	if m.cli == nil || len(m.history) <= compactKeepTail+2 {
		return func() interface{} {
			return compactDoneMsg{err: fmt.Errorf("история ещё короткая — нечего сжимать (нужно >%d сообщений)", compactKeepTail+2)}
		}
	}
	// Снимаем срез prefix → этот кусок идёт на summary.
	hist := append([]llm.AIMessage(nil), m.history...)
	prefix := hist[1 : len(hist)-compactKeepTail] // skip system + tail
	cli := m.cli
	return func() interface{} {
		summary, err := summarizeHistory(context.Background(), cli, prefix)
		if err != nil {
			return compactDoneMsg{err: err}
		}
		return compactDoneMsg{summary: summary, saved: len(prefix)}
	}
}

// summarizeHistory шлёт в aicore запрос: "сожми следующую беседу в краткую сводку".
// Возвращает текст summary (или ошибку).
func summarizeHistory(ctx context.Context, cli llm.StreamingLLM, messages []llm.AIMessage) (string, error) {
	// Собираем плоский transcript для prompt.
	var transcript strings.Builder
	for _, msg := range messages {
		role := msg.Role
		body := llm.ContentText(msg.Content)
		if body == "" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				body += "[tool_call] " + tc.Function.Name + " " + tc.Function.Arguments + "\n"
			}
		}
		if len(body) > 1500 {
			body = body[:1500] + "…(обрезано)"
		}
		fmt.Fprintf(&transcript, "[%s] %s\n", role, body)
	}

	prompt := []llm.AIMessage{
		{Role: "system", Content: "Ты сжиматель контекста для AI-агента. Тебе дают transcript беседы. " +
			"Верни КРАТКОЕ summary (≤500 слов) которое сохранит:\n" +
			"  • ключевые решения и причины\n" +
			"  • важные пути к файлам и команды\n" +
			"  • результаты tool-calls которые могут пригодиться дальше\n" +
			"  • ошибки и как они решены\n" +
			"Опусти болтовню и подтверждения. Пиши на русском, телеграфным стилем."},
		{Role: "user", Content: "Сожми эту беседу:\n\n" + transcript.String()},
	}

	// aicore требует tools: [] (не null) — иначе 422.
	res, err := cli.Stream(ctx, prompt, []map[string]any{}, llm.StreamCallbacks{})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Content), nil
}
