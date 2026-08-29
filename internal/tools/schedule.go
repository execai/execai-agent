// schedule_wakeup — a tool the AI calls to schedule its own wake-up in N
// seconds (like Claude Code's dynamic /loop).
//
// How the AI uses it:
//   - Started a long process (CI build, kubectl rollout, deploy) → schedule_wakeup(60-3600)
//   - Polling an external API waiting for a response → schedule_wakeup
//   - Any autonomous flow that needs to "wait and continue"
//
// Architecture:
//
//	Two delivery paths, picked by the context the tool runs in:
//	  - a per-run sink installed with WithWakeupSink(ctx, fn): the IDE server
//	    runs several chats at once, and a wake-up belongs to the chat whose
//	    turn asked for it — a process-wide slot would hand it to whichever
//	    turn finished first;
//	  - the pkg-level slot (TakeScheduledWakeup) for the TUI, which has
//	    exactly one conversation and reads the slot after agentDoneMsg.
//	On wake the host sends PromptOnWake (or "continue") as the next user turn.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
)

type wakeupSinkKey struct{}

// WithWakeupSink routes schedule_wakeup calls made under ctx to fn instead of
// the process-wide slot.
func WithWakeupSink(ctx context.Context, fn func(ScheduleRequest)) context.Context {
	return context.WithValue(ctx, wakeupSinkKey{}, fn)
}

// MinWakeupDelay is the floor for delay_seconds. EXECAI_WAKEUP_MIN_SECONDS
// lowers it for self-tests: a real wake-up in thirty seconds is right for
// users and hopeless for a test run.
func MinWakeupDelay() int {
	if v, err := strconv.Atoi(os.Getenv("EXECAI_WAKEUP_MIN_SECONDS")); err == nil && v > 0 {
		return v
	}
	return 30
}

// ScheduleRequest is what the AI asked for.
type ScheduleRequest struct {
	DelaySeconds int
	Reason       string
	PromptOnWake string
}

var (
	pendingScheduleMu sync.Mutex
	pendingSchedule   *ScheduleRequest
)

// TakeScheduledWakeup takes and clears the pending request. Returns nil if none.
func TakeScheduledWakeup() *ScheduleRequest {
	pendingScheduleMu.Lock()
	defer pendingScheduleMu.Unlock()
	s := pendingSchedule
	pendingSchedule = nil
	return s
}

// ScheduleWakeupTool — the AI calls it when it wants to wake up later.
type ScheduleWakeupTool struct{}

func (*ScheduleWakeupTool) Spec() Spec {
	return Spec{
		Name: "schedule_wakeup",
		Description: "Запланировать СОБСТВЕННОЕ пробуждение через N секунд для продолжения автономной задачи. " +
			"Используй когда нужно подождать без блокировки (CI билд 5+ мин, kubectl rollout, polling внешнего API, ожидание чужого ответа). " +
			"После delay TUI автоматически продолжит беседу — ты снова получишь управление и сможешь проверить состояние. " +
			"Если ждать нечего и задача готова — НЕ зови этот tool, просто ответь финальным текстом.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"delay_seconds": map[string]any{
					"type":        "integer",
					"description": "Через сколько секунд проснуться. Минимум 30, максимум 3600 (1 час).",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Краткое описание зачем (для лога и истории). Например: 'жду пока CI билд закончится'",
				},
				"prompt_on_wake": map[string]any{
					"type":        "string",
					"description": "Какой промт автоматически отправится тебе когда проснёшься. Если пусто — будет 'продолжай'. Имеет смысл явный prompt типа 'проверь статус билда #123'.",
				},
			},
			"required":             []string{"delay_seconds", "reason"},
			"additionalProperties": false,
		},
	}
}

func (*ScheduleWakeupTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		DelaySeconds int    `json:"delay_seconds"`
		Reason       string `json:"reason"`
		PromptOnWake string `json:"prompt_on_wake"`
	}
	if err := resolveJSON(args, &p); err != nil {
		return "", err
	}
	if p.DelaySeconds <= 0 {
		return "", errors.New("delay_seconds обязателен и > 0")
	}
	if p.DelaySeconds < MinWakeupDelay() {
		p.DelaySeconds = MinWakeupDelay()
	}
	if p.DelaySeconds > 3600 {
		p.DelaySeconds = 3600
	}
	req := ScheduleRequest{
		DelaySeconds: p.DelaySeconds,
		Reason:       p.Reason,
		PromptOnWake: p.PromptOnWake,
	}
	if sink, ok := ctx.Value(wakeupSinkKey{}).(func(ScheduleRequest)); ok && sink != nil {
		sink(req)
	} else {
		pendingScheduleMu.Lock()
		pendingSchedule = &req
		pendingScheduleMu.Unlock()
	}
	mins := p.DelaySeconds / 60
	secs := p.DelaySeconds % 60
	when := ""
	if mins > 0 {
		when = fmt.Sprintf("%dмин %dс", mins, secs)
	} else {
		when = fmt.Sprintf("%dс", secs)
	}
	return fmt.Sprintf("✓ Запланировано пробуждение через %s. Reason: %s. Теперь заверши текущий ответ — хост (терминал или редактор) сам разбудит тебя через указанное время.", when, p.Reason), nil
}

func (*ScheduleWakeupTool) RequiresApproval(json.RawMessage) bool { return false }
