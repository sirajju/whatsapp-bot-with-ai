package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Broadcast sends a message to every joined group. Owner only.
func Broadcast() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: !broadcast <message>")
		}
		msg := strings.Join(env.Args, " ")

		groups, err := env.Cli.GetJoinedGroups(ctx)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "broadcast: "+err.Error())
		}
		sent, failed := 0, 0
		for _, g := range groups {
			if err := sendText(ctx, env.Cli, g.JID, "[broadcast] "+msg); err != nil {
				failed++
			} else {
				sent++
			}
			time.Sleep(500 * time.Millisecond) // avoid being flagged
		}
		return sendText(ctx, env.Cli, env.Chat,
			fmt.Sprintf("broadcast sent to %d groups (failed %d)", sent, failed))
	}
}

// Block adds a user to the bot's blocklist. They get no further command dispatch.
func Block() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		target, ok := targetJID(env)
		if !ok {
			return sendText(ctx, env.Cli, env.Chat, "reply to a user or pass a phone with !block")
		}
		reason := "blocked by owner"
		if len(env.Args) > 1 {
			reason = strings.Join(env.Args[1:], " ")
		}
		if err := env.Store.Block(ctx, target.String(), reason); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "block: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "blocked "+target.User)
	}
}

func Unblock() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		target, ok := targetJID(env)
		if !ok {
			return sendText(ctx, env.Cli, env.Chat, "reply to a user or pass a phone with !unblock")
		}
		if err := env.Store.Unblock(ctx, target.String()); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "unblock: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "unblocked "+target.User)
	}
}

// Restart exits cleanly; systemd (or your supervisor) will bring the process back.
func Restart() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		_ = sendText(ctx, env.Cli, env.Chat, "restarting…")
		// Delay slightly so the message actually flushes before we exit.
		go func() {
			time.Sleep(500 * time.Millisecond)
			panicExit()
		}()
		return nil
	}
}

// panicExit is separated so main can swap it in tests if desired.
var panicExit = func() { exitWithCode(0) }

func exitWithCode(code int) {
	// os.Exit without import cycles: call through a function ref main sets at startup.
	if osExit != nil {
		osExit(code)
	}
}

// osExit is wired by main.go to os.Exit.
var osExit func(int)

// Unused helper to silence imports if trimmed.
var _ = types.JID{}
