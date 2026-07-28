package llm

// KimiModels — модели Kimi Code (kimi.com/code) через api.kimi.com/coding
// (Anthropic-compat / OpenAI-compat). Актуально на 2026-07 по официальной
// доке www.kimi.com/code/docs/en/kimi-code/models.html.
//
// ВНИМАНИЕ по ID: сервер принимает ТОЛЬКО эти три ID; версии типа "kimi-k3"
// или "K2.7 Code" вызывают HTTP 400/401.
//
// Планы Kimi (от младшего к старшему, по темпу в музыке):
// Moderato → Allegretto → Allegro → Presto / Vivace.
//
//   * k3                          — Kimi K3, флагман. Доступен с Moderato.
//                                    Thinking обязателен (без thinking роутится на K2.6).
//   * kimi-for-coding             — Kimi K2.7 Code. Есть на всех планах.
//   * kimi-for-coding-highspeed   — HighSpeed вариант K2.7 Code (низкая
//                                    latency). Доступен от Allegretto.
//
// Reasoning effort: low / high / max (max = default).
func KimiModels() []Model {
	return []Model{
		{ID: "k3", Provider: "kimi", Name: "Kimi K3", Description: "Флагман Moonshot. Thinking включён. Доступен на всех планах Kimi Code кроме младших.", Tier: "flagship", IsPrimary: true, HasTools: true},
		{ID: "kimi-for-coding", Provider: "kimi", Name: "Kimi K2.7 Code", Description: "K2.7 Code — топ для рефакторинга и агентских задач. Есть на всех планах.", Tier: "flagship", HasTools: true},
		{ID: "kimi-for-coding-highspeed", Provider: "kimi", Name: "Kimi K2.7 HighSpeed", Description: "K2.7 Code со сниженной latency. Доступен от Allegretto и выше.", Tier: "standard", HasTools: true},
	}
}
