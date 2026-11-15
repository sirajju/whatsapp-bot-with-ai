package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

type Anthropic struct {
	Key   string
	Model string
}

func (a *Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error) {
	msgs := make([]map[string]string, 0, len(history)+1)
	for _, m := range history {
		role := m.Role
		if role == "system" {
			continue
		}
		msgs = append(msgs, map[string]string{"role": role, "content": m.Content})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": prompt})

	body := map[string]any{
		"model":      a.Model,
		"max_tokens": 1024,
		"messages":   msgs,
	}
	if system != "" {
		body["system"] = system
	}
	out, err := postJSON(ctx, "https://api.anthropic.com/v1/messages",
		map[string]string{
			"x-api-key":         a.Key,
			"anthropic-version": "2023-06-01",
		}, body)
	if err != nil {
		return "", err
	}
	var r struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return "", err
	}
	for _, c := range r.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic: no text content")
}
