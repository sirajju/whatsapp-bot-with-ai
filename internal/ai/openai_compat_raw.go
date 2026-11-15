package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// rawChatOpenAICompat issues a raw OpenAI-shape chat completion that supports:
//   - full message array (including tool role + tool_calls)
//   - optional tools parameter for function calling
// Returns the assistant's content plus any tool_calls it issued.
func rawChatOpenAICompat(
	ctx context.Context, baseURL, key, model string,
	messages []RawMsg, tools []ToolSpec,
	extraHeaders map[string]string,
) (RawChatResponse, error) {
	msgs := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		item := map[string]any{"role": m.Role}
		if m.Content != "" {
			item["content"] = m.Content
		}
		if len(m.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				calls = append(calls, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})
			}
			item["tool_calls"] = calls
			// assistant with tool_calls needs content=null, which we achieve by not setting it
		}
		if m.ToolCallID != "" {
			item["tool_call_id"] = m.ToolCallID
		}
		msgs = append(msgs, item)
	}

	body := map[string]any{
		"model":    model,
		"messages": msgs,
	}
	if len(tools) > 0 {
		t := make([]map[string]any, 0, len(tools))
		for _, ts := range tools {
			t = append(t, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        ts.Name,
					"description": ts.Description,
					"parameters":  ts.Parameters,
				},
			})
		}
		body["tools"] = t
	}

	headers := map[string]string{"Authorization": "Bearer " + key}
	for k, v := range extraHeaders {
		headers[k] = v
	}

	out, err := postJSON(ctx, baseURL+"/chat/completions", headers, body)
	if err != nil {
		return RawChatResponse{}, err
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return RawChatResponse{}, err
	}
	if len(r.Choices) == 0 {
		return RawChatResponse{}, fmt.Errorf("no choices in response")
	}
	m := r.Choices[0].Message
	var calls []ToolCall
	for _, tc := range m.ToolCalls {
		calls = append(calls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return RawChatResponse{Content: m.Content, ToolCalls: calls}, nil
}
