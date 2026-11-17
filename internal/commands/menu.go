package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Menu returns a handler that lists registered commands using the router's help map.
//
// Audience-aware:
//   - non-owners see only non-owner commands (the owner-only ones are hidden).
//   - owners see everything, grouped: normal commands first, then a separator
//     and the owner-only block.
//
// A command is treated as owner-only when its help string starts with
// "owner:" (case-insensitive) — the same marker used throughout main.go when
// registering privileged commands, so no new tagging mechanism is needed.
func Menu(help map[string]string) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		var normal, ownerOnly []string
		for n, h := range help {
			if isOwnerHelp(h) {
				ownerOnly = append(ownerOnly, n)
			} else {
				normal = append(normal, n)
			}
		}
		sort.Strings(normal)
		sort.Strings(ownerOnly)

		var b strings.Builder
		fmt.Fprintf(&b, "*wabot* — commands (prefix: `%s`)\n\n", env.Cfg.Prefix)
		for _, n := range normal {
			fmt.Fprintf(&b, "• `%s%s` — %s\n", env.Cfg.Prefix, n, help[n])
		}
		if env.IsOwner && len(ownerOnly) > 0 {
			b.WriteString("\n━━━━━ *owner-only* ━━━━━\n")
			for _, n := range ownerOnly {
				// Strip the "owner:" marker so the owner-only section doesn't
				// repeat "owner:" on every line; the header already says it.
				desc := strings.TrimSpace(strings.TrimPrefix(
					strings.TrimPrefix(help[n], "owner:"), "Owner:"))
				fmt.Fprintf(&b, "• `%s%s` — %s\n", env.Cfg.Prefix, n, desc)
			}
		}
		return sendText(ctx, env.Cli, env.Chat, b.String())
	}
}

// isOwnerHelp returns true when a help string starts with "owner:".
func isOwnerHelp(h string) bool {
	h = strings.TrimSpace(h)
	return strings.HasPrefix(strings.ToLower(h), "owner:")
}
