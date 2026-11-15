package commands

import (
	"context"
	"regexp"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/store"
)

// urlRe is a loose URL matcher — good enough for antilink.
var urlRe = regexp.MustCompile(`(?i)\bhttps?://\S+|\bwww\.\S+|\b\S+\.(com|net|org|io|xyz|gg|me|co|link|click)(/\S*)?`)

// BlocklistHook swallows any message whose sender is on the bot's blocklist.
func BlocklistHook() router.Hook {
	return func(ctx context.Context, env *router.Env) bool {
		if env.Store.IsBlocked(ctx, env.Sender.String()) {
			env.Log.Debug().Str("from", env.Sender.String()).Msg("blocked user — dropping")
			return true
		}
		return false
	}
}

// AntilinkHook deletes link-bearing messages in groups where antilink is on,
// and warns the sender (auto-kick at 3 warns via the warns table).
func AntilinkHook() router.Hook {
	return func(ctx context.Context, env *router.Env) bool {
		if !env.IsGroup {
			return false
		}
		if env.IsOwner {
			return false
		}
		g, err := env.Store.Group(ctx, env.Chat.String())
		if err != nil || !g.Antilink {
			return false
		}
		if !urlRe.MatchString(env.RawText) {
			return false
		}
		// Delete the message
		revoked := env.Cli.BuildRevoke(env.Chat, env.Sender, env.Msg.Info.ID)
		_, _ = env.Cli.SendMessage(ctx, env.Chat, revoked)

		n, _ := env.Store.AddWarn(ctx, env.Chat.String(), env.Sender.String(), "antilink")
		_ = sendText(ctx, env.Cli, env.Chat,
			"link removed — "+env.Sender.User+" warned ("+itoa(n)+"/3)")
		if n >= 3 {
			_, _ = env.Cli.UpdateGroupParticipants(ctx, env.Chat,
				[]types.JID{env.Sender}, whatsmeow.ParticipantChangeRemove)
			_ = env.Store.ClearWarns(ctx, env.Chat.String(), env.Sender.String())
		}
		return true
	}
}

// GroupParticipantHandler is the whatsmeow event handler for joins and leaves.
// Wire it in main.go after the client is built.
func GroupParticipantHandler(cli *whatsmeow.Client, st *store.Store) func(evt *events.GroupInfo) {
	return func(evt *events.GroupInfo) {
		ctx := context.Background()
		g, err := st.Group(ctx, evt.JID.String())
		if err != nil {
			return
		}
		groupName := evt.Name
		gname := ""
		if groupName != nil {
			gname = groupName.Name
		}

		for _, j := range evt.Join {
			if g.Welcome == "" {
				continue
			}
			msg := applyPlaceholders(g.Welcome, j.User, gname)
			_ = sendText(ctx, cli, evt.JID, msg)
		}
		for _, j := range evt.Leave {
			if g.Goodbye == "" {
				continue
			}
			msg := applyPlaceholders(g.Goodbye, j.User, gname)
			_ = sendText(ctx, cli, evt.JID, msg)
		}
	}
}

func applyPlaceholders(tpl, user, group string) string {
	r := strings.NewReplacer("{user}", "@"+user, "{group}", group)
	return r.Replace(tpl)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
