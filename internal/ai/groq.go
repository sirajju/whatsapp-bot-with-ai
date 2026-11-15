package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// Groq exposes an OpenAI-compatible chat endpoint. Free tier is generous.
type Groq struct {
	Key   string
	Model string
}

func (g *Groq) Name() string { return "groq" }

func (g *Groq) Chat(ctx context.Context, prompt string) (string, error) {
	body := map[string]any{
		"model": g.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	out, err := postJSON(ctx, "https://api.groq.com/openai/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + g.Key}, body)
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
