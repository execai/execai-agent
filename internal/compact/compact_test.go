package compact

import (
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/llm"
)

func msg(role, body string) llm.AIMessage { return llm.AIMessage{Role: role, Content: body} }

func callMsg() llm.AIMessage {
	return llm.AIMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
		{ID: "t1", Function: llm.ToolCallFunc{Name: "Bash", Arguments: `{"command":"ls"}`}},
	}}
}

// Граница не должна рвать пару tool_call → tool_result: провайдеры отвергают
// историю, где ответ инструмента приходит без своего вызова.
func TestCut_NeverSplitsToolPairs(t *testing.T) {
	hist := []llm.AIMessage{
		msg("system", "s"),
		msg("user", "делай"),
		msg("assistant", "ок"),
		callMsg(),               // 3
		msg("tool", "result-1"), // 4
		msg("tool", "result-2"), // 5 ← наивный len-keepTail=2 указал бы сюда
		msg("assistant", "готово"),
		msg("user", "дальше"),
	}
	cut := Cut(hist, 3) // наивно 5 → внутри пачки tool-ответов
	if hist[cut].Role == "tool" {
		t.Fatalf("хвост начинается с tool-ответа без вызова: cut=%d", cut)
	}
	if cut != 3 {
		t.Errorf("ждали сдвиг к assistant с вызовами (3), получили %d", cut)
	}
}

func TestCut_ShortHistory(t *testing.T) {
	hist := []llm.AIMessage{msg("system", "s"), msg("user", "u")}
	if cut := Cut(hist, 6); cut != 1 {
		t.Errorf("короткая история: cut=%d, ждали 1 (резать нечего)", cut)
	}
}

// Rebuild сохраняет системное сообщение, кладёт пересказ и хвост по границе.
func TestRebuild(t *testing.T) {
	hist := []llm.AIMessage{
		msg("system", "SYS"),
		msg("user", "старое"),
		msg("assistant", "старый ответ"),
		msg("user", "свежее"),
		msg("assistant", "свежий ответ"),
	}
	out := Rebuild(hist, 3, "ПЕРЕСКАЗ", 2)
	if len(out) != 4 {
		t.Fatalf("ждали 4 сообщения (sys+summary+2 хвоста), получили %d", len(out))
	}
	if out[0].Role != "system" {
		t.Errorf("системное сообщение потеряно: %+v", out[0])
	}
	if !strings.Contains(llm.ContentText(out[1].Content), "ПЕРЕСКАЗ") {
		t.Errorf("пересказ не вписан: %+v", out[1])
	}
	if llm.ContentText(out[2].Content) != "свежее" {
		t.Errorf("хвост не по границе: %+v", out[2])
	}
}

// usage провайдера главнее оценки; без usage работает оценка по байтам.
func TestPercent(t *testing.T) {
	hist := []llm.AIMessage{msg("user", strings.Repeat("a", 400_000))} // ~100k токенов
	if p := Percent(100_000, hist, "claude-sonnet-4-6"); p != 50 {
		t.Errorf("по usage ждали 50%% от окна 200k, получили %d", p)
	}
	if p := Percent(0, hist, "claude-sonnet-4-6"); p < 45 || p > 55 {
		t.Errorf("оценка без usage сильно уехала: %d%%", p)
	}
}

func TestContextWindowFor(t *testing.T) {
	cases := map[string]int{
		"claude-sonnet-4-6": 200_000,
		"glm-5.2":           200_000,
		"kimi-k3":           256_000,
		"gpt-5":             400_000,
		"qwen3-coder:480b":  256_000,
		"llama3.1:8b":       128_000, // неизвестная — консервативный дефолт
	}
	for id, want := range cases {
		if got := ContextWindowFor(id); got != want {
			t.Errorf("%s: окно %d, ждали %d", id, got, want)
		}
	}
}
