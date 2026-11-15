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

func (g *Gemini) Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error) {
	contents := make([]map[string]any, 0, len(history)+1)
	for _, m := range history {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		if role == "system" {
			continue
		}
		contents = append(contents, map[string]any{
			"role":  role,
			"parts": []map[string]any{{"text": m.Content}},
		})
	}
	contents = append(contents, map[string]any{
		"role":  "user",
		"parts": []map[string]any{{"text": prompt}},
	})

	body := map[string]any{"contents": contents}
	if system != "" {
		body["system_instruction"] = map[string]any{
			"parts": []map[string]any{{"text": system}},
		}
	}
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		g.Model, g.Key)
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
