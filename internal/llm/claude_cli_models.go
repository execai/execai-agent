package llm

// ClaudeCLIModels — models available via the local `claude` CLI.
// Passed as --model <id>. The CLI accepts short aliases (opus/sonnet/fable/haiku)
// and full anthropic-style IDs. Aliases are more convenient — the CLI maps
// them to the newest model it knows, so they move forward with `claude update`.
// Pinned IDs need a recent enough CLI (Opus 5.5 — 2.1.280+, Fable 5.1 — 2.1.251+);
// an older one answers with an error that names the required version.
//
// An empty string (Default) means "do not pass --model" → the model from
// `claude config` (defaultModel) or from the OAuth plan is used.
func ClaudeCLIModels() []Model {
	return []Model{
		{ID: "", Provider: "claude-cli", Name: "Default (из claude config)", Description: "Без --model — claude использует defaultModel из своего конфига", Tier: "standard", IsPrimary: true, HasTools: false},
		{ID: "opus", Provider: "claude-cli", Name: "Opus (alias)", Description: "Алиас — самый новый Opus, который знает твой claude", Tier: "flagship", HasTools: false},
		{ID: "sonnet", Provider: "claude-cli", Name: "Sonnet (alias)", Description: "Алиас — самый новый Sonnet, который знает твой claude", Tier: "flagship", HasTools: false},
		{ID: "fable", Provider: "claude-cli", Name: "Fable (alias)", Description: "Алиас — самый новый Fable (если есть в плане)", Tier: "flagship", HasTools: false},
		{ID: "haiku", Provider: "claude-cli", Name: "Haiku (alias)", Description: "Алиас — быстрая и дешёвая", Tier: "lite", HasTools: false},
		{ID: "claude-opus-5-5", Provider: "claude-cli", Name: "Claude Opus 5.5 (pinned)", Description: "Конкретная версия, нужен claude 2.1.280+", Tier: "flagship", HasTools: false},
		{ID: "claude-sonnet-5-5", Provider: "claude-cli", Name: "Claude Sonnet 5.5 (pinned)", Description: "Конкретная версия", Tier: "flagship", HasTools: false},
		{ID: "claude-fable-5-1", Provider: "claude-cli", Name: "Claude Fable 5.1 (pinned)", Description: "Конкретная версия, нужен claude 2.1.251+", Tier: "flagship", HasTools: false},
		{ID: "claude-haiku-4-5", Provider: "claude-cli", Name: "Claude Haiku 4.5 (pinned)", Description: "Конкретная версия", Tier: "lite", HasTools: false},
	}
}
