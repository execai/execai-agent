package inbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/version"
)

// Client talks to agents-vbai (and to integrations-vbai for the project list)
// through the API gateway with the agent's JWT. The zero value is not usable;
// build it with NewClient. A Client is cheap: callers that see the token
// change (re-login) simply build a new one.
type Client struct {
	Base  string // API base, e.g. https://api.execai.ru
	Token string // the agent's JWT
}

// NewClient builds a client for the API base and token.
func NewClient(base, token string) *Client {
	return &Client{Base: strings.TrimRight(base, "/"), Token: token}
}

// HTTPError keeps the response status: a 401 (re-read the credentials) and a
// 5xx (wait and retry) call for different reactions, and the error string
// alone does not tell them apart.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body) }

// IsUnauthorized reports whether err is an HTTP 401.
func IsUnauthorized(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.Status == http.StatusUnauthorized
}

// do performs one request. timeout bounds the whole exchange.
func (c *Client) do(ctx context.Context, method, path string, body []byte, timeout time.Duration) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Base, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", version.UserAgent())
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode >= 400 {
		return nil, &HTTPError{Status: resp.StatusCode, Body: strings.TrimSpace(string(data))}
	}
	return data, nil
}

// Poll waits up to wait for tasks. skip — tasks this process has seen and
// left for another owner: a server that knows the field does not offer them
// again (an older one ignores it and the caller has to slow down itself).
//
// What poll returns is only an offer: a task is ours after Ack.
func (c *Client) Poll(ctx context.Context, wait time.Duration, skip []string) ([]Task, error) {
	body := []byte(`{}`)
	if len(skip) > 0 {
		body, _ = json.Marshal(map[string]any{"skip": skip})
	}
	path := fmt.Sprintf("/agents-vbai/inbox/poll?wait=%d", int(wait.Seconds()))
	data, err := c.do(ctx, http.MethodPost, path, body, requestTimeout)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Tasks []Task `json:"tasks"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("inbox.err.parse"), err)
	}
	return resp.Tasks, nil
}

// Ack confirms receipt and returns only the tasks the server really pinned to
// this machine. Until then a task stays pending: an offer that went into a
// dead connection is not lost, and a task is never run twice.
func (c *Client) Ack(ctx context.Context, tasks []Task) ([]Task, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	body, _ := json.Marshal(map[string]any{"task_ids": ids})
	data, err := c.do(ctx, http.MethodPost, "/agents-vbai/inbox/ack", body, apiTimeout)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Acked []string `json:"acked"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("inbox.err.parse"), err)
	}
	acked := map[string]bool{}
	for _, id := range resp.Acked {
		acked[id] = true
	}
	out := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		if acked[t.ID] {
			out = append(out, t)
		}
	}
	return out, nil
}

// PostResult delivers a task's result. It runs on its own context: when the
// caller's one is already cancelled (Ctrl+C, the editor closed), the result
// still has to arrive, otherwise the web chat hangs until its timeout.
func (c *Client) PostResult(ctx context.Context, taskID, chunk, data string) error {
	body, _ := json.Marshal(map[string]string{"chunk_type": chunk, "data": data})
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ResultTimeout)
	defer cancel()
	_, err := c.do(ctx, http.MethodPost, "/agents-vbai/tasks/"+taskID+"/result", body, ResultTimeout)
	return err
}

// PutFleet reports this process's subagents (internal/fleet) to the server,
// which shows them on the machine's card in the web panel. body is the JSON
// object {agents, pid, cwd}; the server replaces what it had for this
// machine and forgets it after a few minutes without a report.
func (c *Client) PutFleet(ctx context.Context, body []byte) error {
	_, err := c.do(ctx, http.MethodPut, "/agents-vbai/presence/fleet", body, apiTimeout)
	return err
}

// Ask puts a permission question into the web chat and waits for the answer.
// The server holds the request open while the person thinks; a cancelled ctx
// drops the request, and the server closes the question as unanswered.
func (c *Client) Ask(ctx context.Context, taskID, tool, summary, args string) (answer, questionID string, err error) {
	body, _ := json.Marshal(map[string]string{"tool": tool, "summary": summary, "args": args})
	data, err := c.do(ctx, http.MethodPost, "/agents-vbai/tasks/"+taskID+"/ask", body, AskTimeout)
	if err != nil {
		return "", "", err
	}
	var resp struct {
		Answer     string `json:"answer"`
		QuestionID string `json:"question_id"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", "", fmt.Errorf("%s: %w", i18n.T("inbox.err.parse"), err)
	}
	return resp.Answer, resp.QuestionID, nil
}

// Question is an open permission question as the web chat lists it.
type Question struct {
	ID     string `json:"id"`
	TaskID string `json:"task_id"`
	Tool   string `json:"tool"`
	Status string `json:"status"`
}

// OpenQuestions lists the account's open questions (GET /questions — the
// same list the web chat shows). The agent's JWT carries the account, so the
// agent sees exactly what its owner sees.
func (c *Client) OpenQuestions(ctx context.Context) ([]Question, error) {
	data, err := c.do(ctx, http.MethodGet, "/agents-vbai/questions", nil, apiTimeout)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Questions []Question `json:"questions"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("inbox.err.parse"), err)
	}
	return resp.Questions, nil
}

// AnswerQuestion answers a question the way the web chat does
// (POST /questions/<id>/answer).
func (c *Client) AnswerQuestion(ctx context.Context, questionID, answer string) error {
	body, _ := json.Marshal(map[string]string{"answer": answer})
	_, err := c.do(ctx, http.MethodPost, "/agents-vbai/questions/"+questionID+"/answer", body, apiTimeout)
	return err
}

// Withdraw closes the open questions of a task with the answer already given
// elsewhere (in the editor panel), so the web chat stops asking. Returns how
// many it closed; errors are not fatal — the question also closes itself when
// the agent drops its Ask request.
func (c *Client) Withdraw(ctx context.Context, taskID, answer string) (int, error) {
	qs, err := c.OpenQuestions(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, q := range qs {
		if q.TaskID != taskID || (q.Status != "" && q.Status != "open") {
			continue
		}
		if err := c.AnswerQuestion(ctx, q.ID, answer); err == nil {
			n++
		}
	}
	return n, nil
}

// Binding ties a project (workspace) to a directory on a machine.
type Binding struct {
	AgentID     string `json:"agent_id"`
	WorkspaceID string `json:"workspace_id"`
	LocalPath   string `json:"local_path"`
}

// Bindings lists this agent's project bindings.
func (c *Client) Bindings(ctx context.Context) ([]Binding, error) {
	data, err := c.do(ctx, http.MethodGet, "/agents-vbai/workspaces/bindings", nil, apiTimeout)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Bindings []Binding `json:"bindings"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("inbox.err.parse"), err)
	}
	return resp.Bindings, nil
}

// Bind binds the project to a directory on this machine.
func (c *Client) Bind(ctx context.Context, workspaceID, localPath string) error {
	body, _ := json.Marshal(map[string]string{"workspace_id": workspaceID, "local_path": localPath})
	_, err := c.do(ctx, http.MethodPost, "/agents-vbai/workspaces/bind", body, apiTimeout)
	return err
}

// Unbind removes the project's binding on this machine.
func (c *Client) Unbind(ctx context.Context, workspaceID string) error {
	body, _ := json.Marshal(map[string]string{"workspace_id": workspaceID})
	_, err := c.do(ctx, http.MethodPost, "/agents-vbai/workspaces/unbind", body, apiTimeout)
	return err
}
