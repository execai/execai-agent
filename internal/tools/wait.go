// wait — blocking pause inside a turn: "give it N seconds" or "poll until this
// command succeeds". The model used to *say* «I'll check in five minutes» and
// end the turn; nothing checked anything. schedule_wakeup is the other half
// of the answer (release the chat, come back later); this one keeps the turn
// and its context alive for the short waits a deploy or a CI run needs.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// WaitTool sleeps, optionally re-running a shell check until it passes.
type WaitTool struct{ cwd string }

const (
	waitMaxSeconds    = 1800
	waitMinInterval   = 2
	waitMaxInterval   = 300
	waitDefaultPoll   = 10
	waitCheckTimeoutS = 120
)

func (*WaitTool) Spec() Spec {
	return Spec{
		Name: "wait",
		Description: "Подождать внутри текущего хода: просто N секунд, либо пока shell-команда `until` не завершится " +
			"с кодом 0 (проверяется каждые `interval` секунд, до `seconds` максимум). Используй, когда сказал бы " +
			"«проверю через пару минут»: дождись CI, раскатки, появления файла — и продолжай в том же ходу. " +
			"Для долгих ожиданий (больше получаса) используй schedule_wakeup. Максимум 1800 секунд.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"seconds":  map[string]any{"type": "integer", "description": "Сколько ждать (без until — ровно столько; с until — не дольше). 1..1800."},
				"until":    map[string]any{"type": "string", "description": "Необязательно: shell-команда; ожидание заканчивается, когда она вернёт код 0."},
				"interval": map[string]any{"type": "integer", "description": "Как часто повторять until, секунд (2..300, по умолчанию 10)."},
				"reason":   map[string]any{"type": "string", "description": "Зачем ждём — человек увидит это в карточке инструмента."},
			},
			"required":             []string{"seconds"},
			"additionalProperties": false,
		},
	}
}

type waitArgs struct {
	Seconds  int    `json:"seconds"`
	Until    string `json:"until"`
	Interval int    `json:"interval"`
	Reason   string `json:"reason"`
}

// RequiresApproval: a plain pause is free; a check command is a shell command
// and is judged exactly like Bash would judge it.
func (w *WaitTool) RequiresApproval(args json.RawMessage) bool {
	var p waitArgs
	_ = resolveJSON(args, &p)
	if strings.TrimSpace(p.Until) == "" {
		return false
	}
	b := &BashTool{cwd: w.cwd}
	raw, _ := json.Marshal(map[string]string{"command": p.Until})
	return b.RequiresApproval(raw)
}

func (w *WaitTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return w.ExecuteStream(ctx, args, func(string) {})
}

// ExecuteStream reports progress as it goes, so a long pause is visibly alive
// in the tool card rather than a frozen spinner.
func (w *WaitTool) ExecuteStream(ctx context.Context, args json.RawMessage, onChunk func(string)) (string, error) {
	var p waitArgs
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	if p.Seconds <= 0 {
		return "", errors.New("seconds обязателен и > 0")
	}
	if p.Seconds > waitMaxSeconds {
		p.Seconds = waitMaxSeconds
	}
	until := strings.TrimSpace(p.Until)
	start := time.Now()
	deadline := start.Add(time.Duration(p.Seconds) * time.Second)

	if until == "" {
		onChunk(fmt.Sprintf("жду %dс%s\n", p.Seconds, reasonSuffix(p.Reason)))
		if err := sleepCtx(ctx, deadline, onChunk); err != nil {
			return "", err
		}
		return fmt.Sprintf("✓ подождал %dс", p.Seconds), nil
	}

	interval := p.Interval
	if interval <= 0 {
		interval = waitDefaultPoll
	}
	if interval < waitMinInterval {
		interval = waitMinInterval
	}
	if interval > waitMaxInterval {
		interval = waitMaxInterval
	}
	onChunk(fmt.Sprintf("жду до %dс, проверяю каждые %dс: %s%s\n", p.Seconds, interval, until, reasonSuffix(p.Reason)))
	attempt := 0
	for {
		attempt++
		ok, tail := w.runCheck(ctx, until)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		elapsed := int(time.Since(start).Seconds())
		if ok {
			onChunk(fmt.Sprintf("✓ условие выполнено на попытке %d (%dс)\n", attempt, elapsed))
			return fmt.Sprintf("✓ условие выполнено через %dс (попытка %d)%s", elapsed, attempt, lastLine(tail)), nil
		}
		if time.Now().Add(time.Duration(interval) * time.Second).After(deadline) {
			return fmt.Sprintf("✗ условие не выполнилось за %dс (%d попыток)%s", elapsed, attempt, lastLine(tail)), nil
		}
		onChunk(fmt.Sprintf("… попытка %d не прошла (%dс), следующая через %dс\n", attempt, elapsed, interval))
		if err := sleepCtx(ctx, time.Now().Add(time.Duration(interval)*time.Second), nil); err != nil {
			return "", err
		}
	}
}

func (w *WaitTool) runCheck(ctx context.Context, command string) (bool, string) {
	cctx, cancel := context.WithTimeout(ctx, waitCheckTimeoutS*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(cctx, "/bin/sh", "-c", command)
	}
	cmd.Dir = w.cwd
	out, err := cmd.CombinedOutput()
	return err == nil, string(out)
}

// sleepCtx waits until the deadline, ticking a heartbeat every 30s so the
// card shows the pause is alive, and returns early when the turn is stopped.
func sleepCtx(ctx context.Context, deadline time.Time, onChunk func(string)) error {
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return nil
		}
		step := left
		if step > 30*time.Second {
			step = 30 * time.Second
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(step):
		}
		if onChunk != nil && time.Until(deadline) > 0 {
			onChunk(fmt.Sprintf("… осталось %dс\n", int(time.Until(deadline).Seconds())))
		}
	}
}

func reasonSuffix(r string) string {
	if strings.TrimSpace(r) == "" {
		return ""
	}
	return " — " + strings.TrimSpace(r)
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	l := strings.TrimSpace(lines[len(lines)-1])
	if len([]rune(l)) > 160 {
		l = string([]rune(l)[:160]) + "…"
	}
	return "; вывод: " + l
}
