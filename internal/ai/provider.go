// Package ai defines the Provider interface and implementations for each LLM backend.
// Pick() returns the best configured provider or nil if none set.
package ai

import "context"

// Msg is one history turn passed to a provider.
type Msg struct {
	Role    string // "user" | "assistant" | "system"
	Content string
}

// Provider is the common surface for an LLM backend.
type Provider interface {
	Name() string
	// Chat sends system + prior history + the new user prompt, returns the reply.
	Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error)
}

// Config mirrors the subset of env the ai package needs.
type Config struct {
	OllamaURL    string
	OllamaModel  string
	GroqKey      string
	GroqModel    string
	AnthropicKey string
	AnthropicMdl string
	OpenAIKey    string
	OpenAIModel  string
	GeminiKey    string
	GeminiModel  string
}

// Pick returns the highest-priority configured provider, or nil if none.
// Order: Ollama (local) > Groq (free) > Anthropic > OpenAI > Gemini.
func Pick(c Config) Provider {
	if c.OllamaURL != "" {
		return &Ollama{URL: c.OllamaURL, Model: c.OllamaModel}
	}
	if c.GroqKey != "" {
		return &Groq{Key: c.GroqKey, Model: c.GroqModel}
	}
	if c.AnthropicKey != "" {
		return &Anthropic{Key: c.AnthropicKey, Model: c.AnthropicMdl}
	}
	if c.OpenAIKey != "" {
		return &OpenAI{Key: c.OpenAIKey, Model: c.OpenAIModel}
	}
	if c.GeminiKey != "" {
		return &Gemini{Key: c.GeminiKey, Model: c.GeminiModel}
	}
	return nil
}
