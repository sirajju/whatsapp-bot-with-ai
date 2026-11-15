package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

type OpenAI struct {
	Key   string
	Model string
}

func (o *OpenAI) Name() string { return "openai" }

func (o *OpenAI) Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error) {
	msgs := make([]map[string]string, 0, len(history)+2)
	if system != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": system})
	}
	for _, m := range history {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": prompt})

	out, err := postJSON(ctx, "https://api.openai.com/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + o.Key},
		map[string]any{"model": o.Model, "messages": msgs})
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
		return "", fmt.Errorf("openai: no choices")
	}
	return r.Choices[0].Message.Content, nil
}
