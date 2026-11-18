// syscheck is an e2e smoke check of all 4 sources. NOT part of the main
// build, run manually: `go run ./cmd/syscheck`. Sends real requests
// (1 per source). Uses the current ~/.config/execai/.
//
// The goal is to catch a regression like BUG-1 (returning to ExecAI with a
// foreign provider → 401) without the TUI and without manual clicking.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/subscriptions"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	creds, err := config.LoadCredentials()
	if err != nil {
		return fmt.Errorf("нет creds — сначала execai login: %w", err)
	}
	subs, err := subscriptions.Load()
	if err != nil {
		return err
	}

	prompt := "Скажи одно слово: 'OK'."
	msgs := []llm.AIMessage{{Role: "user", Content: prompt}}

	// 1. ExecAI — take the real model+provider from the billing catalog,
	// otherwise the gateway rejects empty ones.
	models, mErr := llm.FetchModels(ctx, cfg.APIBase, creds.Token)
	if mErr != nil {
		fmt.Printf("✗ execai (fetch models): %v\n", mErr)
	} else {
		primary := models[0]
		for _, m := range models {
			if m.IsPrimary {
				primary = m
				break
			}
		}
		check(ctx, fmt.Sprintf("execai (%s)", primary.ID), func() llm.StreamingLLM {
			return llm.NewAICoreClient(cfg.APIBase, creds.Token, primary.ID, primary.Provider)
		}, msgs)
	}

	// 2. Z.ai (if connected)
	if z, ok := subs.Subscriptions["zai"]; ok {
		check(ctx, "zai (GLM-5.2 via anthropic-compat)", func() llm.StreamingLLM {
			return llm.NewAnthropicClient("https://api.z.ai/api/anthropic", z.APIKey, "glm-5.2", 0)
		}, msgs)
	} else {
		fmt.Println("⚠ zai: не подключен, пропуск")
	}

	// 3. Anthropic (if connected)
	if a, ok := subs.Subscriptions["anthropic"]; ok {
		check(ctx, "anthropic (Claude Sonnet 4.6)", func() llm.StreamingLLM {
			return llm.NewAnthropicClient("https://api.anthropic.com", a.APIKey, "claude-sonnet-4-6", 0)
		}, msgs)
	} else {
		fmt.Println("⚠ anthropic: не подключен, пропуск")
	}

	// 4. Claude CLI (if the binary exists)
	if _, ok := subs.Subscriptions["claude-cli"]; ok {
		check(ctx, "claude-cli (default model)", func() llm.StreamingLLM {
			cli, err := llm.NewClaudeCLIClient("")
			if err != nil {
				fmt.Printf("✗ claude-cli: %v\n", err)
				return nil
			}
			return cli
		}, msgs)
	} else {
		fmt.Println("⚠ claude-cli: не подключен, пропуск")
	}

	return nil
}

func check(ctx context.Context, label string, make func() llm.StreamingLLM, msgs []llm.AIMessage) {
	cli := make()
	if cli == nil {
		return
	}
	var buf strings.Builder
	cb := llm.StreamCallbacks{OnText: func(s string) { buf.WriteString(s) }}
	start := time.Now()
	_, err := cli.Stream(ctx, msgs, nil, cb)
	dur := time.Since(start)
	out := strings.TrimSpace(buf.String())
	if len(out) > 60 {
		out = out[:60] + "..."
	}
	if err != nil {
		fmt.Printf("✗ %s [%v]: %v | got=%q\n", label, dur.Round(time.Millisecond), err, out)
		return
	}
	fmt.Printf("✓ %s [%v]: %q\n", label, dur.Round(time.Millisecond), out)
}
