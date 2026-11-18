// Package inbox is the agent's side of the "web → agent" channel.
//
// A person starts a task from the ExecAI web chat (run_on_agent); agents-vbai
// puts it into this machine's inbox; exactly one local process takes it, runs
// it in the directory bound to the task's project and posts the result back.
//
// Two front-ends own the inbox: the background `execai serve` and the editor
// panel (`execai ide`). Everything they share lives here, in one place:
//
//   - the HTTP client: poll, ack, result, permission questions, project
//     bindings and the project list (client.go);
//   - the task, its prompt and the directory it runs in (this file, place.go);
//   - the per-machine lock that keeps the inbox to exactly one owner
//     (lock.go): two owners would split the stream of tasks at random, and the
//     person would not know where their task went;
//   - the timeouts and the answer vocabulary shared with agents-vbai
//     (internal/ask) — an answer that means something different on each side
//     would turn "once" into "always".
package inbox

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

const (
	// PollWait — how long one long-poll is held. The server caps it.
	PollWait = 60 * time.Second
	// TaskTimeout — the limit for one task. A stuck task would otherwise hold
	// the inbox forever, and every next task would wait behind it.
	TaskTimeout = 5 * time.Minute
	// AskTimeout — how long a permission question waits for the person. The
	// server holds the request open meanwhile, so this is longer than its own
	// limit.
	AskTimeout = 5 * time.Minute
	// ResultTimeout — how long delivering a result may take.
	ResultTimeout = 30 * time.Second
	// requestTimeout — the HTTP client limit for poll: longer than PollWait,
	// otherwise every normal long-poll would be cut.
	requestTimeout = 3 * time.Minute
	// apiTimeout — the limit for short calls (ack, bindings, projects).
	apiTimeout = 20 * time.Second
)

// Answers to a permission question. The same vocabulary is used by
// agents-vbai (internal/ask), the web chat, the terminal and the editor panel;
// an unknown value means deny. Values must never diverge: the person presses
// "Once" and must not get "Always".
const (
	AnswerOnce    = "once"    // allow this call
	AnswerSession = "session" // allow the whole tool until the task (chat) ends
	AnswerExact   = "exact"   // allow exactly this call until the task (chat) ends
	AnswerAlways  = "always"  // allow and record in permissions.json
	AnswerDeny    = "deny"
)

// Result chunk types (POST /tasks/<id>/result).
const (
	ChunkFinal = "final"
	ChunkError = "error"
)

// Task is a task from the inbox.
type Task struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Payload     string `json:"payload"`
}

// ExtractPrompt returns the task text from its payload ({"prompt": …}); a
// payload that is a bare string is a task too.
func ExtractPrompt(payload string) string {
	var p struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return strings.TrimSpace(payload)
	}
	return strings.TrimSpace(p.Prompt)
}

// Outcome turns what a run produced into the task's result. A finished run
// gives its text; a failed one gives the partial text with the reason
// appended (a partial answer is more useful than a bare error), or the error
// alone when there was no text.
func Outcome(text string, err error) (chunk, data string) {
	if err != nil {
		if strings.TrimSpace(text) != "" {
			return ChunkFinal, text + "\n\n" + i18n.Tf("inbox.result.interrupted", err.Error())
		}
		return ChunkError, err.Error()
	}
	if strings.TrimSpace(text) == "" {
		return ChunkFinal, i18n.T("inbox.result.noText")
	}
	return ChunkFinal, text
}

// Short is the first 8 characters of an id, for logs.
func Short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// FirstLine is the first line of s, cut to 80 runes (by runes: a byte cut
// would break Cyrillic in half).
func FirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return string(r)
}
