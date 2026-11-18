package chat

import (
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

func TestRenderAskBlock_KeepsDescriptions(t *testing.T) {
	for _, w := range []int{100, 80, 60, 0} {
		m := &tuiModel{
			width:       w,
			asking:      true,
			askQuestion: "Какой линтер подключить к проекту?",
			askOptions: []tools.AskOption{
				{Label: "golangci-lint", Description: "Агрегатор множества линтеров, конфигурация через .golangci.yml"},
				{Label: "staticcheck", Description: "Одиночный анализатор, быстрее и строже"},
			},
		}
		out := m.renderAskBlock()
		t.Logf("width=%d →\n%s", w, out)
		if !strings.Contains(out, "golangci-lint") {
			t.Errorf("width=%d: нет label", w)
		}
		// width=0 must fall back to 80, not collapse the description to one word.
		if !strings.Contains(out, "Агрегатор множества") {
			t.Errorf("width=%d: описание обрезано слишком рано", w)
		}
	}
}
