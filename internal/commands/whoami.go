package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Whoami reports the JID(s) the bot sees you as, so you can tell exactly which
// value needs to be in OWNERS for owner commands to work.
func Whoami() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		var b strings.Builder
		fmt.Fprintf(&b, "🪪 *Who you are to me*\n")
		fmt.Fprintf(&b, "sender JID: `%s`\n", env.Sender.String())
		fmt.Fprintf(&b, "sender user: `%s`  (compare this to OWNERS)\n", env.Sender.User)
		if alt := env.Msg.Info.SenderAlt; !alt.IsEmpty() && alt.User != "" {
			fmt.Fprintf(&b, "sender alt: `%s`  (lid→phone or phone→lid mapping)\n", alt.String())
		}
		fmt.Fprintf(&b, "is owner: *%v*\n", env.IsOwner)
		fmt.Fprintf(&b, "is group: %v\n", env.IsGroup)
		fmt.Fprintf(&b, "owners configured: `%s`", strings.Join(env.Cfg.Owners, ", "))
		return sendText(ctx, env.Cli, env.Chat, b.String())
	}
}
