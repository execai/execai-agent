package chat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/tools"
)

// The hint under a question names the digits that pick an option: as many
// as there are options, at most 1-4 (the keys the window takes).
func TestAskHint_DigitRangeFollowsOptions(t *testing.T) {
	defer i18n.SetLocale("en")
	for _, loc := range []string{"en", "ru"} {
		i18n.SetLocale(loc)
		for n, want := range map[int]string{2: "1-2", 3: "1-3", 4: "1-4", 5: "1-4"} {
			m := &tuiModel{width: 100, asking: true, askQuestion: "?"}
			for i := 0; i < n; i++ {
				m.askOptions = append(m.askOptions, tools.AskOption{Label: fmt.Sprintf("option %d", i+1)})
			}
			out := m.renderAskBlock()
			hint := out[strings.LastIndex(out, "↑↓"):]
			if !strings.Contains(hint, want) {
				t.Errorf("%s, %d options: hint %q lacks %q", loc, n, hint, want)
			}
			for _, other := range []string{"1-2", "1-3", "1-4"} {
				if other != want && strings.Contains(hint, other) {
					t.Errorf("%s, %d options: hint %q says %q", loc, n, hint, other)
				}
			}
		}
	}
}
