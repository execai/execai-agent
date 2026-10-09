package agent

import "encoding/json"

// Delegated tools — the tools of a CLI the agent hands the turn to
// (claude-cli). The CLI runs them itself, but the person must see them and
// consent to them exactly as to execai's own: same cards, same questions, same
// security level, and an "always" given to one applies to the other.
//
// So each call is translated into the matching execai tool. Claude Code says
// `file_path`, execai says `path`; with the translation the existing perimeter
// rules (project directory is quiet, outside asks, secrets always ask) and the
// saved permissions apply without a second rulebook.

// delegatedToNative maps a Claude Code tool call onto the execai tool with the
// same effect. Unknown tools keep their name and input.
func delegatedToNative(name string, input json.RawMessage) (string, json.RawMessage) {
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	str := func(k string) string {
		v, _ := in[k].(string)
		return v
	}
	pick := func(pairs ...string) json.RawMessage {
		out := map[string]any{}
		for i := 0; i+1 < len(pairs); i += 2 {
			if v, ok := in[pairs[i]]; ok && v != nil && v != "" {
				out[pairs[i+1]] = v
			}
		}
		raw, _ := json.Marshal(out)
		return raw
	}
	switch name {
	case "Bash":
		// Not the timeout: claude counts it in milliseconds, execai in seconds.
		return "Bash", pick("command", "command", "description", "description")
	case "Read":
		return "Read", pick("file_path", "path")
	case "Write":
		return "Write", pick("file_path", "path", "content", "content")
	case "Edit":
		return "Edit", pick("file_path", "path", "old_string", "old_string", "new_string", "new_string", "replace_all", "replace_all")
	case "MultiEdit":
		return "Edit", pick("file_path", "path")
	case "NotebookEdit":
		if str("notebook_path") != "" {
			return "Edit", pick("notebook_path", "path")
		}
	case "Glob":
		return "Glob", pick("pattern", "pattern", "path", "path")
	case "Grep":
		return "Grep", pick("pattern", "pattern", "path", "path")
	case "LS":
		return "LS", pick("path", "path")
	case "WebFetch":
		return "WebFetch", pick("url", "url")
	case "WebSearch":
		return "WebSearch", pick("query", "query")
	case "Agent":
		return "Task", input
	}
	return name, input
}

// approveDelegated answers a permission request of a delegated CLI. It is the
// same decision runTool makes before an execai tool, applied to the
// translated call.
func (a *Agent) approveDelegated(name string, input json.RawMessage) (bool, string) {
	native, args := delegatedToNative(name, input)
	t, ok := a.Tools.Get(native)
	if ok && !t.RequiresApproval(args) {
		return true, ""
	}
	if !ok {
		// A tool execai does not have (an MCP tool of claude, a tool the
		// read-only registry of a subagent lacks): ask, never wave through.
		t = nil
	}
	if a.approve(t, native, args) {
		return true, ""
	}
	return false, "Пользователь отклонил выполнение этого инструмента."
}
