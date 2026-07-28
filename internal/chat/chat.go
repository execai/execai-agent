// Простой REPL: ввод → стрим → печать. Без TUI. Используется через --plain
// или автоматически если stdout не TTY.
package chat

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/velesbsdllc/agent-vbai/internal/auth"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/llm"
)

func RunPlain(ctx context.Context, cfg *config.Config) error {
	cr, err := auth.Require()
	if err != nil {
		return err
	}

	// REPL переживает Ctrl+C: SIGINT во время стрима отменяет только текущий
	// запрос, REPL продолжает работать. Родительский ctx из main отменён
	// сигналом и нам не подходит — используем background и ловим сигналы сами.
	ctx = context.Background()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	models, err := llm.FetchModels(ctx, cfg.APIBase, cr.Token)
	if err != nil {
		return fmt.Errorf("не удалось получить список моделей: %w", err)
	}
	if len(models) == 0 {
		return fmt.Errorf("сервер вернул пустой список моделей")
	}

	current := llm.PickDefault(models, cfg.SelectedModelID)
	if current == nil {
		return fmt.Errorf("не удалось выбрать модель (список пустой?)")
	}
	// Если в конфиге модель отсутствует или указана несуществующая — обновим
	// конфиг, чтобы при следующем запуске стартовать с тем же выбором.
	if cfg.SelectedModelID != current.ID {
		cfg.SelectedModelID = current.ID
		_ = config.Save(cfg)
	}

	cli := llm.New(cfg.APIBase, cr.Token, current.ID, current.Provider)

	fmt.Printf("execai chat (plain) | %s | %s/%s | %s\n", cfg.APIBase, current.Provider, current.ID, cr.Email)
	fmt.Println("Команды: /model — выбрать модель, /clear — очистить историю, /quit — выход.")

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
			fmt.Println("(история очищена)")
			continue
		}
		if strings.HasPrefix(line, "/model") {
			arg := strings.TrimSpace(strings.TrimPrefix(line, "/model"))
			if arg == "" {
				printModels(models, current.ID)
				fmt.Println("\nПереключение: /model <номер> или /model <model_name>")
				continue
			}
			next := pickByArg(models, arg)
			if next == nil {
				fmt.Printf("(модель %q не найдена. /model — посмотреть список)\n", arg)
				continue
			}
			current = next
			cfg.SelectedModelID = current.ID
			_ = config.Save(cfg)
			cli = llm.New(cfg.APIBase, cr.Token, current.ID, current.Provider)
			fmt.Printf("(переключено на %s/%s — %s; история сохранена)\n",
				current.Provider, current.ID, current.Name)
			continue
		}

		history = append(history, llm.Message{Role: llm.RoleUser, Content: line})

		// Ctrl+C во время стрима — cancel только этого запроса, REPL остаётся.
		// Слушаем sigCh ровно на время запроса.
		// Drain заранее — на случай старого сигнала.
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

		full, err := cli.Stream(reqCtx, history, func(d string) { fmt.Print(d) })
		close(done)
		cancel()
		fmt.Println()
		if err != nil {
			history = history[:len(history)-1]
			if reqCtx.Err() == context.Canceled {
				fmt.Fprintln(os.Stderr, "(прервано)")
			} else {
				fmt.Fprintln(os.Stderr, "ошибка:", err)
			}
			continue
		}
		history = append(history, llm.Message{Role: llm.RoleAssistant, Content: full})
	}
}

// Once — однократный запрос без REPL. Берёт модель из cfg.SelectedModelID
// (или дефолт), ничего в конфиг не записывает.
func Once(ctx context.Context, cfg *config.Config, prompt string) error {
	cr, err := auth.Require()
	if err != nil {
		return err
	}
	models, err := llm.FetchModels(ctx, cfg.APIBase, cr.Token)
	if err != nil {
		return err
	}
	current := llm.PickDefault(models, cfg.SelectedModelID)
	if current == nil {
		return fmt.Errorf("не удалось выбрать модель")
	}
	cli := llm.New(cfg.APIBase, cr.Token, current.ID, current.Provider)

	_, err = cli.Stream(ctx, []llm.Message{{Role: llm.RoleUser, Content: prompt}},
		func(d string) { fmt.Print(d) })
	fmt.Println()
	return err
}

func printModels(models []llm.Model, currentID string) {
	fmt.Println("\nДоступные модели (★ — primary, • — current):")

	// Считаем максимум для красивого выравнивания
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
	// Сначала пробуем как номер в списке (1-based).
	if n, err := strconv.Atoi(arg); err == nil {
		if n >= 1 && n <= len(models) {
			return &models[n-1]
		}
		return nil
	}
	// Иначе — точное совпадение по ID.
	for i := range models {
		if models[i].ID == arg {
			return &models[i]
		}
	}
	// Затем по подстроке в ID или Name (case-insensitive).
	low := strings.ToLower(arg)
	for i := range models {
		if strings.Contains(strings.ToLower(models[i].ID), low) ||
			strings.Contains(strings.ToLower(models[i].Name), low) {
			return &models[i]
		}
	}
	return nil
}
