package chat

import (
	"context"
	"fmt"

	"github.com/velesbsdllc/agent-vbai/internal/compact"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
)

// compactDoneMsg — asynchronous result of /compact.
type compactDoneMsg struct {
	summary string
	saved   int // how many messages were compacted
	cut     int // граница из compact.Cut — хвост пересобирается по ней же
	err     error
}

// compactKeepTail — сколько последних сообщений переживают сжатие.
const compactKeepTail = compact.KeepTail

// compactCmd — collects old messages, sends them to the LLM with a summary request.
// Preserves system + the summary at the start, keeps the last messages after
// the compact.Cut boundary (never splits a tool_call from its results).
func (m *tuiModel) compactCmd() func() interface{} {
	if m.cli == nil || len(m.history) <= compactKeepTail+2 {
		return func() interface{} {
			return compactDoneMsg{err: fmt.Errorf("%s", i18n.Tf("ui.compact.tooShort", compactKeepTail+2))}
		}
	}
	hist := append([]llm.AIMessage(nil), m.history...)
	cut := compact.Cut(hist, compactKeepTail)
	prefix := hist[1:cut] // skip system
	cli := m.cli
	return func() interface{} {
		summary, err := compact.Summarize(context.Background(), cli, prefix)
		if err != nil {
			return compactDoneMsg{err: err}
		}
		return compactDoneMsg{summary: summary, saved: len(prefix), cut: cut}
	}
}
