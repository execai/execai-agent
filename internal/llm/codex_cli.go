// CodexCLIClient delegates requests to a locally installed OpenAI Codex CLI
// (`codex` in PATH), using the OAuth session of a ChatGPT Plus/Pro subscription.
// Billing is the user's subscription quota; our agent is not involved.
//
// The codex CLI supports `codex exec <prompt>` non-interactively + `--json` for
// stream-json output. For now we do a simple stdout capture as a single content
// (no token streaming) — the codex JSONL format differs from claude/anthropic
// and needs a separate parser. TODO: full stream parser.
//
// Requirements: the user must be logged in via `codex login` to a ChatGPT account.
// Codex CLI installation: https://github.com/openai/codex
package llm

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// CodexCLIClient is a wrapper around the local `codex` CLI.
type CodexCLIClient struct {
	Path  string
	Model string // optional --model
}

// NewCodexCLIClient resolves `codex` in PATH.
func NewCodexCLIClient(model string) (*CodexCLIClient, error) {
	path, err := exec.LookPath("codex")
	if err != nil {
		return nil, fmt.Errorf("codex CLI не найден в PATH. Установка: https://github.com/openai/codex\nПосле установки: codex login")
	}
	return &CodexCLIClient{Path: path, Model: model}, nil
}

var _ StreamingLLM = (*CodexCLIClient)(nil)

// Stream calls `codex exec` non-interactively and reads stdout as the answer.
// No token streaming: codex exec returns the final text, not JSONL.
// cb.OnText is called once with the full content.
func (c *CodexCLIClient) Stream(ctx context.Context, messages []AIMessage, tools []map[string]any, cb StreamCallbacks) (*StreamResult, error) {
	prompt := flattenMessagesToPrompt(messages)
	args := []string{"exec"}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	// `codex exec -` — read the prompt from stdin.
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

// CodexCLIModels — models available via a ChatGPT Plus/Pro subscription.
// The exact list is defined by OpenAI's servers and updates automatically when
// switching models via `codex config` — here we show the basic options.
func CodexCLIModels() []Model {
	return []Model{
		{ID: "gpt-5", Provider: "codex-cli", Name: "GPT-5 (Codex)", Description: "Флагман через ChatGPT-подписку, роутится Codex CLI.", Tier: "flagship", IsPrimary: true, HasTools: false},
		{ID: "o3", Provider: "codex-cli", Name: "o3 (Codex)", Description: "Reasoning-модель через Codex CLI.", Tier: "flagship", HasTools: false},
		{ID: "o4-mini", Provider: "codex-cli", Name: "o4-mini (Codex)", Description: "Дешёвая reasoning-модель.", Tier: "standard", HasTools: false},
	}
}
