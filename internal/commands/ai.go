package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// AI binds the configured provider (may be nil if none enabled).
func AI(provider ai.Provider) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if provider == nil {
			return sendText(ctx, env.Cli, env.Chat,
				"No AI provider configured. Set OLLAMA_URL / GROQ_API_KEY / ANTHROPIC_API_KEY / OPENAI_API_KEY / GEMINI_API_KEY and restart.")
		}
		prompt := strings.TrimSpace(strings.Join(env.Args, " "))
		if prompt == "" {
			return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("usage: %sai <prompt>", env.Cfg.Prefix))
		}

		env.Log.Info().Str("provider", provider.Name()).Int("prompt_len", len(prompt)).Msg("ai call")
		reply, err := provider.Chat(ctx, prompt)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "AI error: "+err.Error())
		}
		if reply == "" {
			reply = "(empty response)"
		}
		return sendText(ctx, env.Cli, env.Chat, reply)
	}
}
