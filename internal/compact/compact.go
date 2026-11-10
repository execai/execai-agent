// Package compact — сжатие истории диалога, общее для TUI и IDE.
//
// Зачем отдельный пакет: логика «когда сжимать и по какой границе резать»
// обязана быть одна на все поверхности. Пока она жила внутри TUI, редактор
// не сжимал вовсе — длинный чат в панели тихо упирался в потолок контекста
// провайдера и умирал с ошибкой «prompt is too long».
package compact

import (
	"context"
	"fmt"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
)

// KeepTail — сколько последних сообщений переживают сжатие. Хвост — это
// живой рабочий контекст: модель продолжает начатое, а не вспоминает по
// пересказу.
const KeepTail = 6

// Cut возвращает индекс границы: history[1:cut] уходит в пересказ,
// history[cut:] остаётся как есть (history[0] — системное, не трогаем).
//
// Граница НЕ может рвать пару tool_call → tool_result: assistant-сообщение
// с вызовами и ответы инструментов обязаны остаться по одну сторону, иначе
// провайдер отвергает историю («tool message without preceding call»).
// Поэтому срез сдвигается влево, пока первый оставляемый элемент — ответ
// инструмента.
func Cut(history []llm.AIMessage, keepTail int) int {
	cut := len(history) - keepTail
	if cut < 1 {
		return 1
	}
	for cut > 1 && history[cut].Role == "tool" {
		cut--
	}
	// history[cut] теперь не tool, но если это assistant С вызовами, его
	// результаты уже в хвосте — самого его резать в пересказ нельзя.
	// Сдвиг выше это гарантирует: перед tool всегда стоит их assistant.
	return cut
}

// EstimateTokens — грубая оценка размера сообщений в токенах, когда провайдер
// не отдал usage. Байты/4: для кода и английского это ~честно, для кириллицы
// (2 байта на букву, ~2.5 буквы на токен) — небольшой перезаклад в безопасную
// сторону: сжать чуть раньше лучше, чем упереться в потолок.
func EstimateTokens(history []llm.AIMessage) int {
	total := 0
	for _, msg := range history {
		total += len(llm.ContentText(msg.Content))
		for _, tc := range msg.ToolCalls {
			total += len(tc.Function.Name) + len(tc.Function.Arguments)
		}
		total += 96 // обвязка сообщения: роль, разделители, служебные поля
	}
	return total / 4
}

// ContextWindowFor — окно контекста модели в токенах. Значения консервативные:
// лучше занизить и сжаться раньше, чем поверить в несуществующие токены.
// Каталоги провайдеров окно не отдают, поэтому таблица по подстрокам id.
func ContextWindowFor(modelID string) int {
	id := strings.ToLower(modelID)
	switch {
	case strings.Contains(id, "gpt-5"):
		return 400_000
	case strings.Contains(id, "claude"), strings.Contains(id, "sonnet"),
		strings.Contains(id, "opus"), strings.Contains(id, "haiku"):
		return 200_000
	case strings.Contains(id, "glm-5"), strings.Contains(id, "glm-4.6"):
		return 200_000
	case strings.Contains(id, "kimi"), strings.Contains(id, "k2"), strings.Contains(id, "k3"):
		return 256_000
	case strings.Contains(id, "qwen3-coder"):
		return 256_000
	case strings.Contains(id, "minimax"), strings.Contains(id, "m3"):
		return 200_000
	default:
		// Неизвестная модель (включая локальные ollama): 128k — типичный
		// потолок опенсорс-моделей последних поколений.
		return 128_000
	}
}

// Percent — заполненность окна: used токенов из окна модели, 0–100+.
// used=0 (usage не пришёл) — считаем по оценке истории.
func Percent(used int, history []llm.AIMessage, modelID string) int {
	if used <= 0 {
		used = EstimateTokens(history)
	}
	win := ContextWindowFor(modelID)
	if win <= 0 {
		return 0
	}
	return used * 100 / win
}

// Summarize просит модель пересказать messages. Возвращает текст пересказа.
func Summarize(ctx context.Context, cli llm.StreamingLLM, messages []llm.AIMessage) (string, error) {
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
			body = body[:1500] + i18n.T("ui.compact.truncated")
		}
		fmt.Fprintf(&transcript, "[%s] %s\n", role, body)
	}

	prompt := []llm.AIMessage{
		{Role: "system", Content: i18n.T("ui.compact.promptSystem")},
		{Role: "user", Content: i18n.Tf("ui.compact.promptUser", transcript.String())},
	}

	// aicore требует tools: [] (не null) — иначе 422.
	res, err := cli.Stream(ctx, prompt, []map[string]any{}, llm.StreamCallbacks{})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Content), nil
}

// Rebuild собирает новую историю: системное сообщение + пересказ + хвост.
// cut — граница из Cut() по ТОЙ ЖЕ истории.
func Rebuild(history []llm.AIMessage, cut int, summary string, saved int) []llm.AIMessage {
	out := make([]llm.AIMessage, 0, len(history)-cut+2)
	if len(history) > 0 && history[0].Role == "system" {
		out = append(out, history[0])
	}
	out = append(out, llm.AIMessage{
		Role:    "assistant",
		Content: i18n.Tf("ui.compact.historyNote", saved, summary),
	})
	if cut < len(history) {
		out = append(out, history[cut:]...)
	}
	return out
}
