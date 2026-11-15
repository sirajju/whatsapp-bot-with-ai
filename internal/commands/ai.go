package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

const historyWindow = 10

// AI handles /ai in three modes:
//   /ai              (no args)  → turn on AI session mode (bare messages go to AI till /endai)
//   /ai <prompt>                → single-turn
func AI(provider ai.Provider) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if provider == nil {
			return sendText(ctx, env.Cli, env.Chat,
				"No AI provider configured. Set OLLAMA_URL / GROQ_API_KEY / ANTHROPIC_API_KEY / OPENAI_API_KEY / GEMINI_API_KEY and restart.")
		}
		prompt := strings.TrimSpace(strings.Join(env.Args, " "))
		if prompt == "" {
			// Session-mode toggle ON
			if err := env.Store.SetAIMode(ctx, env.Chat.String(), true); err != nil {
				return sendText(ctx, env.Cli, env.Chat, "ai mode: "+err.Error())
			}
			persona := env.Store.GetPersona(ctx, env.Chat.String())
			if persona == "" {
				persona = "default"
			}
			return sendText(ctx, env.Cli, env.Chat,
				fmt.Sprintf("🤖 *AI mode ON* (persona: *%s*)\nJust type — every message goes to the AI until you send *%sendai*.", persona, env.Cfg.Prefix))
		}
		// Single-turn
		return runAI(ctx, env, provider, prompt)
	}
}

// EndAI stops AI session mode for this chat.
func EndAI() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if err := env.Store.SetAIMode(ctx, env.Chat.String(), false); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "endai: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "🤖 AI mode OFF")
	}
}

// AIFallback is wired as the router's fallback handler. It fires on every
// non-command message, and only acts when AI mode is active for the chat.
func AIFallback(provider ai.Provider) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if provider == nil {
			return nil
		}
		if !env.Store.IsAIMode(ctx, env.Chat.String()) {
			return nil
		}
		prompt := strings.TrimSpace(env.RawText)
		if prompt == "" {
			return nil
		}
		return runAI(ctx, env, provider, prompt)
	}
}

// ClearAI wipes the AI chat history for the current chat.
func ClearAI() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if err := env.Store.ClearHistory(ctx, env.Chat.String()); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "clear: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "AI memory cleared for this chat")
	}
}

// SetPersona switches the per-chat persona.
func SetPersona() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			current := env.Store.GetPersona(ctx, env.Chat.String())
			if current == "" {
				current = "default"
			}
			return sendText(ctx, env.Cli, env.Chat,
				fmt.Sprintf("current persona: *%s*\navailable: %s\nchange: %spersona <name>",
					current, strings.Join(ai.PersonaNames(), ", "), env.Cfg.Prefix))
		}
		name := strings.ToLower(env.Args[0])
		if _, ok := ai.Personas[name]; !ok {
			return sendText(ctx, env.Cli, env.Chat,
				"unknown persona. Available: "+strings.Join(ai.PersonaNames(), ", "))
		}
		if err := env.Store.SetPersona(ctx, env.Chat.String(), name); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "persona: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "persona set to *"+name+"*")
	}
}

// runAI is the shared path for both single-turn and session-mode calls.
// It sends a "thinking" placeholder, calls the provider with history + persona,
// then sends the reply and persists the exchange.
func runAI(ctx context.Context, env *router.Env, provider ai.Provider, prompt string) error {
	_ = sendText(ctx, env.Cli, env.Chat, "🤔 thinking…")

	chatJID := env.Chat.String()
	persona := env.Store.GetPersona(ctx, chatJID)
	system := ai.SystemPrompt(persona)

	hist, _ := env.Store.History(ctx, chatJID, historyWindow)
	storeHistory := make([]ai.Msg, 0, len(hist))
	for _, h := range hist {
		storeHistory = append(storeHistory, ai.Msg{Role: h.Role, Content: h.Content})
	}

	env.Log.Info().Str("provider", provider.Name()).Str("persona", persona).Int("history", len(hist)).Msg("ai call")
	reply, err := provider.Chat(ctx, system, storeHistory, prompt)
	if err != nil {
		return sendText(ctx, env.Cli, env.Chat, "AI error: "+err.Error())
	}
	if reply == "" {
		reply = "(empty response)"
	}

	_ = env.Store.AppendHistory(ctx, chatJID, "user", prompt)
	_ = env.Store.AppendHistory(ctx, chatJID, "assistant", reply)
	return sendText(ctx, env.Cli, env.Chat, reply)
}
