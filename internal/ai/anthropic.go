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

func (a *Anthropic) Chat(ctx context.Context, prompt string) (string, error) {
	body := map[string]any{
		"model":      a.Model,
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
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
