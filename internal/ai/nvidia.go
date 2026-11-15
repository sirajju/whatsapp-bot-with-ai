package ai

import "context"

// NVIDIA NIM OpenAI-compatible endpoint, free dev tier.
type NVIDIA struct {
	Key   string
	Model string
}

func (n *NVIDIA) Name() string { return "nvidia" }

func (n *NVIDIA) Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error) {
	return chatOpenAICompat(ctx, "https://integrate.api.nvidia.com/v1", n.Key, n.Model, system, history, prompt, nil)
}

func (n *NVIDIA) RawChat(ctx context.Context, messages []RawMsg, tools []ToolSpec) (RawChatResponse, error) {
	return rawChatOpenAICompat(ctx, "https://integrate.api.nvidia.com/v1", n.Key, n.Model, messages, tools, nil)
}
