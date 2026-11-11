// Simple REPL: input → stream → print. No TUI. Used via --plain
// or automatically when stdout is not a TTY.
package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/velesbsdllc/agent-vbai/internal/auth"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
)

func RunPlain(ctx context.Context, cfg *config.Config) error {
	i18n.Setup(cfg.Locale)
	// The active source, as in the TUI: an own subscription works without a
	// sign-in or a plan; the ExecAI models check the plan on every turn.
	b, err := newPlainBackend(ctx, cfg, false)
	if err != nil {
		return err
	}
	models, current := b.models, b.current

	// The REPL survives Ctrl+C: SIGINT during streaming cancels only the current
	// request, the REPL keeps running. The parent ctx from main is canceled
	// by the signal and does not suit us — use background and catch signals ourselves.
	ctx = context.Background()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	// If the config has no model or names a nonexistent one — update the
	// config so the next launch starts with the same selection.
	if cfg.SelectedModelID != current.ID {
		cfg.SetAutoModel(current.ID) // picked here, not by the person
		_ = config.Save(cfg)
	}

	stream := b.open(current)

	fmt.Printf("execai chat (plain) | %s | %s/%s | %s\n", b.where, current.Provider, current.ID, b.who)
	fmt.Println(i18n.T("plain.commands"))

	history := make([]llm.Message, 0, 32)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "/quit" || line == "/exit" {
			return nil
		}
		if line == "/clear" || line == "/reset" {
			history = history[:0]
			fmt.Println(i18n.T("plain.historyCleared"))
			continue
		}
		if strings.HasPrefix(line, "/model") {
			arg := strings.TrimSpace(strings.TrimPrefix(line, "/model"))
			if arg == "" {
				printModels(models, current.ID)
				fmt.Println(i18n.T("plain.modelSwitchHint"))
				continue
			}
			next := pickByArg(models, arg)
			if next == nil {
				fmt.Println(i18n.Tf("plain.modelNotFound", arg))
				continue
			}
			current = *next
			cfg.ChooseModel(current.ID)
			_ = config.Save(cfg)
			stream = b.open(current)
			fmt.Println(i18n.Tf("plain.modelSwitched",
				current.Provider, current.ID, current.Name))
			continue
		}

		if b.gate != nil {
			if v := b.gate(ctx); !v.Allowed {
				fmt.Fprintln(os.Stderr, v.Message())
				continue
			}
		}

		history = append(history, llm.Message{Role: llm.RoleUser, Content: line})

		// Ctrl+C during streaming — cancel only this request, the REPL stays.
		// Listen to sigCh exactly for the duration of the request.
		// Drain beforehand — in case of a stale signal.
		select {
		case <-sigCh:
		default:
		}
		reqCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			select {
			case <-sigCh:
				cancel()
			case <-done:
			}
		}()

		full, err := stream(reqCtx, history, func(d string) { fmt.Print(d) })
		close(done)
		cancel()
		fmt.Println()
		if err != nil {
			history = history[:len(history)-1]
			if reqCtx.Err() == context.Canceled {
				fmt.Fprintln(os.Stderr, i18n.T("stream.aborted"))
			} else {
				fmt.Fprintln(os.Stderr, i18n.T("plain.errorPrefix"), err)
			}
			continue
		}
		history = append(history, llm.Message{Role: llm.RoleAssistant, Content: full})
	}
}

// Once — a one-shot request without the REPL, on the active source (as in
// the TUI). Takes the model from cfg.SelectedModelID (or the source's
// default), writes nothing to the config.
func Once(ctx context.Context, cfg *config.Config, prompt string) error {
	i18n.Setup(cfg.Locale)
	b, err := newPlainBackend(ctx, cfg, true)
	if err != nil {
		return err
	}
	_, err = b.open(b.current)(ctx, []llm.Message{{Role: llm.RoleUser, Content: prompt}},
		func(d string) { fmt.Print(d) })
	fmt.Println()
	return err
}

// requireLogin — the ExecAI models in run and the plain REPL need a
// sign-in. Missing credentials are explained in the gate's words; a broken
// credentials file keeps its own error.
func requireLogin() (*config.Credentials, error) {
	cr, err := auth.Require()
	if err != nil {
		if c, lerr := config.LoadCredentials(); lerr == nil && (c == nil || c.Token == "") {
			return nil, errors.New(plan.Verdict{Reason: plan.ReasonNotLoggedIn}.Message())
		}
	}
	return cr, err
}

func printModels(models []llm.Model, currentID string) {
	fmt.Println(i18n.T("plain.modelsHeader"))

	// Compute the maximum for nice alignment
	maxName, maxID := 0, 0
	for _, m := range models {
		if l := len(m.Name); l > maxName {
			maxName = l
		}
		if l := len(m.ID); l > maxID {
			maxID = l
		}
	}

	for i, m := range models {
		marker := " "
		if m.IsPrimary {
			marker = "★"
		}
		cur := " "
		if m.ID == currentID {
			cur = "•"
		}
		fmt.Printf("%s%s %2d. %-*s  %-*s  [%s/%s]\n",
			cur, marker, i+1, maxName, m.Name, maxID, m.ID, m.Provider, m.Tier)
	}
}

func pickByArg(models []llm.Model, arg string) *llm.Model {
	// First try as a list number (1-based).
	if n, err := strconv.Atoi(arg); err == nil {
		if n >= 1 && n <= len(models) {
			return &models[n-1]
		}
		return nil
	}
	// Otherwise — exact match by ID.
	for i := range models {
		if models[i].ID == arg {
			return &models[i]
		}
	}
	// Then by substring in ID or Name (case-insensitive).
	low := strings.ToLower(arg)
	for i := range models {
		if strings.Contains(strings.ToLower(models[i].ID), low) ||
			strings.Contains(strings.ToLower(models[i].Name), low) {
			return &models[i]
		}
	}
	return nil
}
