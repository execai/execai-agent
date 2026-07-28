package llm

// MoonshotModels — модели Moonshot Platform (platform.moonshot.ai) — pay-per-token
// API-ключ, отдельная система от Kimi Code (kimi.com/code) с подпиской.
//
// Endpoint: https://api.moonshot.ai/v1 (OpenAI-compat). Auth: Bearer sk-.
// Тарификация: pay-per-token (см. platform.moonshot.ai/pricing).
//
// Актуально на 2026-07. При /connect kimi-api список также подтягивается
// с GET /v1/models — если сервер вернул что-то другое, это перебивает hardcode.
func MoonshotModels() []Model {
	return []Model{
		{ID: "kimi-latest", Provider: "kimi-api", Name: "Kimi Latest", Description: "Автомат: сервер сам выбирает актуальную флагман-версию.", Tier: "flagship", IsPrimary: true, HasTools: true},
		{ID: "kimi-k2-turbo-preview", Provider: "kimi-api", Name: "Kimi K2 Turbo", Description: "K2 Turbo — быстрый флагман для чата.", Tier: "flagship", HasTools: true},
		{ID: "kimi-thinking-preview", Provider: "kimi-api", Name: "Kimi Thinking", Description: "Модель с reasoning-режимом (preview).", Tier: "flagship", HasTools: true},
		{ID: "moonshot-v1-auto", Provider: "kimi-api", Name: "Moonshot v1 Auto", Description: "Автовыбор длины контекста (8k/32k/128k).", Tier: "standard", HasTools: true},
		{ID: "moonshot-v1-128k", Provider: "kimi-api", Name: "Moonshot v1 128K", Description: "Base — 128K контекст, дешевле флагманов.", Tier: "standard", HasTools: true},
		{ID: "moonshot-v1-32k", Provider: "kimi-api", Name: "Moonshot v1 32K", Description: "Base — 32K контекст, минимальная цена.", Tier: "budget", HasTools: true},
	}
}
