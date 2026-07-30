// Registry of the agent's local tools (like Claude Code: Read, Write, Edit,
// Bash, Grep, Glob, LS, WebFetch). Each tool is a separate file in this
// package, registered in Default(). The LLM receives an array of Spec() in
// the OpenAI tool definitions format.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Tool is an executable tool. Implementation responsibilities:
// - Spec returns the JSON-Schema description to send to the LLM.
// - Execute receives arguments as json.RawMessage (whatever the model put in
//   tool_call.arguments — usually valid JSON, but occasionally the model sends
//   broken JSON, in which case Execute returns an error the agent shows to the model).
// - RequiresApproval returns true if the user must be asked before execution.
//   Read-only tools return false; Bash depends on the command
//   (see the tool with the dynamic check).
type Tool interface {
	Spec() Spec
	Execute(ctx context.Context, args json.RawMessage) (string, error)
	RequiresApproval(args json.RawMessage) bool
}

// StreamingTool is an optional interface for tools that can emit output
// incrementally (Bash). If a tool does not implement it, the agent uses the
// regular Execute and the UI gets the whole result at once upon completion.
type StreamingTool interface {
	Tool
	ExecuteStream(ctx context.Context, args json.RawMessage, onChunk func(string)) (string, error)
}

type Spec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON-Schema (Draft-07 compatible)
}

// ToolDefinition is the format expected by the OpenAI Chat Completions API in
// the tools field: [{type:"function", function:{name, description, parameters}}].
type ToolDefinition struct {
	Type     string `json:"type"`
	Function Spec   `json:"function"`
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Spec().Name] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Definitions is the list of tool definitions in OpenAI/Anthropic format for
// sending to the LLM. Sorted by name for prompt stability (cache hits).
func (r *Registry) Definitions() []ToolDefinition {
	out := make([]ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, ToolDefinition{Type: "function", Function: t.Spec()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Function.Name < out[j].Function.Name })
	return out
}

func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.tools))
	for n := range r.tools {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Default is the standard set like Claude Code's, without a browser.
// CWD is passed to the tools that operate on the working directory.
func Default(cwd string) *Registry {
	r := NewRegistry()
	r.Register(&ReadTool{cwd: cwd})
	r.Register(&WriteTool{cwd: cwd})
	r.Register(&EditTool{cwd: cwd})
	r.Register(&BashTool{cwd: cwd})
	r.Register(&GrepTool{cwd: cwd})
	r.Register(&GlobTool{cwd: cwd})
	r.Register(&LSTool{cwd: cwd})
	r.Register(&TreeTool{cwd: cwd})
	r.Register(&WebFetchTool{})
	r.Register(&TodoWriteTool{})
	r.Register(&ScheduleWakeupTool{})
	return r
}

// resolveJSON is a convenience helper for tools: parses args into the given struct.
func resolveJSON[T any](args json.RawMessage, dst *T) error {
	if len(args) == 0 || string(args) == "null" {
		return nil
	}
	if err := json.Unmarshal(args, dst); err != nil {
		return fmt.Errorf("неверный JSON в аргументах: %w", err)
	}
	return nil
}
