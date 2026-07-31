package llm

// KimiModels — Kimi Code models (kimi.com/code) via api.kimi.com/coding
// (Anthropic-compat / OpenAI-compat). Current as of 2026-07 per the official
// docs at www.kimi.com/code/docs/en/kimi-code/models.html.
//
// NOTE on IDs: the server accepts ONLY these three IDs; versions like "kimi-k3"
// or "K2.7 Code" cause HTTP 400/401.
//
// Kimi plans (from lowest to highest, by musical tempo):
// Moderato → Allegretto → Allegro → Presto / Vivace.
//
//   * k3                          — Kimi K3, the flagship. Available from Moderato.
//                                    Thinking is mandatory (without thinking it routes to K2.6).
//   * kimi-for-coding             — Kimi K2.7 Code. Available on all plans.
//   * kimi-for-coding-highspeed   — HighSpeed variant of K2.7 Code (low
//                                    latency). Available from Allegretto.
//
// Reasoning effort: low / high / max (max = default).
func KimiModels() []Model {
	return []Model{
		{ID: "k3", Provider: "kimi", Name: "Kimi K3", Description: "Флагман Moonshot. Thinking включён. Доступен на всех планах Kimi Code кроме младших.", Tier: "flagship", IsPrimary: true, HasTools: true},
		{ID: "kimi-for-coding", Provider: "kimi", Name: "Kimi K2.7 Code", Description: "K2.7 Code — топ для рефакторинга и агентских задач. Есть на всех планах.", Tier: "flagship", HasTools: true},
		{ID: "kimi-for-coding-highspeed", Provider: "kimi", Name: "Kimi K2.7 HighSpeed", Description: "K2.7 Code со сниженной latency. Доступен от Allegretto и выше.", Tier: "standard", HasTools: true},
	}
}
