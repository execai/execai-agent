package tools

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/security"
)

// The read-only registry is a safety boundary, not a convenience: a subagent's
// actions are never shown to the user for approval. If someone adds Bash or
// Write to it, the subagent can change the machine silently.
func TestReadOnlyRegistry_HasNoActingTools(t *testing.T) {
	r := ReadOnly(".")
	forbidden := []string{"Bash", "Write", "Edit", "ScheduleWakeup"}
	for _, name := range forbidden {
		if _, ok := r.Get(name); ok {
			t.Errorf("субагенту доступен %s — он может менять машину без подтверждения", name)
		}
	}
	// Recursion would multiply the user's provider quota spend invisibly.
	if _, ok := r.Get("Task"); ok {
		t.Error("субагент может порождать субагентов — рекурсия сожжёт квоту")
	}
	// And it must still be able to investigate.
	for _, name := range []string{"Read", "Grep", "Glob", "LS"} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("субагент без %s не сможет ничего изучить", name)
		}
	}
	// Периметр действует и на субагента.
	//
	// Раньше здесь стояло обратное требование: «ни один инструмент субагента
	// не должен спрашивать». Оно казалось безопасным (иначе субагент повиснет
	// на невидимом диалоге), но на деле означало, что субагент читает ЛЮБОЙ
	// файл и ходит на ЛЮБОЙ адрес — то есть служит обходом всех проверок
	// сразу. Висеть он при этом всё равно не мог: его согласователь отвечает
	// отказом, не спрашивая никого (см. chat/subagent.go).
	//
	// Правильный инвариант: инструменты, выносящие данные наружу, обязаны
	// спрашивать и у субагента — а значит выполнятся только там, где человек
	// уже выдал разрешение.
	security.Set(security.Deep)
	for _, name := range []string{"WebFetch", "WebSearch"} {
		tool, ok := r.Get(name)
		if !ok {
			continue
		}
		if !tool.RequiresApproval(json.RawMessage(`{"url":"https://example.com"}`)) {
			t.Errorf("%s у субагента не спрашивает — это канал утечки в обход периметра", name)
		}
	}
	outside, _ := r.Get("Read")
	if !outside.RequiresApproval(json.RawMessage(`{"path":"/etc/shadow"}`)) {
		t.Error("субагент читает вне проекта без разрешения — периметр обходится делегированием")
	}
}

