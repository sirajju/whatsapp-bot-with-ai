package aitools

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
)

func addReminderTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "add_reminder",
			Description: "Schedule a reminder that the bot will deliver to the user later. " +
				"The duration is a Go-style string (e.g. \"30m\", \"2h15m\", \"3d\"). " +
				"Units: s, m, h, d. Returns the reminder's id and local wall-clock time.",
			Parameters: objectSchema(map[string]any{
				"duration": stringProp("How long from now, e.g. '30m' or '1h30m' or '2d'."),
				"text":     stringProp("What to remind the user about."),
			}, "duration", "text"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			durStr, err := requireString(args, "duration")
			if err != nil {
				return "", err
			}
			text, err := requireString(args, "text")
			if err != nil {
				return "", err
			}
			dur, err := parseHumanDuration(durStr)
			if err != nil {
				return "", fmt.Errorf("bad duration %q: %w", durStr, err)
			}
			at := time.Now().Add(dur).Unix()
			id, err := env.Store.AddReminder(env.Ctx, env.ChatJID, env.SenderJID, at, text)
			if err != nil {
				return "", err
			}
			when := time.Unix(at, 0).Format("2006-01-02 15:04 MST")
			return fmt.Sprintf("reminder #%d set for %s: %s", id, when, text), nil
		},
	}
}

func listRemindersTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "list_reminders",
			Description: "List the user's pending reminders.",
			Parameters:  objectSchema(map[string]any{}),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			rs, err := env.Store.ListReminders(env.Ctx, env.SenderJID)
			if err != nil {
				return "", err
			}
			if len(rs) == 0 {
				return "No pending reminders.", nil
			}
			var b strings.Builder
			b.WriteString("Pending reminders:\n")
			for _, r := range rs {
				when := time.Unix(r.RemindAt, 0).Format("2006-01-02 15:04")
				fmt.Fprintf(&b, "#%d at %s: %s\n", r.ID, when, r.Text)
			}
			return strings.TrimSpace(b.String()), nil
		},
	}
}

// parseHumanDuration is a copy of the one in commands/remind.go so the tools package
// stays import-cycle-free.
func parseHumanDuration(s string) (time.Duration, error) {
	var out strings.Builder
	i := 0
	for i < len(s) {
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
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
