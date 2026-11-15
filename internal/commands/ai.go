package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/aitools"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

const (
	historyWindow = 10
	maxToolIter   = 5
)

// AI handles /ai in two modes:
//   /ai              → turn on AI session mode (bare messages go to AI till /endai)
//   /ai <prompt>     → single-turn
func AI(provider ai.Provider, reg *aitools.Registry) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if provider == nil {
			return sendText(ctx, env.Cli, env.Chat,
				"No AI provider configured. Set OLLAMA_URL / GROQ_API_KEY / ANTHROPIC_API_KEY / OPENAI_API_KEY / GEMINI_API_KEY and restart.")
		}
		prompt := strings.TrimSpace(strings.Join(env.Args, " "))
		if prompt == "" {
			if err := env.Store.SetAIMode(ctx, env.Chat.String(), true); err != nil {
				return sendText(ctx, env.Cli, env.Chat, "ai mode: "+err.Error())
			}
			persona := env.Store.GetPersona(ctx, env.Chat.String())
			if persona == "" {
				persona = "default"
			}
			return sendText(ctx, env.Cli, env.Chat,
				fmt.Sprintf("🤖 *AI mode ON* (persona: *%s*, tools: enabled)\nJust type — every message goes to the AI until you send *%sendai*.", persona, env.Cfg.Prefix))
		}
		return runAI(ctx, env, provider, reg, prompt)
	}
}

func EndAI() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if err := env.Store.SetAIMode(ctx, env.Chat.String(), false); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "endai: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "🤖 AI mode OFF")
	}
}

// AIFallback is wired as the router's fallback. Fires on non-command messages in
// chats where AI mode is active.
func AIFallback(provider ai.Provider, reg *aitools.Registry) router.Handler {
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
		return runAI(ctx, env, provider, reg, prompt)
	}
}

func ClearAI() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if err := env.Store.ClearHistory(ctx, env.Chat.String()); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "clear: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "AI memory cleared for this chat")
	}
}

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

// WelcomeHook fires before dispatch on every message. On the first-ever DM from
// a chat it greets the user and turns AI mode on by default. Does not swallow
// the message, so the user's first words still reach the normal command/fallback
// path. Group chats are skipped — auto-enabling AI in a shared group would spam.
func WelcomeHook() router.Hook {
	return func(ctx context.Context, env *router.Env) bool {
		if env.IsGroup {
			return false
		}
		isNew, err := env.Store.MarkSeenIfNew(ctx, env.Chat.String())
		if err != nil || !isNew {
			return false
		}
		_ = env.Store.SetAIMode(ctx, env.Chat.String(), true)
		_ = sendText(ctx, env.Cli, env.Chat,
			"👋 Hi! *AI mode is ON* by default — just send any message.\n"+
				"Turn it off with *"+env.Cfg.Prefix+"endai*. See all commands with *"+env.Cfg.Prefix+"menu*.")
		return false
	}
}

// runAI is the shared path. When the provider supports tool calling AND the registry
// is non-nil, it runs a tool loop. Otherwise it falls back to a plain Chat() call.
// On any provider error, AI mode is disabled for this chat so the user isn't
// stuck in a broken loop — they can re-enable with /ai.
func runAI(ctx context.Context, env *router.Env, provider ai.Provider, reg *aitools.Registry, prompt string) error {
	_ = sendText(ctx, env.Cli, env.Chat, "🤔 thinking…")

	chatJID := env.Chat.String()
	persona := env.Store.GetPersona(ctx, chatJID)
	system := ai.SystemPrompt(persona)

	hist, _ := env.Store.History(ctx, chatJID, historyWindow)
	storeHistory := make([]ai.Msg, 0, len(hist))
	for _, h := range hist {
		storeHistory = append(storeHistory, ai.Msg{Role: h.Role, Content: h.Content})
	}

	tp, canTool := provider.(ai.TooledProvider)
	hasTools := reg != nil && len(reg.Specs()) > 0

	var reply string
	var err error
	if canTool && hasTools {
		reply, err = runAIToolLoop(ctx, env, tp, reg, system, storeHistory, prompt)
	} else {
		reply, err = provider.Chat(ctx, system, storeHistory, prompt)
	}
	if err != nil {
		_ = env.Store.SetAIMode(ctx, chatJID, false)
		return sendText(ctx, env.Cli, env.Chat,
			"⚠️ AI error: "+err.Error()+"\nAI mode turned off. Send *"+env.Cfg.Prefix+"ai* to re-enable.")
	}
	if reply == "" {
		reply = "(empty response)"
	}
	reply = ai.FormatForWhatsApp(reply)

	_ = env.Store.AppendHistory(ctx, chatJID, "user", prompt)
	_ = env.Store.AppendHistory(ctx, chatJID, "assistant", reply)
	return sendText(ctx, env.Cli, env.Chat, reply)
}

// runAIToolLoop drives the OpenAI-style function-calling loop.
func runAIToolLoop(ctx context.Context, env *router.Env, provider ai.TooledProvider,
	reg *aitools.Registry, system string, history []ai.Msg, prompt string) (string, error) {

	msgs := make([]ai.RawMsg, 0, len(history)+4)
	if system != "" {
		msgs = append(msgs, ai.RawMsg{Role: "system", Content: system})
	}
	for _, h := range history {
		msgs = append(msgs, ai.RawMsg{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, ai.RawMsg{Role: "user", Content: prompt})

	toolEnv := &aitools.ToolEnv{
		Ctx:       ctx,
		Store:     env.Store,
		Cfg:       env.Cfg,
		Cli:       env.Cli,
		SenderJID: env.Sender.String(),
		ChatJID:   env.Chat.String(),
		Log:       env.Log,
	}

	specs := reg.Specs()

	for i := 0; i < maxToolIter; i++ {
		resp, err := provider.RawChat(ctx, msgs, specs)
		if err != nil {
			return "", err
		}
		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}
		env.Log.Info().Int("tool_calls", len(resp.ToolCalls)).Msg("ai requested tools")
		msgs = append(msgs, ai.RawMsg{Role: "assistant", ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			result := reg.Invoke(toolEnv, tc.Name, tc.Arguments)
			msgs = append(msgs, ai.RawMsg{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result,
			})
		}
	}
	return "(tool loop exceeded " + fmt.Sprint(maxToolIter) + " iterations)", nil
}
