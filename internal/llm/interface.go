// Унифицированный интерфейс для LLM-клиентов: ExecAI gateway (AICoreClient)
// и внешние подписки (GLMClient, AnthropicClient, OpenAIClient).
//
// Agent держит ссылку на StreamingLLM, не зная откуда конкретно идут запросы.
// Переключение делается заменой Client при /use <provider> в TUI.
package llm

import "context"

// StreamingLLM — стандартный контракт LLM-провайдера для tool-use loop.
type StreamingLLM interface {
	Stream(ctx context.Context, messages []AIMessage, tools []map[string]any, cb StreamCallbacks) (*StreamResult, error)
}

// Убеждаемся что AICoreClient (наш базовый) реализует интерфейс.
var _ StreamingLLM = (*AICoreClient)(nil)
