package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/velesbsdllc/agent-vbai/internal/auth"
	"github.com/velesbsdllc/agent-vbai/internal/chat"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/serve"
	// Blank import: registers all locales via init() → i18n.Register(...).
	_ "github.com/velesbsdllc/agent-vbai/internal/i18n/messages"
	// Aliased: the package name would collide with the build-time `version` var.
	agentver "github.com/velesbsdllc/agent-vbai/internal/version"
)

// version is overridden at build time via -ldflags "-X main.version=..."
var version = "dev"

const loginHelp = `Альфа-авторизация: вставьте JWT-токен из активной браузерной сессии ExecAI.

Где взять токен:
  1. Откройте https://execai.ru (или ваш веб-интерфейс) и залогиньтесь
     любым способом (Яндекс / ВК / magic-link / пароль).
  2. Откройте DevTools (F12) → вкладка Application/Storage → Cookies →
     домен execai.ru. Найдите cookie с токеном (обычно "auth_token" или
     "token"). Скопируйте его значение.
  3. Запустите команду login и вставьте токен в скрытый ввод.

Когда в auth-vbai появится Authorization Code Flow с PKCE и
loopback-redirect, эта команда будет открывать браузер автоматически.`

func main() {
	// Client identity for every upstream request (User-Agent). Must be set before
	// any HTTP call, otherwise providers see the "dev" build version.
	agentver.Set(version)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	root := &cobra.Command{
		Use:           "execai",
		Short:         "execai — CLI-агент экосистемы ExecAI/VBAI (как Claude Code)",
		SilenceUsage:  true,
		SilenceErrors: true,
		// With no arguments, open the TUI chat. Login/logout/config are slash
		// commands inside the TUI; the subcommands below remain for scripts (CI, curl|bash).
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if !term.IsTerminal(int(os.Stdout.Fd())) {
				return chat.RunPlain(cmd.Context(), cfg)
			}
			return chat.RunTUI(cmd.Context(), cfg, version)
		},
	}

	root.AddCommand(
		newLoginCmd(),
		newLogoutCmd(),
		newWhoamiCmd(),
		newChatCmd(),
		newRunCmd(),
		newServeCmd(),
		newConfigCmd(),
		newVersionCmd(),
	)

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newLoginCmd() *cobra.Command {
	var tokenFlag string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Сохранить JWT-токен (paste из браузерной сессии)",
		Long:  loginHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			token := strings.TrimSpace(tokenFlag)
			if token == "" {
				if !term.IsTerminal(int(os.Stdin.Fd())) {
					return errors.New("stdin не TTY — передайте токен через --token <jwt>")
				}
				fmt.Printf("Вставьте JWT-токен (ввод скрыт), API base: %s\n> ", cfg.APIBase)
				raw, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println()
				if err != nil {
					return err
				}
				token = strings.TrimSpace(string(raw))
			}

			cr, err := auth.Login(cmd.Context(), cfg, token)
			if err != nil {
				return err
			}
			fmt.Printf("OK: вошли как %s, токен сохранён.\n", cr.Email)
			return nil
		},
	}
	cmd.Flags().StringVar(&tokenFlag, "token", "", "JWT-токен напрямую (вместо интерактивного ввода)")
	return cmd
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Удалить сохранённый токен",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := auth.Logout(); err != nil {
				return err
			}
			fmt.Println("OK: токен удалён.")
			return nil
		},
	}
}

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Показать текущего пользователя (проверяет токен)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cr, err := auth.Require()
			if err != nil {
				return err
			}
			email, err := auth.Verify(cmd.Context(), cfg.APIBase, cr.Token)
			if err != nil {
				return err
			}
			fmt.Printf("%s @ %s\n", email, cfg.APIBase)
			return nil
		},
	}
}

func newChatCmd() *cobra.Command {
	var plain bool
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Интерактивный чат с агентом (TUI; --plain — простой REPL)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			// If stdout is not a TTY, the TUI is pointless (it cannot render
			// properly), so fall back to plain without the flag.
			if plain || !term.IsTerminal(int(os.Stdout.Fd())) {
				return chat.RunPlain(cmd.Context(), cfg)
			}
			return chat.RunTUI(cmd.Context(), cfg, version)
		},
	}
	cmd.Flags().BoolVar(&plain, "plain", false, "Простой REPL вместо TUI")
	return cmd
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run [prompt]",
		Short: "Однократный запрос к агенту",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return chat.Once(cmd.Context(), cfg, strings.Join(args, " "))
		},
	}
}

