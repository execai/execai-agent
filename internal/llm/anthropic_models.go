package llm

// AnthropicModels — модели доступные через настоящий Anthropic API
// (https://api.anthropic.com/v1/messages). Авторизация через API-key
// из console.anthropic.com (sk-ant-...). Billing — pay-per-token.
//
// Для Claude Pro/Max consumer-OAuth — отдельная история (TODO),
// сейчас работает только через API-key.
func AnthropicModels() []Model {
	return []Model{
		{ID: "claude-sonnet-4-6", Provider: "anthropic", Name: "Claude Sonnet 4.6", Description: "Sonnet flagship — баланс speed/quality, vision, 200K ctx", Tier: "flagship", IsPrimary: true, HasTools: true},
		{ID: "claude-opus-4-8", Provider: "anthropic", Name: "Claude Opus 4.8", Description: "Opus — топовая модель для сложного reasoning", Tier: "flagship", HasTools: true},
		{ID: "claude-haiku-4-5", Provider: "anthropic", Name: "Claude Haiku 4.5", Description: "Haiku — быстрая и дешёвая", Tier: "lite", HasTools: true},
		{ID: "claude-sonnet-4-5-20250929", Provider: "anthropic", Name: "Claude Sonnet 4.5 (pinned)", Description: "Конкретная версия sonnet 4.5", Tier: "standard", HasTools: true},
	}
}
