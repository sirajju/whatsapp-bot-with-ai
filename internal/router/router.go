// Package router dispatches inbound text events to registered commands.
package router

import (
	"context"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/config"
)

// Handler is a command implementation. Returning an error logs it; nothing is sent back.
type Handler func(ctx context.Context, env *Env) error

// Env is the per-invocation context passed to a Handler.
type Env struct {
	Cli     *whatsmeow.Client
	Cfg     *config.Config
	Log     zerolog.Logger
	Msg     *events.Message
	Chat    types.JID
	Sender  types.JID
	IsGroup bool
	IsOwner bool
	Cmd     string
	Args    []string
	RawText string
}

type Router struct {
	cli      *whatsmeow.Client
	cfg      *config.Config
	log      zerolog.Logger
	handlers map[string]Handler
	help     map[string]string
}

func New(cli *whatsmeow.Client, cfg *config.Config, log zerolog.Logger) *Router {
	return &Router{
		cli:      cli,
		cfg:      cfg,
		log:      log,
		handlers: make(map[string]Handler),
		help:     make(map[string]string),
	}
}

// Register wires a command name (lowercase, no prefix) to its handler.
func (r *Router) Register(name, help string, h Handler) {
	r.handlers[strings.ToLower(name)] = h
	r.help[strings.ToLower(name)] = help
}

func (r *Router) Help() map[string]string { return r.help }

// Dispatch parses a message event and calls the matching handler, if any.
func (r *Router) Dispatch(ctx context.Context, evt *events.Message) {
	text := extractText(evt)
	if text == "" {
		return
	}

	isGroup := evt.Info.IsGroup
	if isGroup && !r.cfg.RespondInGroups {
		return
	}
	// A group_trigger=mention_only flag could gate on @mention; left as a TODO for now.

	if !strings.HasPrefix(text, r.cfg.Prefix) {
		return
	}
	fields := strings.Fields(strings.TrimPrefix(text, r.cfg.Prefix))
	if len(fields) == 0 {
		return
	}
	cmd := strings.ToLower(fields[0])
	h, ok := r.handlers[cmd]
	if !ok {
		return
	}

	sender := evt.Info.Sender
	isOwner := false
	for _, o := range r.cfg.Owners {
		if sender.User == o {
			isOwner = true
			break
		}
	}

	env := &Env{
		Cli:     r.cli,
		Cfg:     r.cfg,
		Log:     r.log.With().Str("cmd", cmd).Str("chat", evt.Info.Chat.String()).Logger(),
		Msg:     evt,
		Chat:    evt.Info.Chat,
		Sender:  sender,
		IsGroup: isGroup,
		IsOwner: isOwner,
		Cmd:     cmd,
		Args:    fields[1:],
		RawText: text,
	}

	if err := h(ctx, env); err != nil {
		env.Log.Error().Err(err).Msg("command failed")
	}
}

func extractText(evt *events.Message) string {
	m := evt.Message
	if m == nil {
		return ""
	}
	if m.GetConversation() != "" {
		return m.GetConversation()
	}
	if ext := m.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	if img := m.GetImageMessage(); img != nil {
		return img.GetCaption()
	}
	if vid := m.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	}
	return ""
}
