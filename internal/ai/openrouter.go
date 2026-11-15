package ai

import "context"

// OpenRouter is an aggregator: one API key, hundreds of models from many vendors.
type OpenRouter struct {
	Key   string
	Model string
}

func (o *OpenRouter) Name() string { return "openrouter" }

var openRouterHeaders = map[string]string{
	"HTTP-Referer": "https://github.com/sirajju/whatsapp-bot-with-ai",
	"X-Title":      "wabot",
}

func (o *OpenRouter) Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error) {
	return chatOpenAICompat(ctx, "https://openrouter.ai/api/v1", o.Key, o.Model, system, history, prompt, openRouterHeaders)
}

func (o *OpenRouter) RawChat(ctx context.Context, messages []RawMsg, tools []ToolSpec) (RawChatResponse, error) {
	return rawChatOpenAICompat(ctx, "https://openrouter.ai/api/v1", o.Key, o.Model, messages, tools, openRouterHeaders)
}
