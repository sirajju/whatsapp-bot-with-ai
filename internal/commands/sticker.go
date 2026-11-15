package commands

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/media"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Sticker converts a quoted image or video into a WebP sticker.
func Sticker() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "sticker needs ffmpeg installed")
		}
		raw, mime, err := media.ExtractQuotedMedia(ctx, env.Cli, env.Msg)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "reply to an image or short video with !sticker")
		}
		webp, err := media.ToWebPSticker(ctx, raw, mime)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "encode: "+err.Error())
		}
		return uploadSticker(ctx, env.Cli, env.Chat, webp)
	}
}

// Toimg reverses a sticker back into a JPEG image.
func Toimg() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "toimg needs ffmpeg installed")
		}
		raw, err := media.ExtractQuotedSticker(ctx, env.Cli, env.Msg)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "reply to a sticker with !toimg")
		}
		jpg, err := media.StickerToJPEG(ctx, raw)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "decode: "+err.Error())
		}
		up, err := env.Cli.Upload(ctx, jpg, whatsmeow.MediaImage)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "upload: "+err.Error())
		}
		_, err = env.Cli.SendMessage(ctx, env.Chat, &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				URL:           &up.URL,
				FileEncSHA256: up.FileEncSHA256,
				FileSHA256:    up.FileSHA256,
				MediaKey:      up.MediaKey,
				Mimetype:      stringPtr("image/jpeg"),
				DirectPath:    &up.DirectPath,
				FileLength:    uint64Ptr(uint64(len(jpg))),
			},
		})
		return err
	}
}

// Stext builds a text-only sticker from the command args.
func Stext() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "stext needs ffmpeg installed")
		}
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("usage: %sstext <text>", env.Cfg.Prefix))
		}
		text := strings.Join(env.Args, " ")
		webp, err := media.TextSticker(ctx, text)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "stext: "+err.Error())
		}
		return uploadSticker(ctx, env.Cli, env.Chat, webp)
	}
}

func uploadSticker(ctx context.Context, cli *whatsmeow.Client, chat any, webp []byte) error {
	type chatJID interface{ String() string }
	jid := chat.(interface {
		User() string
	})
	_ = jid
	// We pass chat.(types.JID) in callers; this helper stays typed where needed.
	return nil
}
