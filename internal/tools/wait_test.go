package tools

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWaitPlainPause(t *testing.T) {
	w := &WaitTool{cwd: t.TempDir()}
	start := time.Now()
	out, err := w.Execute(context.Background(), json.RawMessage(`{"seconds":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Fatalf("returned too early: %v", time.Since(start))
	}
	if !strings.Contains(out, "подождал 1с") {
		t.Fatalf("unexpected result: %q", out)
	}
}

func TestWaitUntilSucceedsOnLaterAttempt(t *testing.T) {
	dir := t.TempDir()
	w := &WaitTool{cwd: dir}
	// The flag file appears after the first poll — the second attempt passes.
	go func() {
		time.Sleep(2500 * time.Millisecond)
		_ = os.WriteFile(dir+"/flag", []byte("ok"), 0o644)
	}()
	out, err := w.Execute(context.Background(), json.RawMessage(`{"seconds":30,"until":"test -f flag","interval":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "условие выполнено") {
		t.Fatalf("unexpected result: %q", out)
	}
}

func TestWaitUntilTimesOut(t *testing.T) {
	w := &WaitTool{cwd: t.TempDir()}
	out, err := w.Execute(context.Background(), json.RawMessage(`{"seconds":3,"until":"false","interval":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "не выполнилось") {
		t.Fatalf("unexpected result: %q", out)
	}
}

func TestWaitStopsWithContext(t *testing.T) {
	w := &WaitTool{cwd: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := w.Execute(ctx, json.RawMessage(`{"seconds":60}`))
	if err == nil {
		t.Fatal("expected a cancellation error")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("cancellation not honoured promptly: %v", time.Since(start))
	}
}

func TestWaitApprovalMirrorsBash(t *testing.T) {
	w := &WaitTool{cwd: t.TempDir()}
	if w.RequiresApproval(json.RawMessage(`{"seconds":5}`)) {
		t.Fatal("a plain pause must not ask for approval")
	}
	if !w.RequiresApproval(json.RawMessage(`{"seconds":5,"until":"rm -rf /tmp/x"}`)) {
		t.Fatal("a state-changing check command must ask, as Bash would")
	}
}

func TestWakeupSinkRoutesToContext(t *testing.T) {
	var got *ScheduleRequest
	ctx := WithWakeupSink(context.Background(), func(r ScheduleRequest) { got = &r })
	tool := &ScheduleWakeupTool{}
	if _, err := tool.Execute(ctx, json.RawMessage(`{"delay_seconds":45,"reason":"ci"}`)); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.DelaySeconds != 45 || got.Reason != "ci" {
		t.Fatalf("sink did not receive the request: %+v", got)
	}
	if TakeScheduledWakeup() != nil {
		t.Fatal("the pkg-level slot must stay empty when a sink is installed")
	}
	// Without a sink the TUI path still works.
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"delay_seconds":45,"reason":"x"}`)); err != nil {
		t.Fatal(err)
	}
	if TakeScheduledWakeup() == nil {
		t.Fatal("pkg-level slot expected without a sink")
	}
}
