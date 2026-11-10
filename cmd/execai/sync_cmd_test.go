package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

// scriptFlow asks two questions and records the answers.
type scriptFlow struct{ answers []string }

func (f *scriptFlow) Start(context.Context) agentsync.Step {
	return agentsync.Step{Text: "intro", Prompt: "password?", Secret: true}
}

func (f *scriptFlow) Answer(_ context.Context, a string) agentsync.Step {
	f.answers = append(f.answers, a)
	if len(f.answers) == 1 {
		return agentsync.Step{Prompt: "again?", Secret: true}
	}
	return agentsync.Step{Text: "done"}
}

// From a pipe each answer is a line, trimmed; the flow's texts and
// questions are printed, the answers are not.
func TestRunSyncFlow_Pipe(t *testing.T) {
	f := &scriptFlow{}
	var out bytes.Buffer
	if err := runSyncFlow(context.Background(), f, strings.NewReader("  secret one \nsecret one"), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.answers, "|") != "secret one|secret one" {
		t.Fatalf("answers %q", f.answers)
	}
	s := out.String()
	if !strings.Contains(s, "intro") || !strings.Contains(s, "again?") || !strings.Contains(s, "done") || strings.Contains(s, "secret one") {
		t.Fatalf("output %q", s)
	}
}

// Input ends before the questions do: the flow stops quietly.
func TestRunSyncFlow_EOF(t *testing.T) {
	f := &scriptFlow{}
	var out bytes.Buffer
	if err := runSyncFlow(context.Background(), f, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if len(f.answers) != 0 || strings.Contains(out.String(), "done") {
		t.Fatalf("answers %q out %q", f.answers, out.String())
	}
}
