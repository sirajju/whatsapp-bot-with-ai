package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// chatOpenAICompat sends an OpenAI-shape /chat/completions call to any compatible endpoint.
// Groq, OpenRouter, NVIDIA NIM, and OpenAI itself all accept this shape.
func chatOpenAICompat(ctx context.Context, baseURL, key, model string, system string, history []Msg, prompt string, extraHeaders map[string]string) (string, error) {
	msgs := make([]map[string]string, 0, len(history)+2)
	if system != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": system})
	}
	for _, m := range history {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": prompt})

	headers := map[string]string{"Authorization": "Bearer " + key}
	for k, v := range extraHeaders {
		headers[k] = v
	}

	out, err := postJSON(ctx, baseURL+"/chat/completions", headers,
		map[string]any{"model": model, "messages": msgs})
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
		return "", fmt.Errorf("no choices in response")
	}
	return r.Choices[0].Message.Content, nil
}
