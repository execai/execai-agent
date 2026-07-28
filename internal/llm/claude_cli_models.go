package llm

// ClaudeCLIModels — модели, доступные через локальный `claude` CLI.
// Передаются как --model <id>. CLI принимает короткие алиасы (sonnet/opus/haiku)
// и полные anthropic-style ID. Алиасы удобнее — Anthropic сам мапит их на
// актуальную версию в твоём аккаунте/плане.
//
// Пустая строка (Default) значит "не передавать --model" → используется
// модель из `claude config` (defaultModel) или из OAuth-плана.
func ClaudeCLIModels() []Model {
	return []Model{
		{ID: "", Provider: "claude-cli", Name: "Default (из claude config)", Description: "Без --model — claude использует defaultModel из своего конфига", Tier: "standard", IsPrimary: true, HasTools: false},
		{ID: "sonnet", Provider: "claude-cli", Name: "Sonnet (alias)", Description: "Алиас — claude сам выберет актуальный sonnet (4.6/4.5)", Tier: "flagship", HasTools: false},
		{ID: "opus", Provider: "claude-cli", Name: "Opus (alias)", Description: "Алиас — топовая модель для сложного reasoning", Tier: "flagship", HasTools: false},
		{ID: "haiku", Provider: "claude-cli", Name: "Haiku (alias)", Description: "Алиас — быстрая и дешёвая", Tier: "lite", HasTools: false},
		{ID: "claude-sonnet-4-6", Provider: "claude-cli", Name: "Claude Sonnet 4.6 (pinned)", Description: "Конкретная версия", Tier: "flagship", HasTools: false},
		{ID: "claude-opus-4-8", Provider: "claude-cli", Name: "Claude Opus 4.8 (pinned)", Description: "Конкретная версия", Tier: "flagship", HasTools: false},
		{ID: "claude-haiku-4-5", Provider: "claude-cli", Name: "Claude Haiku 4.5 (pinned)", Description: "Конкретная версия", Tier: "lite", HasTools: false},
	}
}
