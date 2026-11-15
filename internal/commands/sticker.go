package commands

import (
	"context"
	"os/exec"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/media"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Sticker converts a quoted image/video into a WebP sticker and sends it back.
// Requires ffmpeg on PATH.
func Sticker() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "sticker needs ffmpeg installed")
		}

		// Find the media: either in this message or in a reply.
		raw, mime, err := media.ExtractQuotedMedia(ctx, env.Cli, env.Msg)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat,
				"reply to an image or short video with !sticker")
		}

		webp, err := media.ToWebPSticker(ctx, raw, mime)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "sticker encode failed: "+err.Error())
		}

		up, err := env.Cli.Upload(ctx, webp, whatsmeow.MediaImage)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "upload failed: "+err.Error())
		}

		_, err = env.Cli.SendMessage(ctx, env.Chat, &waE2E.Message{
			StickerMessage: &waE2E.StickerMessage{
				URL:           &up.URL,
				FileEncSHA256: up.FileEncSHA256,
				FileSHA256:    up.FileSHA256,
				MediaKey:      up.MediaKey,
				Mimetype:      stringPtr("image/webp"),
				DirectPath:    &up.DirectPath,
				FileLength:    uint64Ptr(uint64(len(webp))),
			},
		})
		return err
	}
}

func stringPtr(s string) *string { return &s }
func uint64Ptr(u uint64) *uint64 { return &u }
