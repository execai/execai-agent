// Package llmpick — выбор LLM-клиента по активной подписке.
//
// Вынесено из TUI, чтобы фоновый режим работал по ТЕМ ЖЕ правилам, что и
// интерактив. Пока логика жила методом на модели TUI, `execai serve` всегда
// ходил в бэкенд ExecAI: человек работал локально на своей подписке Kimi или
// Z.ai, а задачи из веба молча тратили баланс ExecAI и могли идти другой
// моделью. «Агент действует по локальным настройкам» должно означать по всем,
// а не по трём из пяти.
//
// Регистрация в ExecAI при этом остаётся нужной — она даёт управление
// (веб-чат, маршрутизацию задач, реестр машин), а токены пользователь может
// приносить свои.
package llmpick

import (
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

// Client возвращает клиента для активной подписки, а если её нет — для
// бэкенда ExecAI.
//
// modelID/provider — выбранная модель (в подписках берётся её id, в ExecAI ещё
// и provider). token — JWT ExecAI, нужен только для fallback-пути.
func Client(cfg *config.Config, subs *subscriptions.Store, modelID, provider, token string) llm.StreamingLLM {
	if subs != nil {
		if active := subs.ActiveSubscription(); active != nil {
			switch active.Provider {
			case subscriptions.SourceZAI:
				// Ключ Z.ai Coding Plan работает ТОЛЬКО через Anthropic-совместимый
				// endpoint (как Claude Code). OpenAI /chat/completions с таким
				// ключом отдаёт 429 «Insufficient balance» — он тарифицируется
				// отдельно от подписки.
				base := active.BaseURL
				if base == "" {
					base = "https://api.z.ai/api/anthropic"
				}
				return llm.NewAnthropicClient(base, active.APIKey, modelID, cfg.ThinkingBudget)
			case subscriptions.SourceAnthropic:
				base := active.BaseURL
				if base == "" {
					base = "https://api.anthropic.com"
				}
				return llm.NewAnthropicClient(base, active.APIKey, modelID, cfg.ThinkingBudget)
			case subscriptions.SourceKimi:
				// Kimi Code Coding Plan: api.kimi.com/coding — Anthropic-compat + thinking.
				base := active.BaseURL
				if base == "" {
					base = "https://api.kimi.com/coding"
				}
				return llm.NewAnthropicClient(base, active.APIKey, modelID, cfg.ThinkingBudget)
			case subscriptions.SourceZAIAPI:
				// Открытая платформа Z.ai, pay-per-token (НЕ Coding Plan): этот
				// путь не ограничен списком одобренных инструментов.
				base := active.BaseURL
				if base == "" {
					base = "https://api.z.ai/api/paas/v4"
				}
				return llm.NewGLMClient(base, active.APIKey, modelID)
			case subscriptions.SourceKimiAPI:
				base := active.BaseURL
				if base == "" {
					base = "https://api.moonshot.ai/v1"
				}
				return llm.NewGLMClient(base, active.APIKey, modelID)
			case subscriptions.SourceOpenAI:
				base := active.BaseURL
				if base == "" {
					base = "https://api.openai.com/v1"
				}
				return llm.NewGLMClient(base, active.APIKey, modelID)
			case subscriptions.SourceOpenRouter:
				base := active.BaseURL
				if base == "" {
					base = "https://openrouter.ai/api/v1"
				}
				return llm.NewGLMClient(base, active.APIKey, modelID)
			case subscriptions.SourceCodexCLI:
				cli, err := llm.NewCodexCLIClient(modelID)
				if err != nil {
					return llm.NewAICoreClient(cfg.APIBase, token, modelID, provider)
				}
				return cli
			case subscriptions.SourceClaudeCLI:
				cli, err := llm.NewClaudeCLIClient(modelID)
				if err != nil {
					// Тихий fallback — недоступность показывает интерфейс.
					return llm.NewAICoreClient(cfg.APIBase, token, modelID, provider)
				}
				return cli
			case subscriptions.SourceOllama:
				base := active.BaseURL
				// Cloud (ollama.com) — Anthropic-совместимый endpoint, нужен ключ.
				// Local (localhost:11434) — OpenAI-compat, без ключа.
				if IsOllamaCloud(base, active.Plan) {
					if base == "" {
						base = "https://ollama.com"
					}
					return llm.NewAnthropicClient(base, active.APIKey, modelID, cfg.ThinkingBudget)
				}
				if base == "" {
					base = "http://localhost:11434"
				}
				return llm.NewOllamaClient(base, modelID)
			}
		}
	}
	return llm.NewAICoreClient(cfg.APIBase, token, modelID, provider)
}

// IsOllamaCloud отличает облачный ollama.com от локального сервера: у них
// разные протоколы и разные требования к ключу.
func IsOllamaCloud(baseURL, plan string) bool {
	if plan == "cloud" {
		return true
	}
	return strings.Contains(strings.ToLower(baseURL), "ollama.com")
}
