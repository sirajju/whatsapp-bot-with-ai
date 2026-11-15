package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Download shells out to yt-dlp to grab a short video from any supported site
// (YouTube, TikTok, Instagram Reel, Twitter/X, Facebook...) and sends it back.
func Download() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if _, err := exec.LookPath("yt-dlp"); err != nil {
			return sendText(ctx, env.Cli, env.Chat,
				"yt-dlp not installed. On the server: sudo apt install yt-dlp (or pip install yt-dlp)")
		}
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("usage: %sdl <url>", env.Cfg.Prefix))
		}
		url := env.Args[0]
		if !strings.HasPrefix(url, "http") {
			return sendText(ctx, env.Cli, env.Chat, "pass a full http(s) URL")
		}

		tmp, err := os.MkdirTemp("", "wabot-dl-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		out := filepath.Join(tmp, "v.mp4")
		// Cap at ~60s and ~25 MB to avoid WhatsApp upload limits.
		cmd := exec.CommandContext(ctx, "yt-dlp",
			"-f", "mp4[filesize<25M]/best[filesize<25M]/best",
			"--no-playlist", "--max-filesize", "25M",
			"-o", out, url)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "download failed: "+truncErr(stderr.String()))
		}

		raw, err := os.ReadFile(out)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "read result: "+err.Error())
		}
		up, err := env.Cli.Upload(ctx, raw, whatsmeow.MediaVideo)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "upload: "+err.Error())
		}
		_, err = env.Cli.SendMessage(ctx, env.Chat, &waE2E.Message{
			VideoMessage: &waE2E.VideoMessage{
				URL:           &up.URL,
				FileEncSHA256: up.FileEncSHA256,
				FileSHA256:    up.FileSHA256,
				MediaKey:      up.MediaKey,
				Mimetype:      stringPtr("video/mp4"),
				DirectPath:    &up.DirectPath,
				FileLength:    uint64Ptr(uint64(len(raw))),
			},
		})
		return err
	}
}

func truncErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
