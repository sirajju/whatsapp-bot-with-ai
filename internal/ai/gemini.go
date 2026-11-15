package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

type Gemini struct {
	Key   string
	Model string
}

func (g *Gemini) Name() string { return "gemini" }

func (g *Gemini) Chat(ctx context.Context, prompt string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		g.Model, g.Key)
	body := map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]any{{"text": prompt}}},
		},
	}
	out, err := postJSON(ctx, url, nil, body)
	if err != nil {
		return "", err
	}
	var r struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return "", err
	}
	if len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: empty response")
	}
	return r.Candidates[0].Content.Parts[0].Text, nil
}
