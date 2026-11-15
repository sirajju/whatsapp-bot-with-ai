// wabot — a lightweight native WhatsApp bot built on whatsmeow.
package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/auth"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/commands"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/config"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panicLog("config", err)
	}

	lvl, _ := zerolog.ParseLevel(cfg.LogLevel)
	if lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}
	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		Level(lvl).With().Timestamp().Logger()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cli, err := auth.NewClient(ctx, cfg.SessionDir, log)
	if err != nil {
		log.Fatal().Err(err).Msg("new client")
	}

	// Build router and register commands.
	r := router.New(cli, cfg, log)
	provider := ai.Pick(ai.Config{
		OllamaURL: cfg.OllamaURL, OllamaModel: cfg.OllamaModel,
		GroqKey: cfg.GroqKey, GroqModel: cfg.GroqModel,
		AnthropicKey: cfg.AnthropicKey, AnthropicMdl: cfg.AnthropicMdl,
		OpenAIKey: cfg.OpenAIKey, OpenAIModel: cfg.OpenAIModel,
		GeminiKey: cfg.GeminiKey, GeminiModel: cfg.GeminiModel,
	})
	if provider != nil {
		log.Info().Str("provider", provider.Name()).Msg("AI provider ready")
	} else {
		log.Info().Msg("no AI provider configured (!ai will report so)")
	}

	start := time.Now()
	r.Register("ping", "liveness check", commands.Ping(start))
	r.Register("ai", "ask the configured LLM", commands.AI(provider))
	r.Register("sticker", "reply to image/video to turn into sticker", commands.Sticker())
	r.Register("kick", "owner-only: remove quoted user from group", commands.Kick())
	// menu is registered last so it sees every other command.
	r.Register("menu", "show this list", commands.Menu(r.Help()))

	// Wire event handler.
	cli.AddEventHandler(func(evt any) {
		switch e := evt.(type) {
		case *events.Message:
			r.Dispatch(ctx, e)
		case *events.Connected:
			log.Info().Msg("connected")
		case *events.Disconnected:
			log.Warn().Msg("disconnected (whatsmeow will auto-reconnect)")
		case *events.LoggedOut:
			log.Error().Str("reason", e.Reason.String()).Msg("logged out — delete session/ and relink")
		}
	})

	// Login.
	switch strings.ToLower(cfg.LoginMode) {
	case "qr":
		if err := auth.LoginQR(ctx, cli, log); err != nil {
			log.Fatal().Err(err).Msg("qr login")
		}
	default:
		if err := auth.LoginPairing(ctx, cli, cfg.PairPhone, log); err != nil {
			log.Fatal().Err(err).Msg("pairing")
		}
	}

	// Wait for Ctrl-C / SIGTERM.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info().Msg("shutting down")
	cli.Disconnect()
}

func panicLog(where string, err error) {
	// Use stderr directly before zerolog is up.
	os.Stderr.WriteString(where + ": " + err.Error() + "\n")
	os.Exit(1)
}
