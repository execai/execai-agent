package llm

// Live model lists of the two delegated CLIs.
//
// Neither has an API key to call a /models endpoint with — the account lives
// inside the CLI. Both, however, answer the question themselves over the
// protocol their own SDK / editor extension uses:
//
//   - Claude Code: `claude -p --input-format stream-json` answers the
//     `initialize` control request with the models of the account and of
//     this CLI version (what the Agent SDK's supportedModels() returns);
//   - Codex: `codex app-server` (JSON-RPC over stdio, what the Codex editor
//     extension talks to) answers `model/list`.
//
// Neither request starts a model turn, so asking costs no tokens. The
// built-in lists these replace had gone stale twice: Codex still offered
// gpt-5/o3/o4-mini when the account had gpt-5.6-sol, and Claude pinned IDs
// the installed CLI refused.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/velesbsdllc/agent-vbai/internal/version"
)

// CLIModel is one model a delegated CLI offers.
type CLIModel struct {
	ID          string // value for --model; "" = do not pass --model
	Name        string
	Description string
	Default     bool
}

// cliListTimeout bounds one listing. Claude Code needs a couple of seconds to
// start; a hung CLI must not hang the picker.
const cliListTimeout = 25 * time.Second

// ListClaudeCLIModels asks the newest local Claude Code for its models.
// "default" becomes the empty ID: without --model claude uses the model of
// its own config / plan, exactly what "Default" means in its picker.
func ListClaudeCLIModels(ctx context.Context) ([]CLIModel, error) {
	bin, ok := findClaude()
	if !ok {
		return nil, errors.New("claude CLI не найден")
	}
	ctx, cancel := context.WithTimeout(ctx, cliListTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin.Path, "--print",
		"--input-format", "stream-json", "--output-format", "stream-json", "--verbose")
	// A neutral directory: project settings and CLAUDE.md of wherever the
	// agent was started have nothing to do with the list of models.
	cmd.Dir = os.TempDir()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	req, _ := json.Marshal(map[string]any{
		"type":       "control_request",
		"request_id": "execai-models",
		"request":    map[string]any{"subtype": "initialize"},
	})
	if _, err := stdin.Write(append(req, '\n')); err != nil {
		return nil, err
	}
	var models []struct {
		Value       string `json:"value"`
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
	}
	err = readJSONLines(stdout, func(line []byte) (bool, error) {
		var msg struct {
			Type     string `json:"type"`
			Response struct {
				Subtype   string          `json:"subtype"`
				RequestID string          `json:"request_id"`
				Error     string          `json:"error"`
				Response  json.RawMessage `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(line, &msg) != nil || msg.Type != "control_response" ||
			msg.Response.RequestID != "execai-models" {
			return false, nil
		}
		if msg.Response.Subtype != "success" {
			return true, fmt.Errorf("claude: %s", msg.Response.Error)
		}
		var body struct {
			Models json.RawMessage `json:"models"`
		}
		if err := json.Unmarshal(msg.Response.Response, &body); err != nil {
			return true, err
		}
		return true, json.Unmarshal(body.Models, &models)
	})
	if err != nil {
		return nil, err
	}
	out := make([]CLIModel, 0, len(models))
	for _, m := range models {
		id := m.Value
		def := false
		if id == "default" {
			id, def = "", true
		}
		out = append(out, CLIModel{ID: id, Name: m.DisplayName, Description: m.Description, Default: def})
	}
	if len(out) == 0 {
		return nil, errors.New("claude не вернул ни одной модели")
	}
	return out, nil
}

// ListCodexCLIModels asks `codex app-server` for the models of the signed-in
// ChatGPT account. Hidden entries are what the Codex picker hides as well.
func ListCodexCLIModels(ctx context.Context) ([]CLIModel, error) {
	path, err := exec.LookPath("codex")
	if err != nil {
		return nil, errors.New("codex CLI не найден в PATH")
	}
	ctx, cancel := context.WithTimeout(ctx, cliListTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "app-server")
	cmd.Dir = os.TempDir()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	send := func(v map[string]any) error {
		b, _ := json.Marshal(v)
		_, err := stdin.Write(append(b, '\n'))
		return err
	}
	if err := send(map[string]any{"method": "initialize", "id": 0, "params": map[string]any{
		"clientInfo": map[string]any{"name": "execai", "title": "ExecAI", "version": version.Get()},
	}}); err != nil {
		return nil, err
	}

	var out []CLIModel
	nextID := 1
	cursor := ""
	requestPage := func() error {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		return send(map[string]any{"method": "model/list", "id": nextID, "params": params})
	}
	err = readJSONLines(stdout, func(line []byte) (bool, error) {
		var msg struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(line, &msg) != nil || msg.ID == nil {
			return false, nil // notifications
		}
		if msg.Error != nil {
			return true, fmt.Errorf("codex: %s", msg.Error.Message)
		}
		if *msg.ID == 0 {
			if err := send(map[string]any{"method": "initialized"}); err != nil {
				return true, err
			}
			return false, requestPage()
		}
		if *msg.ID != nextID {
			return false, nil
		}
		var page struct {
			Data []struct {
				ID          string `json:"id"`
				Model       string `json:"model"`
				DisplayName string `json:"displayName"`
				Description string `json:"description"`
				Hidden      bool   `json:"hidden"`
				IsDefault   bool   `json:"isDefault"`
			} `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		if err := json.Unmarshal(msg.Result, &page); err != nil {
			return true, err
		}
		for _, m := range page.Data {
			if m.Hidden {
				continue
			}
			id := m.Model
			if id == "" {
				id = m.ID
			}
			out = append(out, CLIModel{ID: id, Name: m.DisplayName, Description: m.Description, Default: m.IsDefault})
		}
		if page.NextCursor == nil || *page.NextCursor == "" || nextID > 20 {
			return true, nil
		}
		cursor = *page.NextCursor
		nextID++
		return false, requestPage()
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("codex не вернул ни одной модели")
	}
	return out, nil
}

// readJSONLines feeds stdout lines to fn until it reports done or the stream
// ends. A stream that ends first is an error: the answer never came.
func readJSONLines(r io.Reader, fn func(line []byte) (done bool, err error)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] != '{' {
			continue
		}
		done, err := fn([]byte(line))
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return errors.New("CLI завершился, не ответив")
}
