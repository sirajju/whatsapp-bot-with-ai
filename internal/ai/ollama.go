package ai

import (
	"context"
	"encoding/json"
	"strings"
)

// Ollama hits a local server at OLLAMA_URL (e.g. http://localhost:11434).
// Uses the /api/chat endpoint so history works.
type Ollama struct {
	URL   string
	Model string
}

func (o *Ollama) Name() string { return "ollama" }

func (o *Ollama) Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error) {
	msgs := make([]map[string]string, 0, len(history)+2)
	if system != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": system})
	}
	for _, m := range history {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": prompt})

	body := map[string]any{
		"model":    o.Model,
		"messages": msgs,
		"stream":   false,
	}
	url := strings.TrimRight(o.URL, "/") + "/api/chat"
	out, err := postJSON(ctx, url, nil, body)
	if err != nil {
		return "", err
	}
	var r struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return "", err
	}
	return r.Message.Content, nil
}
