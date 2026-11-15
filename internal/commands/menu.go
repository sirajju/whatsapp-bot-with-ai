package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Menu returns a handler that lists registered commands using the router's help map.
// A second-stage registration (after all commands are wired) passes the map in.
func Menu(help map[string]string) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		names := make([]string, 0, len(help))
		for n := range help {
			names = append(names, n)
		}
		sort.Strings(names)

		var b strings.Builder
		fmt.Fprintf(&b, "*wabot* — commands (prefix: `%s`)\n\n", env.Cfg.Prefix)
		for _, n := range names {
			fmt.Fprintf(&b, "• `%s%s` — %s\n", env.Cfg.Prefix, n, help[n])
		}
		return sendText(ctx, env.Cli, env.Chat, b.String())
	}
}
