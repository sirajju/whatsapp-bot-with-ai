// Package media handles media download and sticker encoding.
package media

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
	"go.mau.fi/whatsmeow/types/events"
)

// ExtractQuotedMedia returns the raw bytes + mime type of the image/video attached to
// the current message or (if replied) its quoted message. Error if nothing qualifies.
func ExtractQuotedMedia(ctx context.Context, cli *whatsmeow.Client, evt *events.Message) ([]byte, string, error) {
	// 1. Direct attach on this message
	if img := evt.Message.GetImageMessage(); img != nil {
		b, err := cli.Download(ctx, img)
		return b, img.GetMimetype(), err
	}
	if vid := evt.Message.GetVideoMessage(); vid != nil {
		b, err := cli.Download(ctx, vid)
		return b, vid.GetMimetype(), err
	}

	// 2. Reply (quoted) media
	ext := evt.Message.GetExtendedTextMessage()
	if ext == nil {
		return nil, "", fmt.Errorf("no media")
	}
	q := ext.GetContextInfo().GetQuotedMessage()
	if q == nil {
		return nil, "", fmt.Errorf("no quoted media")
	}
	if img := q.GetImageMessage(); img != nil {
		b, err := downloadQuoted(ctx, cli, img)
		return b, img.GetMimetype(), err
	}
	if vid := q.GetVideoMessage(); vid != nil {
		b, err := downloadQuoted(ctx, cli, vid)
		return b, vid.GetMimetype(), err
	}
	return nil, "", fmt.Errorf("no image or video to convert")
}

// downloadQuoted re-wraps a quoted media message so cli.Download() sees a standalone one.
func downloadQuoted(ctx context.Context, cli *whatsmeow.Client, m any) ([]byte, error) {
	switch v := m.(type) {
	case *waE2E.ImageMessage:
		return cli.Download(ctx, v)
	case *waE2E.VideoMessage:
		return cli.Download(ctx, v)
	default:
		return nil, fmt.Errorf("unsupported quoted media type")
	}
}

// ToWebPSticker re-encodes raw media into a 512x512 WebP (image if image/*, animated if video/*).
func ToWebPSticker(ctx context.Context, raw []byte, mime string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "wabot-sticker-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	inExt := ".jpg"
	switch {
	case strings.HasPrefix(mime, "video/"):
		inExt = ".mp4"
	case strings.Contains(mime, "png"):
		inExt = ".png"
	case strings.Contains(mime, "webp"):
		inExt = ".webp"
	}

	inPath := filepath.Join(tmpDir, "in"+inExt)
	outPath := filepath.Join(tmpDir, "out.webp")
	if err := os.WriteFile(inPath, raw, 0o644); err != nil {
		return nil, err
	}

	var args []string
	if strings.HasPrefix(mime, "video/") {
		args = []string{
			"-y", "-i", inPath,
			"-vcodec", "libwebp",
			"-vf", "scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=0x00000000,fps=12",
			"-loop", "0", "-ss", "0", "-t", "6",
			"-preset", "default", "-an", "-vsync", "0",
			outPath,
		}
	} else {
		args = []string{
			"-y", "-i", inPath,
			"-vcodec", "libwebp",
			"-vf", "scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=0x00000000",
			"-preset", "default",
			outPath,
		}
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	return os.ReadFile(outPath)
}
