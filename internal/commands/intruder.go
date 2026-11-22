package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/intercom"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Intruder handles !intruder / !check — performs an instant security audit
// correlating Wi-Fi presence with an on-demand camera snapshot from the wall phone.
func Intruder(ic *intercom.Bridge) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}

		_ = sendText(ctx, env.Cli, env.Chat, "🛡️ Running security check & waking camera…")

		// 1. Check presence
		var s presenceStatusResp
		_ = presenceGet(ctx, "/api/status", &s)

		homeCount := s.Counts["home"]
		var presenceSummary string
		if homeCount == 0 {
			presenceSummary = "🚨 *House is EMPTY* (0 devices connected to Wi-Fi)"
		} else {
			var names []string
			for _, d := range s.Home {
				name := d.Label
				if name == "" {
					name = d.Hostname
				}
				if name == "" {
					name = d.MAC
				}
				names = append(names, name)
			}
			presenceSummary = fmt.Sprintf("🏠 *House is OCCUPIED* (%d device(s): %s)", homeCount, strings.Join(names, ", "))
		}

		caption := fmt.Sprintf(
			"🛡️ *Security & Intruder Report*\n\n"+
				"• %s\n"+
				"• *Timestamp*: %s\n"+
				"• *Camera*: Wall-Phone Living Room",
			presenceSummary,
			time.Now().Format("15:04:05 Mon, 02 Jan"),
		)

		// 2. Capture living room camera snapshot
		img, mime, err := CaptureSnapshot(ctx, env.Cfg, ic)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, caption+"\n\n⚠️ Camera snapshot failed: "+err.Error())
		}

		return SendImageWithCaption(ctx, env.Cli, env.Chat, img, mime, caption)
	}
}
