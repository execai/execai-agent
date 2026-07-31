package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
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
	// Every tool it does have must be approval-free, otherwise the subagent
	// blocks forever on a modal nobody will ever see.
	for _, name := range r.Names() {
		tool, _ := r.Get(name)
		if tool.RequiresApproval(json.RawMessage(`{}`)) {
			t.Errorf("%s требует подтверждения — субагент повиснет на невидимом диалоге", name)
		}
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
