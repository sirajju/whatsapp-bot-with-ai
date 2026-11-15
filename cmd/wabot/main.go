// wabot — a lightweight native WhatsApp bot built on whatsmeow.
package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
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
	"github.com/sirajju/whatsapp-bot-with-ai/internal/store"
)

func main() {
	commands.SetOSExit(os.Exit)

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

	// State store (sqlite) lives alongside the session DB.
	st, err := store.Open(filepath.Join(cfg.SessionDir, "state"))
	if err != nil {
		log.Fatal().Err(err).Msg("open store")
	}
	defer st.Close()

	cli, err := auth.NewClient(ctx, cfg.SessionDir, log)
	if err != nil {
		log.Fatal().Err(err).Msg("new client")
	}

	// Build router + register commands.
	r := router.New(cli, cfg, st, log)
	provider := ai.Pick(ai.Config{
		Preferred:       cfg.AIPreferred,
		OllamaURL:       cfg.OllamaURL, OllamaModel: cfg.OllamaModel,
		GroqKey:         cfg.GroqKey, GroqModel: cfg.GroqModel,
		AnthropicKey:    cfg.AnthropicKey, AnthropicMdl: cfg.AnthropicMdl,
		OpenAIKey:       cfg.OpenAIKey, OpenAIModel: cfg.OpenAIModel,
		GeminiKey:       cfg.GeminiKey, GeminiModel: cfg.GeminiModel,
		NvidiaKey:       cfg.NvidiaKey, NvidiaModel: cfg.NvidiaModel,
		OpenRouterKey:   cfg.OpenRouterKey, OpenRouterModel: cfg.OpenRouterModel,
		OpencodeArgv:    cfg.OpencodeArgv,
	})
	if provider != nil {
		log.Info().Str("provider", provider.Name()).Msg("AI provider ready")
	} else {
		log.Info().Msg("no AI provider configured")
	}

	// Passive hooks run before command dispatch, in order.
	r.AddHook(commands.BlocklistHook())
	r.AddHook(commands.AntilinkHook())
	// Fallback: in AI session mode, bare messages go to the LLM.
	r.SetFallback(commands.AIFallback(provider))

	start := time.Now()
	// Core
	r.Register("ping", "liveness check", commands.Ping(start))
	// AI
	r.Register("ai", "ask the LLM (no args = enter chat mode)", commands.AI(provider), "chat", "gpt")
	r.Register("endai", "leave AI chat mode", commands.EndAI())
	r.Register("clearai", "wipe AI chat memory for this chat", commands.ClearAI())
	r.Register("persona", "switch AI persona for this chat", commands.SetPersona())
	r.Register("run", "owner: run a script from ./scripts/", commands.Run())
	// Stickers
	r.Register("sticker", "reply to image/video → sticker", commands.Sticker(), "s")
	r.Register("toimg", "reply to a sticker → jpeg image", commands.Toimg())
	r.Register("stext", "render text as a sticker", commands.Stext())
	// Group moderation
	r.Register("kick", "owner: remove quoted/named user", commands.Kick())
	r.Register("add", "owner: add a user by phone", commands.Add())
	r.Register("promote", "owner: promote to admin", commands.Promote())
	r.Register("demote", "owner: demote admin", commands.Demote())
	r.Register("mute", "owner: lock group to admins-only", commands.Mute())
	r.Register("unmute", "owner: unlock group", commands.Unmute())
	r.Register("warn", "owner: warn user (auto-kick at 3)", commands.Warn())
	r.Register("warns", "show warn count", commands.Warns())
	r.Register("unwarn", "owner: clear warns", commands.Unwarn())
	r.Register("antilink", "owner: toggle link removal (on/off)", commands.Antilink())
	r.Register("welcome", "owner: set welcome message", commands.SetWelcome())
	r.Register("goodbye", "owner: set goodbye message", commands.SetGoodbye())
	// Downloader + fun
	r.Register("dl", "download video from URL (yt-dlp)", commands.Download(), "download")
	r.Register("tr", "translate text", commands.Translate(), "translate")
	r.Register("weather", "weather by city", commands.Weather())
	r.Register("8ball", "magic 8 ball", commands.EightBall())
	r.Register("roll", "roll dice (!roll 2d20)", commands.Roll())
	r.Register("calc", "arithmetic calculator", commands.Calc())
	r.Register("quote", "random inspirational quote", commands.Quote())
	r.Register("lyrics", "fetch song lyrics", commands.Lyrics())
	// Admin
	r.Register("broadcast", "owner: send to every joined group", commands.Broadcast())
	r.Register("block", "owner: block a user from the bot", commands.Block())
	r.Register("unblock", "owner: unblock a user", commands.Unblock())
	r.Register("restart", "owner: exit (expects supervisor to respawn)", commands.Restart())
	// Voice
	r.Register("tts", "text → voice note", commands.TTS())
	r.Register("ttsvoice", "pick Groq Orpheus voice for this chat", commands.TTSVoice())
	r.Register("voice", "speak text with a cloned voice", commands.Voice())
	r.Register("clonevoice", "owner: clone a voice from quoted voice note", commands.CloneVoice())
	r.Register("voices", "list cloned voices", commands.Voices())
	r.Register("stt", "reply to a voice note → transcribe (Groq Whisper)", commands.STT(), "transcribe")
	r.Register("sts", "reply to a voice note → re-voice as <name> (ElevenLabs)", commands.STS())
	// Extras
	r.Register("wiki", "Wikipedia summary", commands.Wiki())
	r.Register("define", "dictionary lookup", commands.Define(), "def")
	r.Register("urban", "Urban Dictionary", commands.Urban())
	r.Register("joke", "random joke", commands.Joke())
	r.Register("short", "shorten a URL (is.gd)", commands.Short())
	r.Register("qr", "generate a QR code image", commands.QR())
	r.Register("meme", "make a meme (memegen.link)", commands.Meme())
	// Menu last so it sees everything else.
	r.Register("menu", "show this list", commands.Menu(r.Help()), "help")

	// Wire whatsmeow event handler.
	cli.AddEventHandler(func(evt any) {
		switch e := evt.(type) {
		case *events.Message:
			r.Dispatch(ctx, e)
		case *events.GroupInfo:
			commands.GroupParticipantHandler(cli, st)(e)
		case *events.Connected:
			log.Info().Msg("connected")
		case *events.Disconnected:
			log.Warn().Msg("disconnected (whatsmeow will auto-reconnect)")
		case *events.LoggedOut:
			log.Error().Str("reason", e.Reason.String()).Msg("logged out — delete session/ and relink")
		}
	})

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

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info().Msg("shutting down")
	cli.Disconnect()
}

func panicLog(where string, err error) {
	os.Stderr.WriteString(where + ": " + err.Error() + "\n")
	os.Exit(1)
}
