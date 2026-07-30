package llm

// ClaudeCLIModels — models available via the local `claude` CLI.
// Passed as --model <id>. The CLI accepts short aliases (sonnet/opus/haiku)
// and full anthropic-style IDs. Aliases are more convenient — Anthropic itself maps
// them to the current version in your account/plan.
//
// An empty string (Default) means "do not pass --model" → the model from
// `claude config` (defaultModel) or from the OAuth plan is used.
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
