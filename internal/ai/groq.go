package ai

import "context"

// Groq is OpenAI-compatible with a generous free tier.
type Groq struct {
	Key   string
	Model string
}

func (g *Groq) Name() string { return "groq" }

func (g *Groq) Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error) {
	return chatOpenAICompat(ctx, "https://api.groq.com/openai/v1", g.Key, g.Model, system, history, prompt, nil)
}
