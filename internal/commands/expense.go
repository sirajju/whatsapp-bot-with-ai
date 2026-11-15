package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Expense routes the `!expense` command to the skills/expense Python tool,
// with one special-case: `sync` reaches into whatsmeow to update the group's
// WhatsApp description with the current totals.
func Expense() router.Handler {
	skillHandler := Skill()
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) > 0 && strings.ToLower(env.Args[0]) == "sync" {
			return expenseSyncGroupDesc(ctx, env)
		}
		// Everything else goes through the generic skill runner,
		// which now sends raw output so *bold* etc. render.
		shim := *env
		shim.Args = append([]string{"expense"}, env.Args...)
		return skillHandler(ctx, &shim)
	}
}

// expenseSyncGroupDesc runs `expense total` and writes the result into the
// WhatsApp group description. Owner-only, group-only.
func expenseSyncGroupDesc(ctx context.Context, env *router.Env) error {
	if !env.IsGroup {
		return sendText(ctx, env.Cli, env.Chat, "sync only works in group chats")
	}
	if !env.IsOwner {
		return sendText(ctx, env.Cli, env.Chat, "owner only")
	}
	total, err := runExpenseSkill(ctx, env, "total")
	if err != nil {
		return sendText(ctx, env.Cli, env.Chat, "sync: "+err.Error())
	}
	desc := total
	// WhatsApp group descriptions accept several kB in practice, but keep it
	// readable — ~1500 chars is generous.
	if len(desc) > 1500 {
		desc = desc[:1500] + "…"
	}
	if err := env.Cli.SetGroupDescription(ctx, env.Chat, desc); err != nil {
		return sendText(ctx, env.Cli, env.Chat, "could not update description: "+err.Error())
	}
	return sendText(ctx, env.Cli, env.Chat, "✅ group description updated")
}

// runExpenseSkill shells out to skills/expense and returns the trimmed stdout.
func runExpenseSkill(ctx context.Context, env *router.Env, subcmd ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "./skills/expense", subcmd...)
	cmd.Env = append(os.Environ(),
		"WABOT_SENDER="+env.Sender.String(),
		"WABOT_CHAT="+env.Chat.String(),
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		s := strings.TrimSpace(out.String())
		if s == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", s)
	}
	return strings.TrimSpace(out.String()), nil
}
