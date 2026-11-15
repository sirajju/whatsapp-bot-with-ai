package ai

import "context"

// NVIDIA NIM exposes an OpenAI-compatible endpoint with free tier access to Llama 3,
// Mixtral, Nemotron, etc.
type NVIDIA struct {
	Key   string
	Model string
}

func (n *NVIDIA) Name() string { return "nvidia" }

func (n *NVIDIA) Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error) {
	return chatOpenAICompat(ctx, "https://integrate.api.nvidia.com/v1", n.Key, n.Model, system, history, prompt, nil)
}