// newServeCmd — фоновый режим: агент слушает задачи из веб-чата.
//
// Отдельная команда, а не работа внутри TUI, потому что у инбокса должен быть
// ровно один владелец: иначе задачи делились бы между чатом и демоном
// случайным образом, и пользователь не понимал бы, куда ушла задача.
func newServeCmd() *cobra.Command {
	var pollWait, taskTimeout time.Duration
	var maxIter int
	var readOnly, showStatus, doStop, doForce bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Слушать задачи из веб-чата (фоновый режим)",
		Long: "Агент подключается к инбоксу и выполняет задачи, поставленные из веб-чата.\n\n" +
			"Задача выполняется в каталоге, привязанном к её проекту (execai → /project bind).\n\n" +
			"Что разрешено — берётся из твоего ~/.config/execai/permissions.json (то, что ты\n" +
			"отмечал в чате кнопкой «Всегда»). Остальное отклоняется: рядом нет человека,\n" +
			"который мог бы подтвердить. Если файл пуст — разрешено всё, и об этом будет\n" +
			"сказано при старте.\n\n" +
			"Всё выполненное пишется в ~/.config/execai/serve-audit.log.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Управление демоном не требует ни конфига, ни токена — это
			// операции над локальным процессом.
			if showStatus {
				running, err := serve.Status()
				if err != nil {
					return err
				}
				if !running {
					// Ненулевой код, чтобы скрипты могли на это опираться.
					os.Exit(1)
				}
				return nil
			}
			if doStop {
				return serve.Stop(30*time.Second, doForce)
			}
			if doForce {
				return errors.New("--force имеет смысл только вместе с --stop")
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			opts := serve.DefaultOptions()
			if pollWait > 0 {
				opts.PollWait = pollWait
			}
			if taskTimeout > 0 {
				opts.TaskTimeout = taskTimeout
			}
			if maxIter > 0 {
				opts.MaxIterations = maxIter
			}
			opts.ReadOnly = readOnly
			return serve.Run(cmd.Context(), cfg, opts)
		},
	}
	cmd.Flags().DurationVar(&pollWait, "poll-wait", 0, "длительность long-poll (по умолчанию 60s)")
	cmd.Flags().DurationVar(&taskTimeout, "task-timeout", 0, "предел на одну задачу (по умолчанию 5m)")
	cmd.Flags().IntVar(&maxIter, "max-iterations", 0, "предел итераций инструментов на задачу (по умолчанию 30)")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "только чтение: не менять файлы и не запускать команды")
	cmd.Flags().BoolVar(&showStatus, "status", false, "показать состояние демона и выйти")
	cmd.Flags().BoolVar(&doStop, "stop", false, "мягко остановить запущенный демон (ждёт завершения текущей задачи)")
	cmd.Flags().BoolVar(&doForce, "force", false, "с --stop: добить SIGKILL, если не вышел за 5с (результат задачи потеряется)")
	return cmd
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Показать или изменить конфиг",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "show",
			Short: "Показать текущий конфиг и пути",
			RunE: func(cmd *cobra.Command, args []string) error {
				dir, err := config.Dir()
				if err != nil {
					return err
				}
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				fmt.Printf("config dir       : %s\n", dir)
				fmt.Printf("api_base         : %s\n", cfg.APIBase)
				if cfg.SelectedModelID != "" {
					fmt.Printf("selected_model   : %s\n", cfg.SelectedModelID)
				} else {
					fmt.Println("selected_model   : (auto — DeepSeek или первая primary)")
				}
				cr, _ := config.LoadCredentials()
				if cr != nil {
					fmt.Printf("logged in        : %s (saved %s)\n", cr.Email, cr.SavedAt)
				} else {
					fmt.Println("logged in        : (not logged in)")
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "set [key=value...]",
			Short: "Поставить параметры: api_base, selected_model",
			Args:  cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				for _, kv := range args {
					parts := strings.SplitN(kv, "=", 2)
					if len(parts) != 2 {
						return fmt.Errorf("ожидается key=value, получено %q", kv)
					}
					switch parts[0] {
					case "api_base":
						cfg.APIBase = parts[1]
					case "selected_model", "model":
						cfg.SelectedModelID = parts[1]
					default:
						return fmt.Errorf("неизвестный ключ %q (доступно: api_base, selected_model)", parts[0])
					}
				}
				return config.Save(cfg)
			},
		},
	)
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Версия",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("execai", version)
		},
	}
}
