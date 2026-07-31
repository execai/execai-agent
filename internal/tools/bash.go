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

// Read-only prefixes. This is an approximation: if the trimmed command starts
// with one of these tokens and does not contain rm/mv/cp/sed -i/>/>>/&&/||/;/$(/`,
// it is considered safe and the user is not asked.
var readOnlyPrefixes = []string{
	"ls", "ll", "la", "cat", "head", "tail", "less", "more",
	"pwd", "whoami", "id", "uname", "hostname", "uptime", "date",
	"echo", "printf",
	"grep", "egrep", "fgrep", "rg", "ripgrep", "ack",
	"find", "fd", "tree",
	"file", "stat", "wc", "du", "df",
	"ps", "top", "htop", "free",
	"env", "printenv",
	"which", "whereis", "type", "command",
	"git status", "git log", "git diff", "git show", "git branch", "git remote",
	"git config --get", "git rev-parse", "git ls-files", "git blame",
	"go version", "go env", "go list", "go vet", "go doc",
	"python --version", "python -V", "python3 --version", "python3 -V",
	"node --version", "npm --version", "yarn --version",
	"docker ps", "docker images", "docker version", "docker info",
	"kubectl get", "kubectl describe", "kubectl logs",
	"yc compute", "yc storage bucket get", "yc storage bucket list",
	"curl -I", "curl --head",
}

// danger — patterns that make a command unsafe even if its prefix is whitelisted.
var dangerPatterns = []string{
	"rm ", "rmdir", "unlink", "shred",
	"mv ", "cp -f", "ln ",
	"chmod ", "chown ",
	"sed -i", "perl -i",
	" >", " >>", " | tee ",
	" && ", " || ", "; ",
	"$(", "`", "exec ",
	"sudo ", "doas ", "su ",
	"shutdown", "reboot", "poweroff", "halt", "init ",
	"mkfs", "fdisk", "parted", "dd ",
	"systemctl ", "service ",
	"iptables", "nftables", "ufw ",
	"curl -X POST", "curl -X PUT", "curl -X DELETE", "curl -X PATCH",
	"wget ", "scp ", "rsync ",
}

func isReadOnlyCommand(cmd string) bool {
	t := strings.TrimSpace(cmd)
	if t == "" {
		return false
	}
	for _, d := range dangerPatterns {
		if strings.Contains(t, d) {
			return false
		}
	}
	for _, p := range readOnlyPrefixes {
		if t == p || strings.HasPrefix(t, p+" ") {
			return true
		}
	}
	return false
}

func (b *BashTool) RequiresApproval(args json.RawMessage) bool {
	var p struct{ Command string }
	_ = resolveJSON(args, &p)
	return !isReadOnlyCommand(p.Command)
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

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
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
