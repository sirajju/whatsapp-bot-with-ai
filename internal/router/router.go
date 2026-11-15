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
	"github.com/sirajju/whatsapp-bot-with-ai/internal/store"
)

// Handler is a command implementation.
type Handler func(ctx context.Context, env *Env) error

// Env is the per-invocation context passed to a Handler.
type Env struct {
	Cli     *whatsmeow.Client
	Cfg     *config.Config
	Store   *store.Store
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

// Hook runs before dispatch for every message. Return true to swallow the message
// (no command dispatch, no further hooks). Used for antilink, blocklist, etc.
type Hook func(ctx context.Context, env *Env) bool

type Router struct {
	cli      *whatsmeow.Client
	cfg      *config.Config
	st       *store.Store
	log      zerolog.Logger
	handlers map[string]Handler
	aliases  map[string]string
	help     map[string]string
	hooks    []Hook
	fallback Handler // runs when a non-command, non-swallowed message arrives (e.g. AI session mode)
}

func New(cli *whatsmeow.Client, cfg *config.Config, st *store.Store, log zerolog.Logger) *Router {
	return &Router{
		cli:      cli,
		cfg:      cfg,
		st:       st,
		log:      log,
		handlers: make(map[string]Handler),
		aliases:  make(map[string]string),
		help:     make(map[string]string),
	}
}

func (r *Router) Register(name, help string, h Handler, aliases ...string) {
	name = strings.ToLower(name)
	r.handlers[name] = h
	r.help[name] = help
	for _, a := range aliases {
		r.aliases[strings.ToLower(a)] = name
	}
}

func (r *Router) AddHook(h Hook) { r.hooks = append(r.hooks, h) }

// SetFallback registers a handler to run on any message that is not a command and
// was not swallowed by a hook. Used by AI session mode so bare messages go to the LLM.
func (r *Router) SetFallback(h Handler) { r.fallback = h }

func (r *Router) Help() map[string]string { return r.help }

// Dispatch parses a message event and calls the matching handler, if any.
func (r *Router) Dispatch(ctx context.Context, evt *events.Message) {
	text := ExtractText(evt)
	if text == "" {
		return
	}
	env := r.buildEnv(evt, text)

	// Run hooks (antilink, blocklist) — they can swallow the message.
	for _, h := range r.hooks {
		if h(ctx, env) {
			return
		}
	}

	if !strings.HasPrefix(text, r.cfg.Prefix) {
		if r.fallback != nil {
			if err := r.fallback(ctx, env); err != nil {
				env.Log.Debug().Err(err).Msg("fallback")
			}
		}
		return
	}
	fields := strings.Fields(strings.TrimPrefix(text, r.cfg.Prefix))
	if len(fields) == 0 {
		return
	}
	cmd := strings.ToLower(fields[0])
	if alias, ok := r.aliases[cmd]; ok {
		cmd = alias
	}
	h, ok := r.handlers[cmd]
	if !ok {
		return
	}
	env.Cmd = cmd
	env.Args = fields[1:]
	env.Log = env.Log.With().Str("cmd", cmd).Logger()

	if err := h(ctx, env); err != nil {
		env.Log.Error().Err(err).Msg("command failed")
	}
}

func (r *Router) buildEnv(evt *events.Message, text string) *Env {
	isGroup := evt.Info.IsGroup
	sender := evt.Info.Sender

	// Candidate user ids to compare against OWNERS: the main sender's User plus
	// SenderAlt.User (WhatsApp's LID → phone-number alternative identifier).
	candidates := []string{sender.User}
	if alt := evt.Info.SenderAlt; !alt.IsEmpty() && alt.User != "" {
		candidates = append(candidates, alt.User)
	}
	isOwner := false
checkOwners:
	for _, u := range candidates {
		for _, o := range r.cfg.Owners {
			if u == o {
				isOwner = true
				break checkOwners
			}
		}
	}
	return &Env{
		Cli:     r.cli,
		Cfg:     r.cfg,
		Store:   r.st,
		Log:     r.log.With().Str("chat", evt.Info.Chat.String()).Logger(),
		Msg:     evt,
		Chat:    evt.Info.Chat,
		Sender:  sender,
		IsGroup: isGroup,
		IsOwner: isOwner,
		RawText: text,
	}
}

// ParseJIDSafe is a small helper exposed so other packages (e.g. reminder loop) can parse a
// stored JID string without re-importing whatsmeow/types everywhere.
func ParseJIDSafe(s string) (types.JID, error) { return types.ParseJID(s) }

// ExtractText returns the human-readable text of a message, from any of the common wrappers.
func ExtractText(evt *events.Message) string {
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
