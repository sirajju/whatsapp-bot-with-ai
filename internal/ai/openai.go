package ai

import "context"

type OpenAI struct {
	Key   string
	Model string
}

func (o *OpenAI) Name() string { return "openai" }

func (o *OpenAI) Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error) {
	return chatOpenAICompat(ctx, "https://api.openai.com/v1", o.Key, o.Model, system, history, prompt, nil)
}

func (o *OpenAI) RawChat(ctx context.Context, messages []RawMsg, tools []ToolSpec) (RawChatResponse, error) {
	return rawChatOpenAICompat(ctx, "https://api.openai.com/v1", o.Key, o.Model, messages, tools, nil)
}
