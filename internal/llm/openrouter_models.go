package llm

// OpenRouterModels — the built-in fallback catalog for OpenRouter
// (openrouter.ai) — one API key, every vendor, pay-per-token.
//
// Endpoint: https://openrouter.ai/api/v1 (OpenAI-compat). Auth: Bearer sk-or-…
//
// OpenRouter lists several hundred models, so this list is deliberately short:
// it only covers the case where GET /models could not be reached at /connect.
// The real catalog comes from the server and is sorted by internal/catalog.
func OpenRouterModels() []Model {
	return []Model{
		{ID: "anthropic/claude-sonnet-4.5", Provider: "openrouter", Name: "Claude Sonnet 4.5 · anthropic", Description: "Универсальный флагман Anthropic: код, агентские задачи, длинный контекст.", Tier: "flagship", IsPrimary: true, HasTools: true},
		{ID: "anthropic/claude-opus-4.1", Provider: "openrouter", Name: "Claude Opus 4.1 · anthropic", Description: "Самая сильная модель Anthropic для сложных задач.", Tier: "flagship", HasTools: true},
		{ID: "openai/gpt-5", Provider: "openrouter", Name: "GPT-5 · openai", Description: "Флагман OpenAI: рассуждение и широкая экспертиза.", Tier: "flagship", HasTools: true},
		{ID: "google/gemini-2.5-pro", Provider: "openrouter", Name: "Gemini 2.5 Pro · google", Description: "Флагман Google, очень длинный контекст.", Tier: "flagship", HasTools: true},
		{ID: "moonshotai/kimi-k2", Provider: "openrouter", Name: "Kimi K2 · moonshotai", Description: "Kimi для агентских задач и рефакторинга.", Tier: "flagship", HasTools: true},
		{ID: "z-ai/glm-4.6", Provider: "openrouter", Name: "GLM 4.6 · z-ai", Description: "GLM через OpenRouter — без ограничений Coding Plan.", Tier: "standard", HasTools: true},
		{ID: "deepseek/deepseek-chat", Provider: "openrouter", Name: "DeepSeek Chat · deepseek", Description: "Дешёвый и сильный в коде.", Tier: "standard", HasTools: true},
		{ID: "qwen/qwen3-coder", Provider: "openrouter", Name: "Qwen3 Coder · qwen", Description: "Открытая модель, заточенная под код.", Tier: "standard", HasTools: true},
	}
}
