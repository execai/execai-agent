package agent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

// Asker names who asks for a permission when it is not the main agent: a
// subagent, shown to the person by its title ("agent «review tests» wants to
// run Bash").
type Asker struct {
	ID    string
	Title string
}

// AttributedApprover is implemented by approvers that can show who asks and
// can withdraw a question when the asker is gone (its ctx ended: the
// subagent was stopped or timed out). The terminal shows the title in the
// dialog, the editor panel gets it as a field of the ask event.
type AttributedApprover interface {
	AskApproveAs(ctx context.Context, who Asker, toolName string, args json.RawMessage, summary string) ApproveDecision
}

// AskAs asks ap on behalf of who. An approver that does not know about
// askers gets the title in the summary, and a question nobody answers before
// ctx ends is a refusal — the waiting goroutine is left to the approver.
func AskAs(ctx context.Context, ap Approver, who Asker, toolName string, args json.RawMessage, summary string) ApproveDecision {
	if ap == nil {
		return ApproveDeny
	}
	if ctx.Err() != nil {
		return ApproveDeny
	}
	if aa, ok := ap.(AttributedApprover); ok {
		return aa.AskApproveAs(ctx, who, toolName, args, summary)
	}
	ch := make(chan ApproveDecision, 1)
	go func() { ch <- ap.AskApprove(toolName, args, AttributedSummary(who, summary)) }()
	select {
	case d := <-ch:
		return d
	case <-ctx.Done():
		return ApproveDeny
	}
}

// AttributedSummary prefixes a question's summary with the asker, for
// interfaces that show only text (the web chat, older panels).
func AttributedSummary(who Asker, summary string) string {
	if who.Title == "" {
		return summary
	}
	return i18n.Tf("fleet.ask.prefix", who.Title) + "\n" + summary
}

// TaskCardTitle is the card text of a Task call: the subagent's title and
// the switches that change what it may do.
func TaskCardTitle(args map[string]any) string {
	title, _ := args["description"].(string)
	title = strings.TrimSpace(title)
	var marks []string
	if mode, _ := args["mode"].(string); strings.EqualFold(mode, "write") {
		marks = append(marks, i18n.T("fleet.card.write"))
	}
	if bg, _ := args["background"].(bool); bg {
		marks = append(marks, i18n.T("fleet.card.background"))
	}
	if model, _ := args["model"].(string); model != "" && model != "same" {
		marks = append(marks, model)
	}
	if len(marks) == 0 {
		return "🤖 " + title
	}
	return "🤖 " + title + " · " + strings.Join(marks, " · ")
}
