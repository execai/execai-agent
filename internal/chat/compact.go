package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
)

// compactDoneMsg — asynchronous result of /compact.
type compactDoneMsg struct {
	summary string
	saved   int    // how many messages were compacted
	err     error
}

// compactCmd — collects old messages, sends them to the LLM with a summary request.
// Preserves system + the first user message at the start, keeps the last K messages.
const compactKeepTail = 6

func (m *tuiModel) compactCmd() func() interface{} {
	if m.cli == nil || len(m.history) <= compactKeepTail+2 {
		return func() interface{} {
			return compactDoneMsg{err: fmt.Errorf("история ещё короткая — нечего сжимать (нужно >%d сообщений)", compactKeepTail+2)}
		}
	}
	// Take the prefix slice → this chunk goes to the summary.
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

// summarizeHistory sends a request to aicore: "compress the following conversation into a brief summary".
// Returns the summary text (or an error).
func summarizeHistory(ctx context.Context, cli llm.StreamingLLM, messages []llm.AIMessage) (string, error) {
	// Build a flat transcript for the prompt.
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

	// aicore requires tools: [] (not null) — otherwise 422.
	res, err := cli.Stream(ctx, prompt, []map[string]any{}, llm.StreamCallbacks{})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Content), nil
}
