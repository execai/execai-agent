// CodexCLIClient — делегирует запросы в локально установленный OpenAI Codex CLI
// (`codex` в PATH), используя OAuth-сессию ChatGPT Plus/Pro-подписки. Билинг —
// квота подписки юзера, наш агент в этом не участвует.
//
// codex CLI поддерживает `codex exec <prompt>` неинтерактивно + `--json` для
// stream-json вывода. Пока делаем простой захват stdout как единый content
// (без стриминга по токенам) — codex JSONL формат отличается от claude/anthropic
// и требует отдельного парсера. TODO: полноценный stream parser.
//
// Требования: у юзера должен быть залогинен `codex login` в ChatGPT-аккаунт.
// Установка codex CLI: https://github.com/openai/codex
package llm

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// CodexCLIClient — обёртка над локальным `codex` CLI.
type CodexCLIClient struct {
	Path  string
	Model string // optional --model
}

// NewCodexCLIClient — резолвит `codex` в PATH.
func NewCodexCLIClient(model string) (*CodexCLIClient, error) {
	path, err := exec.LookPath("codex")
	if err != nil {
		return nil, fmt.Errorf("codex CLI не найден в PATH. Установка: https://github.com/openai/codex\nПосле установки: codex login")
	}
	return &CodexCLIClient{Path: path, Model: model}, nil
}

var _ StreamingLLM = (*CodexCLIClient)(nil)

// Stream — вызывает `codex exec` неинтерактивно, читает stdout как ответ.
// Без токенного стриминга: codex exec отдаёт финальный текст, не JSONL.
// cb.OnText вызывается один раз с полным content.
func (c *CodexCLIClient) Stream(ctx context.Context, messages []AIMessage, tools []map[string]any, cb StreamCallbacks) (*StreamResult, error) {
	prompt := flattenMessagesToPrompt(messages)
	args := []string{"exec"}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	// `codex exec -` — читать промт из stdin.
	args = append(args, "-")
	cmd := exec.CommandContext(ctx, c.Path, args...)
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("codex CLI exit: %v (stderr: %s)", err, truncate(string(exitErr.Stderr), 400))
		}
		return nil, fmt.Errorf("codex CLI: %w", err)
	}
	content := strings.TrimSpace(string(out))
	if content == "" {
		return nil, errors.New("codex CLI: пустой ответ")
	}
	if cb.OnText != nil {
		cb.OnText(content)
	}
	logRequest(reqLogEntry{
		Source:         "codex-cli",
		BaseURL:        c.Path,
		ModelRequested: c.Model,
		Status:         "ok",
		ContentLen:     len(content),
	})
	return &StreamResult{
		Content:      content,
		FinishReason: "stop",
	}, nil
}

// CodexCLIModels — модели доступные через ChatGPT Plus/Pro-подписку.
// Точный список задаётся серверами OpenAI и обновляется автоматически при
// смене модели через `codex config` — здесь показываем базовые опции.
func CodexCLIModels() []Model {
	return []Model{
		{ID: "gpt-5", Provider: "codex-cli", Name: "GPT-5 (Codex)", Description: "Флагман через ChatGPT-подписку, роутится Codex CLI.", Tier: "flagship", IsPrimary: true, HasTools: false},
		{ID: "o3", Provider: "codex-cli", Name: "o3 (Codex)", Description: "Reasoning-модель через Codex CLI.", Tier: "flagship", HasTools: false},
		{ID: "o4-mini", Provider: "codex-cli", Name: "o4-mini (Codex)", Description: "Дешёвая reasoning-модель.", Tier: "standard", HasTools: false},
	}
}
