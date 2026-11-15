package commands

import (
	"context"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Expense is a thin shortcut for `!skill expense`. Prepending "expense" to args
// means `!expense add samad 100` is identical to `!skill expense add samad 100`
// — one source of truth (the skill script) with a friendlier entry point.
func Expense() router.Handler {
	skillHandler := Skill()
	return func(ctx context.Context, env *router.Env) error {
		shim := *env
		shim.Args = append([]string{"expense"}, env.Args...)
		return skillHandler(ctx, &shim)
	}
}
