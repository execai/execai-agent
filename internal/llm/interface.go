// Unified interface for LLM clients: the ExecAI gateway (AICoreClient)
// and external subscriptions (GLMClient, AnthropicClient, OpenAIClient).
//
// The Agent holds a StreamingLLM reference without knowing where exactly requests go.
// Switching is done by replacing the Client on /use <provider> in the TUI.
package llm

import "context"

// StreamingLLM is the standard LLM-provider contract for the tool-use loop.
type StreamingLLM interface {
	Stream(ctx context.Context, messages []AIMessage, tools []map[string]any, cb StreamCallbacks) (*StreamResult, error)
}

// Ensure AICoreClient (our base one) implements the interface.
var _ StreamingLLM = (*AICoreClient)(nil)
