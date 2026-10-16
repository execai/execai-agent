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
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	// The text prompt only says " [image] "; the pictures themselves go as
	// files through --image, or codex never sees a screenshot.
	images, cleanup, err := writePromptImages(messages)
	if err != nil {
		return nil, fmt.Errorf("codex CLI: картинки: %w", err)
	}
	defer cleanup()
	args := []string{"exec"}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	for _, f := range images {
		args = append(args, "--image", f)
	}
	// `codex exec -` — read the prompt from stdin. `--` first: --image takes
	// several values and would swallow the "-" as one more file.
	args = append(args, "--", "-")
	cmd := exec.CommandContext(ctx, c.Path, args...)
	cmd.Stdin = strings.NewReader(prompt)
	stderr := &tailBuffer{max: 4096}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("codex CLI exit: %v (stderr: %s)", err, truncate(strings.TrimPrefix(stderr.suffix(), " — "), 400))
		}
		return nil, fmt.Errorf("codex CLI: %w", err)
	}
	content := strings.TrimSpace(string(out))
	if content == "" {
		// codex exits 0 on a failed login too (401 after its retries): the
		// reason is only in stderr.
		return nil, errors.New("codex CLI: пустой ответ" + stderr.suffix())
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

// writePromptImages saves the images of the conversation (the newest
// maxPromptImages) into a temporary directory for --image. cleanup removes
// it; it is safe to call when there were no images.
func writePromptImages(messages []AIMessage) (paths []string, cleanup func(), err error) {
	cleanup = func() {}
	var blocks []map[string]any
	for _, m := range messages {
		if m.Role != "user" {
			continue
		}
		for _, p := range contentParts(m.Content) {
			if p.image != nil {
				blocks = append(blocks, p.image)
			}
		}
	}
	if len(blocks) > maxPromptImages {
		blocks = blocks[len(blocks)-maxPromptImages:]
	}
	if len(blocks) == 0 {
		return nil, cleanup, nil
	}
	dir, err := os.MkdirTemp("", "execai-codex-img-")
	if err != nil {
		return nil, cleanup, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}
	for i, b := range blocks {
		src, _ := b["source"].(map[string]any)
		mt, _ := src["media_type"].(string)
		data, _ := src["data"].(string)
		raw, decErr := base64.StdEncoding.DecodeString(data)
		if decErr != nil {
			continue // a broken image must not cost the whole turn
		}
		e := ext[mt]
		if e == "" {
			e = ".png"
		}
		p := filepath.Join(dir, fmt.Sprintf("image-%02d%s", i+1, e))
		if err := os.WriteFile(p, raw, 0o600); err != nil {
			cleanup()
			return nil, func() {}, err
		}
		paths = append(paths, p)
	}
	return paths, cleanup, nil
}
