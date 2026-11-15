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
			return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"broadcast <message>")
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
			time.Sleep(500 * time.Millisecond)
		}
		return sendText(ctx, env.Cli, env.Chat,
			fmt.Sprintf("broadcast sent to %d groups (failed %d)", sent, failed))
	}
}

// Send lets the owner message any phone from the bot number. Owner only.
//
//	!send <phone> <text>   phone digits with country code, no + or spaces
func Send() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		if len(env.Args) < 2 {
			return sendText(ctx, env.Cli, env.Chat,
				"usage: "+env.Cfg.Prefix+"send <phone> <text>\n"+
					"phone: digits only, country code included (e.g. 917593980565)")
		}
		phone := onlyDigits(env.Args[0])
		if phone == "" {
			return sendText(ctx, env.Cli, env.Chat, "bad phone: need digits (country code included, no +)")
		}
		target := types.NewJID(phone, types.DefaultUserServer)
		text := strings.Join(env.Args[1:], " ")
		if err := sendText(ctx, env.Cli, target, text); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "send failed: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "✅ sent to +"+target.User)
	}
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Block adds a user to the bot's blocklist. They get no further command dispatch.
func Block() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		target, ok := targetJID(env)
		if !ok {
			return sendText(ctx, env.Cli, env.Chat, "reply to a user or pass a phone with "+env.Cfg.Prefix+"block")
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
			return sendText(ctx, env.Cli, env.Chat, "reply to a user or pass a phone with "+env.Cfg.Prefix+"unblock")
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
	if osExit != nil {
		osExit(code)
	}
}

// osExit is wired by main.go to os.Exit.
var osExit func(int)

// Unused helper to silence imports if trimmed.
var _ = types.JID{}
