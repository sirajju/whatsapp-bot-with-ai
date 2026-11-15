package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// Groq is OpenAI-compatible with a generous free tier.
type Groq struct {
	Key   string
	Model string
}

func (g *Groq) Name() string { return "groq" }

func (g *Groq) Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error) {
	msgs := make([]map[string]string, 0, len(history)+2)
	if system != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": system})
	}
	for _, m := range history {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": prompt})

	out, err := postJSON(ctx, "https://api.groq.com/openai/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + g.Key},
		map[string]any{"model": g.Model, "messages": msgs})
	if err != nil {
		return "", err
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return "", err
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("groq: no choices")
	}
	return r.Choices[0].Message.Content, nil
}
