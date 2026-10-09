package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type nopCloser struct{ *bytes.Buffer }

func (nopCloser) Close() error { return nil }

// The stale global install must not win over the fresh binary bundled with the
// editor extension: that is exactly how the agent ended up on claude 2.1.220
// (Opus 5 behind the `opus` alias, Opus 5.5 rejected) while Claude Code in
// Cursor was 2.1.292.
func TestFindClaudePrefersNewestInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("EXECAI_CLAUDE_BIN", "")

	pathDir := filepath.Join(home, "bin")
	writeFakeClaude(t, filepath.Join(pathDir, "claude"), "echo '2.1.220 (Claude Code)'")
	t.Setenv("PATH", pathDir)

	for _, v := range []string{"2.1.281", "2.1.292", "2.1.283"} {
		writeFakeClaude(t, filepath.Join(home, ".cursor", "extensions",
			"anthropic.claude-code-"+v+"-linux-x64", "resources", "native-binary", "claude"), "exit 1")
	}
	// A folder without the binary (older extensions bundled cli.js) is skipped.
	if err := os.MkdirAll(filepath.Join(home, ".vscode", "extensions", "anthropic.claude-code-2.9.0-linux-x64"), 0o755); err != nil {
		t.Fatal(err)
	}

	bin, ok := findClaude()
	if !ok {
		t.Fatal("no claude found")
	}
	if bin.Version != "2.1.292" || !strings.Contains(bin.Path, "2.1.292") {
		t.Fatalf("picked %s %s, want the 2.1.292 extension binary", bin.Version, bin.Path)
	}

	// An explicit pin wins over any search.
	t.Setenv("EXECAI_CLAUDE_BIN", filepath.Join(pathDir, "claude"))
	bin, _ = findClaude()
	if bin.Path != filepath.Join(pathDir, "claude") || bin.Version != "2.1.220" {
		t.Fatalf("EXECAI_CLAUDE_BIN ignored: %+v", bin)
	}
}

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.1.220", "2.1.292", true},
		{"2.1.292", "2.1.220", false},
		{"2.1.99", "2.1.100", true}, // numeric, not lexical
		{"2.1.0", "2.1.0", false},
		{"", "2.0.0", true},
		{"2.0.0", "", false},
	}
	for _, c := range cases {
		if got := versionLess(c.a, c.b); got != c.want {
			t.Errorf("versionLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// A recorded turn: text, a parallel pair of tool calls, a permission request,
// more text. Checks everything the agent relies on at once.
func TestParseClaudeStreamJSON(t *testing.T) {
	lines := []string{
		`{"type":"system","subtype":"init","model":"claude-opus-5-5"}`,
		`{"type":"stream_event","event":{"type":"message_start","message":{"model":"claude-opus-5-5","usage":{"input_tokens":3,"cache_read_input_tokens":100}}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Сначала прочитаю."}}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/w/a.txt"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"/w/b.txt"}}]}}`,
		// a subagent's own text and tools stay out of the answer
		`{"type":"stream_event","parent_tool_use_id":"x","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"SUBAGENT"}}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2","content":"B"},{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"A"}]}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"rm -rf /tmp/x"}}]}}`,
		`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"rm -rf /tmp/x"}}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t3","content":"denied","is_error":true}]}}`,
		`{"type":"control_request","request_id":"r2","request":{"subtype":"hook_callback"}}`,
		`{"type":"stream_event","event":{"type":"message_start","message":{"model":"claude-opus-5-5","usage":{"input_tokens":5}}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Готово."}}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"Готово."}`,
	}
	var events []string
	var asked []string
	cb := StreamCallbacks{
		OnDelegatedTool: func(name string, input json.RawMessage) {
			var in map[string]any
			_ = json.Unmarshal(input, &in)
			p, _ := in["file_path"].(string)
			events = append(events, "call "+name+" "+filepath.Base(p))
		},
		OnDelegatedToolResult: func(name, result string, isError bool) {
			events = append(events, "result "+name+" "+result)
		},
		ApproveDelegatedTool: func(name string, input json.RawMessage) (bool, string) {
			asked = append(asked, name)
			return false, "нельзя"
		},
	}
	out := &bytes.Buffer{}
	w := &lineWriter{w: nopCloser{out}}
	run := parseClaudeStreamJSON(strings.NewReader(strings.Join(lines, "\n")), cb, w)

	if run.res.Content != "Сначала прочитаю.\n\nГотово." {
		t.Errorf("content = %q: messages of one turn must be separated, subagent text excluded", run.res.Content)
	}
	if run.model != "claude-opus-5-5" || run.isError || !run.gotResult {
		t.Errorf("run = %+v", run)
	}
	// Calls come strictly paired, in tool_use order, even though the
	// results of the parallel pair arrived reversed.
	want := []string{"call Read a.txt", "result Read A", "call Read b.txt", "result Read B", "call Bash .", "result Bash denied"}
	if strings.Join(events, " | ") != strings.Join(want, " | ") {
		t.Errorf("events:\n got  %v\n want %v", events, want)
	}
	if len(asked) != 1 || asked[0] != "Bash" {
		t.Errorf("asked = %v", asked)
	}

	replies := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(replies) != 2 {
		t.Fatalf("replies: %v", replies)
	}
	var deny struct {
		Response struct {
			Subtype   string `json:"subtype"`
			RequestID string `json:"request_id"`
			Response  struct {
				Behavior string `json:"behavior"`
				Message  string `json:"message"`
			} `json:"response"`
		} `json:"response"`
	}
	_ = json.Unmarshal([]byte(replies[0]), &deny)
	if deny.Response.RequestID != "r1" || deny.Response.Response.Behavior != "deny" || deny.Response.Response.Message != "нельзя" {
		t.Errorf("deny reply = %s", replies[0])
	}
	// An unknown request still gets an answer, or claude would hang on it.
	if !strings.Contains(replies[1], `"request_id":"r2"`) || !strings.Contains(replies[1], `"subtype":"error"`) {
		t.Errorf("unknown request reply = %s", replies[1])
	}
	if !w.closed {
		t.Error("stdin must be closed after result, or claude waits for more input")
	}
}

// Without an approver nothing is allowed: a silent yes to edits and commands
// is the one outcome that must never happen by accident.
func TestControlRequestWithoutApproverDenies(t *testing.T) {
	reply := answerControlRequest("r", json.RawMessage(`{"subtype":"can_use_tool","tool_name":"Write","input":{"file_path":"/x"}}`), StreamCallbacks{})
	raw, _ := json.Marshal(reply)
	if !strings.Contains(string(raw), `"behavior":"deny"`) {
		t.Fatalf("reply = %s", raw)
	}
	allow := answerControlRequest("r", json.RawMessage(`{"subtype":"can_use_tool","tool_name":"Write","input":{"file_path":"/x"}}`),
		StreamCallbacks{ApproveDelegatedTool: func(string, json.RawMessage) (bool, string) { return true, "" }})
	raw, _ = json.Marshal(allow)
	if !strings.Contains(string(raw), `"behavior":"allow"`) || !strings.Contains(string(raw), `"updatedInput":{"file_path":"/x"}`) {
		t.Fatalf("allow reply = %s", raw)
	}
}

// A CLI error comes as a normal `result` with is_error. It used to be printed
// as the model's answer ("API Error: 400 ... Run 'claude update'").
func TestStreamReturnsCLIErrorAsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary")
	}
	bin := filepath.Join(t.TempDir(), "claude")
	writeFakeClaude(t, bin, `read -r line
echo '{"type":"result","subtype":"success","is_error":true,"result":"API Error: 400 Claude Code 2.1.220 does not support this model; version 2.1.280 or newer is required."}'`)
	cli := &ClaudeCLIClient{Path: bin, Version: "2.1.220", Model: "claude-opus-5-5"}
	var text strings.Builder
	_, err := cli.Stream(context.Background(), []AIMessage{{Role: "user", Content: "привет"}}, nil,
		StreamCallbacks{OnText: func(s string) { text.WriteString(s) }})
	if err == nil || !strings.Contains(err.Error(), "2.1.280 or newer") || !strings.Contains(err.Error(), "claude 2.1.220") {
		t.Fatalf("err = %v", err)
	}
	if text.Len() != 0 {
		t.Errorf("error text leaked into the answer: %q", text.String())
	}
}

// The whole protocol against a live process: the prompt arrives as a
// stream-json user message, the permission answer reaches claude's stdin, and
// claude exits once stdin closes after the result.
func TestStreamPermissionRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	writeFakeClaude(t, bin, `read -r first
case "$first" in *'"type":"user"'*) ;; *) echo "not a user message: $first" >&2; exit 3;; esac
case "$first" in *'привет'*) ;; *) echo "prompt lost: $first" >&2; exit 3;; esac
echo '{"type":"control_request","request_id":"q1","request":{"subtype":"can_use_tool","tool_name":"Edit","input":{"file_path":"a.txt"}}}'
read -r answer
case "$answer" in *'"behavior":"allow"'*) verdict=allowed;; *) verdict=denied;; esac
echo '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"'$verdict'"}}}'
echo '{"type":"result","subtype":"success","is_error":false,"result":"'$verdict'"}'
read -r rest || exit 0
echo "stdin still open after result" >&2; exit 4`)
	cli := &ClaudeCLIClient{Path: bin, Version: "9.9.9"}
	res, err := cli.Stream(context.Background(), []AIMessage{{Role: "user", Content: "привет"}}, nil, StreamCallbacks{
		ApproveDelegatedTool: func(name string, _ json.RawMessage) (bool, string) { return name == "Edit", "" },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "allowed" {
		t.Fatalf("content = %q", res.Content)
	}
}

func writeFakeClaude(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
