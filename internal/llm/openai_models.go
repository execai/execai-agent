package llm

// OpenAIModels — модели OpenAI Platform (platform.openai.com) — pay-per-token.
//
// Endpoint: https://api.openai.com/v1 (OpenAI-compat). Auth: Bearer sk-.
// Тарификация: pay-per-token (см. openai.com/api/pricing).
//
// Актуально на 2026-07. При /connect openai список также подтягивается
// с GET /v1/models — если сервер вернул расширенный набор, он перебивает hardcode
// через buildOpenAIDynamicCatalog.
func OpenAIModels() []Model {
	return []Model{
		{ID: "gpt-5", Provider: "openai", Name: "GPT-5", Description: "Флагман: топ рассуждение + агент, самая широкая экспертиза.", Tier: "flagship", IsPrimary: true, HasTools: true},
		{ID: "o3", Provider: "openai", Name: "o3", Description: "Reasoning-модель для сложных задач (math/coding/science).", Tier: "flagship", HasTools: true},
		{ID: "o4-mini", Provider: "openai", Name: "o4-mini", Description: "Дешёвая reasoning-модель, быстрая.", Tier: "standard", HasTools: true},
		{ID: "gpt-4.1", Provider: "openai", Name: "GPT-4.1", Description: "Улучшенный GPT-4 без reasoning; для кода/чата.", Tier: "standard", HasTools: true},
		{ID: "gpt-4o", Provider: "openai", Name: "GPT-4o", Description: "Multimodal флагман предыдущего поколения.", Tier: "standard", HasTools: true},
		{ID: "gpt-4o-mini", Provider: "openai", Name: "GPT-4o mini", Description: "Дешёвый и быстрый для простых задач.", Tier: "budget", HasTools: true},
	}
}
