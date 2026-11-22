package commands

import (
	"context"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/intercom"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// WallphoneAssistant triggers the official Gemini app/floaty on the wall phone via MQTT.
func WallphoneAssistant(ic *intercom.Bridge) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		if ic == nil {
			return sendText(ctx, env.Cli, env.Chat, "intercom/MQTT not available")
		}

		arg := "gemini"
		if len(env.Args) > 0 {
			arg = strings.ToLower(env.Args[0])
		}

		topic := "wallphone/control"
		var payload string
		var reply string

		switch arg {
		case "off", "dismiss", "stop", "back":
			payload = `{"action":"dismiss"}`
			reply = "📱 Dismissed Gemini on wall phone (back to WallPanel)"
		case "live":
			payload = `{"action":"live"}`
			reply = "🎙️ Started Gemini Live on wall phone"
		case "app":
			payload = `{"action":"gemini_app"}`
			reply = "📱 Opened full Gemini app on wall phone"
		default:
			payload = `{"action":"gemini"}`
			reply = "✨ Summoned Gemini Voice Assistant on wall phone"
		}

		if err := ic.Publish(topic, []byte(payload)); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "failed to trigger wall phone: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, reply)
	}
}