func TestAskUser_RejectsBadOptionCounts(t *testing.T) {
	tool := &AskUserTool{}
	cases := []struct {
		name string
		args string
	}{
		{"один вариант", `{"question":"что делать?","options":[{"label":"а"}]}`},
		{"ни одного", `{"question":"что делать?","options":[]}`},
		{"пустой вопрос", `{"question":"  ","options":[{"label":"а"},{"label":"б"}]}`},
		{"вариант без label", `{"question":"что?","options":[{"label":"а"},{"description":"б"}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := tool.Execute(context.Background(), json.RawMessage(c.args)); err == nil {
				t.Error("ожидалась ошибка, получен успех")
			}
		})
	}
}

// More than four options is the model over-reaching, not a fatal error:
// trim instead of failing the whole turn.
func TestAskUser_TrimsToFourOptions(t *testing.T) {
	var got []AskOption
	SetAskUserFunc(func(_ context.Context, _ string, o []AskOption) (string, error) {
		got = o
		return o[0].Label, nil
	})
	defer SetAskUserFunc(nil)

	args := `{"question":"?","options":[{"label":"1"},{"label":"2"},{"label":"3"},{"label":"4"},{"label":"5"},{"label":"6"}]}`
	if _, err := (&AskUserTool{}).Execute(context.Background(), json.RawMessage(args)); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 4 {
		t.Errorf("вариантов после обрезки: %d, ожидалось 4", len(got))
	}
}

// Without a UI (plain REPL, pipe, or inside a subagent) the tool must say so
// rather than hang waiting for an answer that can never arrive.
func TestAskUser_NoUIReportsUnavailable(t *testing.T) {
	SetAskUserFunc(nil)
	_, err := (&AskUserTool{}).Execute(context.Background(),
		json.RawMessage(`{"question":"?","options":[{"label":"а"},{"label":"б"}]}`))
	if err == nil {
		t.Fatal("без UI ожидалась ошибка")
	}
	if !strings.Contains(err.Error(), "недоступ") {
		t.Errorf("невнятная ошибка: %v", err)
	}
}

func TestTask_RequiresPrompt(t *testing.T) {
	SetSubagentRunner(func(context.Context, string, string) (string, error) { return "ok", nil })
	defer SetSubagentRunner(nil)

	if _, err := (&TaskTool{}).Execute(context.Background(), json.RawMessage(`{"description":"x"}`)); err == nil {
		t.Error("Task без prompt должен возвращать ошибку")
	}
}

// Task parameters: defaults, the iteration cap and a bad mode.
func TestParseSubagentRequest(t *testing.T) {
	req, err := ParseSubagentRequest(json.RawMessage(`{"description":" find X ","prompt":"p"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Mode != "read" || req.Model != "same" || req.MaxIterations != SubagentDefaultIterations || req.Background {
		t.Errorf("defaults: %+v", req)
	}
	if req.Description != "find X" {
		t.Errorf("description %q", req.Description)
	}
	req, err = ParseSubagentRequest(json.RawMessage(`{"description":"d","prompt":"p","mode":"WRITE","model":"fast","max_iterations":500,"background":true,"cwd":"sub"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Mode != "write" || req.Model != "fast" || req.MaxIterations != SubagentMaxIterations || !req.Background || req.Cwd != "sub" {
		t.Errorf("explicit: %+v", req)
	}
	if _, err := ParseSubagentRequest(json.RawMessage(`{"prompt":"p","mode":"admin"}`)); err == nil {
		t.Error("unknown mode accepted")
	}
}

// A write subagent acts, but cannot recurse, ask questions or schedule
// wake-ups for a turn it will never have.
func TestForSubagent_WriteSet(t *testing.T) {
	r := ForSubagent(".", true)
	for _, name := range []string{"Write", "Edit", "Bash", "Read"} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("write subagent lacks %s", name)
		}
	}
	for _, name := range []string{"Task", "TaskWait", "AskUser", "schedule_wakeup"} {
		if _, ok := r.Get(name); ok {
			t.Errorf("write subagent has %s", name)
		}
	}
	if _, ok := ForSubagent(".", false).Get("Bash"); ok {
		t.Error("read subagent has Bash")
	}
}

// The runner comes from the turn's context; the tool-call key goes with it.
func TestTask_UsesRunnerFromContext(t *testing.T) {
	SetSubagentRunner(nil)
	if _, err := (&TaskTool{}).Execute(context.Background(), json.RawMessage(`{"description":"x","prompt":"p"}`)); err != ErrSubagentUnavailable {
		t.Fatalf("without a runner: %v", err)
	}
	r := &recRunner{}
	ctx := WithToolCallID(WithSubagentRunner(context.Background(), r), "call-7")
	out, err := (&TaskTool{}).Execute(ctx, json.RawMessage(`{"description":"x","prompt":"p"}`))
	if err != nil || out != "ran x" || r.got.ToolCall != "call-7" {
		t.Errorf("out %q err %v req %+v", out, err, r.got)
	}
	out, err = (&TaskWaitTool{}).Execute(ctx, json.RawMessage(`{}`))
	if err != nil || out != "waited all" {
		t.Errorf("TaskWait: %q %v", out, err)
	}
}

type recRunner struct{ got SubagentRequest }

func (r *recRunner) Run(_ context.Context, req SubagentRequest) (string, error) {
	r.got = req
	return "ran " + req.Description, nil
}

func (r *recRunner) Wait(_ context.Context, id string) (string, error) { return "waited " + id, nil }
func (r *recRunner) Extend(_ context.Context, id string, steps int) (string, error) {
	return "extended " + id + " by " + strconv.Itoa(steps), nil
}

// The default for new subagents follows the setting and never goes above
// the most a subagent may start with.
func TestSubagentDefault_Configurable(t *testing.T) {
	defer SetSubagentDefault(0)
	if SubagentDefault() != SubagentDefaultIterations || SubagentDefaultIterations != 50 || SubagentMaxIterations != 100 {
		t.Fatalf("defaults %d/%d/%d", SubagentDefault(), SubagentDefaultIterations, SubagentMaxIterations)
	}
	SetSubagentDefault(80)
	req, err := ParseSubagentRequest(json.RawMessage(`{"prompt":"p"}`))
	if err != nil || req.MaxIterations != 80 {
		t.Errorf("configured default: %d %v", req.MaxIterations, err)
	}
	SetSubagentDefault(1000)
	if SubagentDefault() != SubagentMaxIterations {
		t.Errorf("default above the maximum: %d", SubagentDefault())
	}
	SetSubagentDefault(0)
	if SubagentDefault() != SubagentDefaultIterations {
		t.Errorf("reset: %d", SubagentDefault())
	}
}

// TaskExtend reaches the turn's runner; a subagent cannot extend anything.
func TestTaskExtend(t *testing.T) {
	ctx := WithSubagentRunner(context.Background(), &recRunner{})
	out, err := (&TaskExtendTool{}).Execute(ctx, json.RawMessage(`{"id":"sa-1","steps":30}`))
	if err != nil || out != "extended sa-1 by 30" {
		t.Errorf("extend: %q %v", out, err)
	}
	out, _ = (&TaskExtendTool{}).Execute(ctx, json.RawMessage(`{"id":"sa-1","steps":5000}`))
	if out != "extended sa-1 by 100" {
		t.Errorf("one call gives at most SubagentMaxIterations: %q", out)
	}
	if _, err := (&TaskExtendTool{}).Execute(ctx, json.RawMessage(`{}`)); err == nil {
		t.Error("no id accepted")
	}
	if _, ok := Default(".").Get("TaskExtend"); !ok {
		t.Error("the main agent has no TaskExtend")
	}
	if _, ok := ForSubagent(".", true).Get("TaskExtend"); ok {
		t.Error("a write subagent has TaskExtend")
	}
}
