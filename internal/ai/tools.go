package ai

import "context"

// ToolSpec is the function specification sent to the provider. Shape matches
// OpenAI's /chat/completions "tools" parameter — Groq, OpenRouter, NVIDIA
// and OpenAI itself all accept this verbatim.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// ToolCall is returned by the model when it wants a function run.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON string the handler decodes
}

// RawMsg is a provider-level message. Roles: "system", "user", "assistant", "tool".
// For role=assistant, ToolCalls may be set instead of Content.
// For role=tool, ToolCallID identifies which call this result answers.
type RawMsg struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

// RawChatResponse is the parsed assistant message.
type RawChatResponse struct {
	Content   string
	ToolCalls []ToolCall
}

// TooledProvider is implemented by providers that support function calling.
// Only call it when len(tools) > 0; otherwise use Provider.Chat.
type TooledProvider interface {
	Provider
	RawChat(ctx context.Context, messages []RawMsg, tools []ToolSpec) (RawChatResponse, error)
}
