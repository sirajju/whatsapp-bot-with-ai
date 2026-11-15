package ai

import (
	"context"
	"encoding/json"
	"strings"
)

// Ollama hits a local server at OLLAMA_URL (e.g. http://localhost:11434).
type Ollama struct {
	URL   string
	Model string
}

func (o *Ollama) Name() string { return "ollama" }

func (o *Ollama) Chat(ctx context.Context, prompt string) (string, error) {
	body := map[string]any{
		"model":  o.Model,
		"prompt": prompt,
		"stream": false,
	}
	url := strings.TrimRight(o.URL, "/") + "/api/generate"
	out, err := postJSON(ctx, url, nil, body)
	if err != nil {
		return "", err
	}
	var r struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return "", err
	}
	return r.Response, nil
}
