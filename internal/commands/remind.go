package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Remind creates a reminder.
//   /remind 5m check the oven
//   /remind 1h30m call mom
//   /remind 2d review PR
func Remind() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) < 2 {
			return sendText(ctx, env.Cli, env.Chat,
				"usage: "+env.Cfg.Prefix+"remind <duration> <text>\n  e.g. "+env.Cfg.Prefix+"remind 1h30m call mom\n  units: s m h d (combine freely)")
		}
		dur, err := parseHumanDuration(env.Args[0])
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "bad duration: "+err.Error())
		}
		text := strings.Join(env.Args[1:], " ")
		remindAt := time.Now().Add(dur).Unix()
		id, err := env.Store.AddReminder(ctx, env.Chat.String(), env.Sender.String(), remindAt, text)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "remind: "+err.Error())
		}
		when := time.Unix(remindAt, 0).Format("2006-01-02 15:04 MST")
		return sendText(ctx, env.Cli, env.Chat,
			fmt.Sprintf("⏰ reminder #%d set for %s\n%s", id, when, text))
	}
}

// Reminders lists this user's pending reminders.
func Reminders() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		rs, err := env.Store.ListReminders(ctx, env.Sender.String())
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "reminders: "+err.Error())
		}
		if len(rs) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "no pending reminders")
		}
		var b strings.Builder
		b.WriteString("⏰ *Pending reminders*\n")
		for _, r := range rs {
			when := time.Unix(r.RemindAt, 0).Format("2006-01-02 15:04")
			fmt.Fprintf(&b, "\n#%d · %s\n   %s", r.ID, when, r.Text)
		}
		b.WriteString("\n\ncancel: " + env.Cfg.Prefix + "unremind <id>")
		return sendText(ctx, env.Cli, env.Chat, b.String())
	}
}

// Unremind cancels a reminder by id (only if the caller owns it).
func Unremind() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"unremind <id>")
		}
		id, err := strconv.ParseInt(env.Args[0], 10, 64)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "bad id")
		}
		ok, err := env.Store.CancelReminder(ctx, id, env.Sender.String())
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "unremind: "+err.Error())
		}
		if !ok {
			return sendText(ctx, env.Cli, env.Chat, "no pending reminder #"+env.Args[0]+" belongs to you")
		}
		return sendText(ctx, env.Cli, env.Chat, "✅ reminder #"+env.Args[0]+" cancelled")
	}
}

// parseHumanDuration accepts Go's time.ParseDuration shapes PLUS the 'd' (days) suffix.
// Examples: "5m", "1h30m", "2d", "90s", "1d12h".
func parseHumanDuration(s string) (time.Duration, error) {
	// Replace trailing/interior 'd' with hours: find runs of digits followed by 'd' and
	// convert to hours directly.
	var out strings.Builder
	i := 0
	for i < len(s) {
		// consume digits
		j := i
		for j < len(s) && (s[j] >= '0' && s[j] <= '9') {
			j++
		}
		if j > i && j < len(s) && s[j] == 'd' {
			n, err := strconv.Atoi(s[i:j])
			if err != nil {
				return 0, err
			}
			fmt.Fprintf(&out, "%dh", n*24)
			i = j + 1
			continue
		}
		if j > i {
			out.WriteString(s[i:j])
			i = j
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return time.ParseDuration(out.String())
}
