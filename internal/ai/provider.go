// Package ai defines the Provider interface and implementations for each LLM backend.
// Pick() returns the configured provider or nil if none set.
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
	Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error)
}

// Config mirrors the subset of env the ai package needs.
type Config struct {
	// Preferred provider by explicit name (empty = first available in priority order).
	Preferred string

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
	NvidiaKey    string
	NvidiaModel  string
	OpenRouterKey   string
	OpenRouterModel string
	OpencodeArgv    []string // empty = opencode disabled
}

// Pick returns the chosen provider, or nil if none configured.
// If Preferred is set and that one is configured, it wins. Otherwise priority:
// Opencode > Ollama > Groq > NVIDIA > OpenRouter > Anthropic > OpenAI > Gemini.
func Pick(c Config) Provider {
	candidates := map[string]func() Provider{
		"opencode":   func() Provider { if len(c.OpencodeArgv) == 0 { return nil }; return &Opencode{Argv: c.OpencodeArgv} },
		"ollama":     func() Provider { if c.OllamaURL == "" { return nil }; return &Ollama{URL: c.OllamaURL, Model: c.OllamaModel} },
		"groq":       func() Provider { if c.GroqKey == "" { return nil }; return &Groq{Key: c.GroqKey, Model: c.GroqModel} },
		"nvidia":     func() Provider { if c.NvidiaKey == "" { return nil }; return &NVIDIA{Key: c.NvidiaKey, Model: c.NvidiaModel} },
		"openrouter": func() Provider { if c.OpenRouterKey == "" { return nil }; return &OpenRouter{Key: c.OpenRouterKey, Model: c.OpenRouterModel} },
		"anthropic":  func() Provider { if c.AnthropicKey == "" { return nil }; return &Anthropic{Key: c.AnthropicKey, Model: c.AnthropicMdl} },
		"openai":     func() Provider { if c.OpenAIKey == "" { return nil }; return &OpenAI{Key: c.OpenAIKey, Model: c.OpenAIModel} },
		"gemini":     func() Provider { if c.GeminiKey == "" { return nil }; return &Gemini{Key: c.GeminiKey, Model: c.GeminiModel} },
	}
	if c.Preferred != "" {
		if f, ok := candidates[c.Preferred]; ok {
			if p := f(); p != nil {
				return p
			}
		}
	}
	for _, name := range []string{"opencode", "ollama", "groq", "nvidia", "openrouter", "anthropic", "openai", "gemini"} {
		if p := candidates[name](); p != nil {
			return p
		}
	}
	return nil
}
