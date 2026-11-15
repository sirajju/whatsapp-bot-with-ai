package commands

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// ---------- helpers ----------

func requireGroupOwner(env *router.Env) (string, bool) {
	if !env.IsGroup {
		return "group command only", false
	}
	if !env.IsOwner {
		return "owner only", false
	}
	return "", true
}

func quotedParticipant(evt *events.Message) (types.JID, bool) {
	ext := evt.Message.GetExtendedTextMessage()
	if ext == nil {
		return types.JID{}, false
	}
	p := ext.GetContextInfo().GetParticipant()
	if p == "" {
		return types.JID{}, false
	}
	j, err := types.ParseJID(p)
	if err != nil {
		return types.JID{}, false
	}
	return j, true
}

// targetJID picks the first argument (phone digits like 919xxxxxxxxx) or falls back to quoted participant.
func targetJID(env *router.Env) (types.JID, bool) {
	if len(env.Args) > 0 {
		phone := strings.TrimPrefix(strings.TrimSpace(env.Args[0]), "+")
		phone = strings.TrimSuffix(phone, "@s.whatsapp.net")
		if phone != "" {
			j, err := types.ParseJID(phone + "@s.whatsapp.net")
			if err == nil {
				return j, true
			}
		}
	}
	return quotedParticipant(env.Msg)
}

// ---------- commands ----------

func Kick() router.Handler {
	return participantAction("removed", whatsmeow.ParticipantChangeRemove)
}
func Add() router.Handler {
	return participantAction("added", whatsmeow.ParticipantChangeAdd)
}
func Promote() router.Handler {
	return participantAction("promoted", whatsmeow.ParticipantChangePromote)
}
func Demote() router.Handler {
	return participantAction("demoted", whatsmeow.ParticipantChangeDemote)
}

func participantAction(label string, action whatsmeow.ParticipantChange) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if msg, ok := requireGroupOwner(env); !ok {
			return sendText(ctx, env.Cli, env.Chat, msg)
		}
		target, ok := targetJID(env)
		if !ok {
			return sendText(ctx, env.Cli, env.Chat,
				fmt.Sprintf("reply to a user or pass a phone with !%s", action))
		}
		if _, err := env.Cli.UpdateGroupParticipants(ctx, env.Chat,
			[]types.JID{target}, action); err != nil {
			return sendText(ctx, env.Cli, env.Chat, label+" failed: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("%s %s", label, target.User))
	}
}

// Mute flips the group to admins-only messaging.
func Mute() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if msg, ok := requireGroupOwner(env); !ok {
			return sendText(ctx, env.Cli, env.Chat, msg)
		}
		if err := env.Cli.SetGroupAnnounce(ctx, env.Chat, true); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "mute failed: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "group muted (admins only)")
	}
}

func Unmute() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if msg, ok := requireGroupOwner(env); !ok {
			return sendText(ctx, env.Cli, env.Chat, msg)
		}
		if err := env.Cli.SetGroupAnnounce(ctx, env.Chat, false); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "unmute failed: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "group unmuted (everyone can send)")
	}
}

// Warn adds a warn for a user; auto-kicks at 3.
func Warn() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if msg, ok := requireGroupOwner(env); !ok {
			return sendText(ctx, env.Cli, env.Chat, msg)
		}
		target, ok := targetJID(env)
		if !ok {
			return sendText(ctx, env.Cli, env.Chat, "reply to a user with !warn [reason]")
		}
		reason := strings.Join(env.Args[minInt(len(env.Args), 1):], " ")
		if reason == "" {
			reason = "no reason"
		}
		n, err := env.Store.AddWarn(ctx, env.Chat.String(), target.String(), reason)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "warn: "+err.Error())
		}
		if n >= 3 {
			_, _ = env.Cli.UpdateGroupParticipants(ctx, env.Chat,
				[]types.JID{target}, whatsmeow.ParticipantChangeRemove)
			_ = env.Store.ClearWarns(ctx, env.Chat.String(), target.String())
			return sendText(ctx, env.Cli, env.Chat,
				fmt.Sprintf("%s hit 3 warns — removed", target.User))
		}
		return sendText(ctx, env.Cli, env.Chat,
			fmt.Sprintf("warned %s (%d/3)", target.User, n))
	}
}

func Warns() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsGroup {
			return sendText(ctx, env.Cli, env.Chat, "group command only")
		}
		target, ok := targetJID(env)
		if !ok {
			target = env.Sender
		}
		n, err := env.Store.CountWarns(ctx, env.Chat.String(), target.String())
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "warns: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat,
			fmt.Sprintf("%s: %d/3 warns", target.User, n))
	}
}

func Unwarn() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if msg, ok := requireGroupOwner(env); !ok {
			return sendText(ctx, env.Cli, env.Chat, msg)
		}
		target, ok := targetJID(env)
		if !ok {
			return sendText(ctx, env.Cli, env.Chat, "reply to a user with !unwarn")
		}
		if err := env.Store.ClearWarns(ctx, env.Chat.String(), target.String()); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "unwarn: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "cleared warns for "+target.User)
	}
}

// Antilink toggles automatic link removal + warn in this group.
func Antilink() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if msg, ok := requireGroupOwner(env); !ok {
			return sendText(ctx, env.Cli, env.Chat, msg)
		}
		if len(env.Args) == 0 {
			g, _ := env.Store.Group(ctx, env.Chat.String())
			state := "off"
			if g.Antilink {
				state = "on"
			}
			return sendText(ctx, env.Cli, env.Chat,
				fmt.Sprintf("antilink is %s — toggle with !antilink on/off", state))
		}
		on := strings.EqualFold(env.Args[0], "on")
		if err := env.Store.SetAntilink(ctx, env.Chat.String(), on); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "antilink: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("antilink %s", env.Args[0]))
	}
}

func SetWelcome() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if msg, ok := requireGroupOwner(env); !ok {
			return sendText(ctx, env.Cli, env.Chat, msg)
		}
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat,
				"usage: !welcome <message> (placeholders: {user}, {group})")
		}
		text := strings.Join(env.Args, " ")
		if err := env.Store.SetWelcome(ctx, env.Chat.String(), text); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "welcome: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "welcome message saved")
	}
}

func SetGoodbye() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if msg, ok := requireGroupOwner(env); !ok {
			return sendText(ctx, env.Cli, env.Chat, msg)
		}
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat,
				"usage: !goodbye <message> (placeholders: {user}, {group})")
		}
		text := strings.Join(env.Args, " ")
		if err := env.Store.SetGoodbye(ctx, env.Chat.String(), text); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "goodbye: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "goodbye message saved")
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
