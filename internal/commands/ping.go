package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

func Ping(start time.Time) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		up := time.Since(start).Round(time.Second)
		return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("pong — uptime %s", up))
	}
}
