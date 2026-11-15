package commands

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Kick removes the quoted user from the group. Owner-only.
func Kick() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsGroup {
			return sendText(ctx, env.Cli, env.Chat, "group command only")
		}
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		target, ok := quotedParticipant(env.Msg)
		if !ok {
			return sendText(ctx, env.Cli, env.Chat, "reply to a user with !kick")
		}
		_, err := env.Cli.UpdateGroupParticipants(ctx, env.Chat,
			[]types.JID{target}, whatsmeow.ParticipantChangeRemove)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "kick failed: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("removed %s", target.User))
	}
}

func quotedParticipant(evt *events.Message) (types.JID, bool) {
	ext := evt.Message.GetExtendedTextMessage()
	if ext == nil {
		return types.JID{}, false
	}
	ctxInfo := ext.GetContextInfo()
	if ctxInfo == nil {
		return types.JID{}, false
	}
	p := ctxInfo.GetParticipant()
	if p == "" {
		return types.JID{}, false
	}
	j, err := types.ParseJID(p)
	if err != nil {
		return types.JID{}, false
	}
	return j, true
}
