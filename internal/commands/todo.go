package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Todo is the entry point for the /todo family. Subcommands:
//   /todo               list pending
//   /todo all           list pending + done
//   /todo add <text>    add
//   /todo done <n>      mark the nth pending as done
//   /todo del <n>       delete the nth (counts both pending and done)
func Todo() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return listTodos(ctx, env, false)
		}
		sub := strings.ToLower(env.Args[0])
		switch sub {
		case "all":
			return listTodos(ctx, env, true)
		case "add":
			if len(env.Args) < 2 {
				return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"todo add <text>")
			}
			text := strings.Join(env.Args[1:], " ")
			if _, err := env.Store.AddTodo(ctx, env.Sender.String(), text); err != nil {
				return sendText(ctx, env.Cli, env.Chat, "todo: "+err.Error())
			}
			return sendText(ctx, env.Cli, env.Chat, "📝 added: "+text)
		case "done":
			if len(env.Args) < 2 {
				return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"todo done <n>")
			}
			n, err := strconv.Atoi(env.Args[1])
			if err != nil {
				return sendText(ctx, env.Cli, env.Chat, "bad number")
			}
			done, err := env.Store.MarkTodoDone(ctx, env.Sender.String(), n)
			if err != nil {
				return sendText(ctx, env.Cli, env.Chat, "todo: "+err.Error())
			}
			return sendText(ctx, env.Cli, env.Chat, "✅ done: "+done)
		case "del":
			if len(env.Args) < 2 {
				return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"todo del <n>")
			}
			n, err := strconv.Atoi(env.Args[1])
			if err != nil {
				return sendText(ctx, env.Cli, env.Chat, "bad number")
			}
			deleted, err := env.Store.DelTodo(ctx, env.Sender.String(), n)
			if err != nil {
				return sendText(ctx, env.Cli, env.Chat, "todo: "+err.Error())
			}
			return sendText(ctx, env.Cli, env.Chat, "🗑 deleted: "+deleted)
		default:
			// Any other arg is treated as add
			text := strings.Join(env.Args, " ")
			if _, err := env.Store.AddTodo(ctx, env.Sender.String(), text); err != nil {
				return sendText(ctx, env.Cli, env.Chat, "todo: "+err.Error())
			}
			return sendText(ctx, env.Cli, env.Chat, "📝 added: "+text)
		}
	}
}

func listTodos(ctx context.Context, env *router.Env, includeDone bool) error {
	todos, err := env.Store.ListTodos(ctx, env.Sender.String(), includeDone)
	if err != nil {
		return sendText(ctx, env.Cli, env.Chat, "todo: "+err.Error())
	}
	if len(todos) == 0 {
		return sendText(ctx, env.Cli, env.Chat,
			"no todos. Add one: "+env.Cfg.Prefix+"todo add <text>")
	}
	var b strings.Builder
	b.WriteString("📝 *Your todos*\n")
	for i, t := range todos {
		mark := "☐"
		if t.Done {
			mark = "✅"
		}
		fmt.Fprintf(&b, "\n%d. %s %s", i+1, mark, t.Text)
	}
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "mark done: %stodo done <n>\ndelete: %stodo del <n>", env.Cfg.Prefix, env.Cfg.Prefix)
	return sendText(ctx, env.Cli, env.Chat, b.String())
}
