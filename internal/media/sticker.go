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
	"go.mau.fi/whatsmeow/types/events"
)

// ExtractQuotedMedia returns the raw bytes + mime type of the image/video
// attached to the current message or (if replied) its quoted message.
func ExtractQuotedMedia(ctx context.Context, cli *whatsmeow.Client, evt *events.Message) ([]byte, string, error) {
	if img := evt.Message.GetImageMessage(); img != nil {
		b, err := cli.Download(ctx, img)
		return b, img.GetMimetype(), err
	}
	if vid := evt.Message.GetVideoMessage(); vid != nil {
		b, err := cli.Download(ctx, vid)
		return b, vid.GetMimetype(), err
	}
	ext := evt.Message.GetExtendedTextMessage()
	if ext == nil {
		return nil, "", fmt.Errorf("no media")
	}
	q := ext.GetContextInfo().GetQuotedMessage()
	if q == nil {
		return nil, "", fmt.Errorf("no quoted media")
	}
	if img := q.GetImageMessage(); img != nil {
		b, err := cli.Download(ctx, img)
		return b, img.GetMimetype(), err
	}
	if vid := q.GetVideoMessage(); vid != nil {
		b, err := cli.Download(ctx, vid)
		return b, vid.GetMimetype(), err
	}
	return nil, "", fmt.Errorf("no image or video to convert")
}

// ExtractQuotedSticker returns raw webp bytes of a quoted (or current) sticker.
func ExtractQuotedSticker(ctx context.Context, cli *whatsmeow.Client, evt *events.Message) ([]byte, error) {
	if s := evt.Message.GetStickerMessage(); s != nil {
		return cli.Download(ctx, s)
	}
	ext := evt.Message.GetExtendedTextMessage()
	if ext == nil {
		return nil, fmt.Errorf("no sticker")
	}
	q := ext.GetContextInfo().GetQuotedMessage()
	if q == nil {
		return nil, fmt.Errorf("no quoted sticker")
	}
	if s := q.GetStickerMessage(); s != nil {
		return cli.Download(ctx, s)
	}
	return nil, fmt.Errorf("quoted message is not a sticker")
}

// ToWebPSticker re-encodes raw media into a 512x512 WebP (animated if video/*).
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

	args := buildFFmpegArgs(inPath, outPath, mime)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	return os.ReadFile(outPath)
}

func buildFFmpegArgs(inPath, outPath, mime string) []string {
	if strings.HasPrefix(mime, "video/") {
		return []string{
			"-y", "-i", inPath,
			"-vcodec", "libwebp",
			"-vf", "scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=0x00000000,fps=12",
			"-loop", "0", "-ss", "0", "-t", "6",
			"-preset", "default", "-an", "-vsync", "0",
			outPath,
		}
	}
	return []string{
		"-y", "-i", inPath,
		"-vcodec", "libwebp",
		"-vf", "scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=0x00000000",
		"-preset", "default",
		outPath,
	}
}

// StickerToJPEG re-encodes a webp sticker back to jpeg.
func StickerToJPEG(ctx context.Context, webp []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "wabot-toimg-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "in.webp")
	outPath := filepath.Join(tmpDir, "out.jpg")
	if err := os.WriteFile(inPath, webp, 0o644); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y", "-i", inPath, "-frames:v", "1", "-q:v", "2", outPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	return os.ReadFile(outPath)
}

// TextSticker renders text as a 512x512 WebP sticker via ffmpeg drawtext.
func TextSticker(ctx context.Context, text string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "wabot-stext-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	outPath := filepath.Join(tmpDir, "out.webp")
	safe := strings.NewReplacer(`\`, "", `:`, " ", `'`, "", `"`, "").Replace(text)
	font := "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
	if _, err := os.Stat(font); err != nil {
		font = ""
	}
	drawtext := "drawtext=text='" + safe +
		"':fontcolor=white:fontsize=64:box=1:boxcolor=black@0.6:boxborderw=12:x=(w-text_w)/2:y=(h-text_h)/2"
	if font != "" {
		drawtext = "drawtext=fontfile=" + font + ":text='" + safe +
			"':fontcolor=white:fontsize=64:box=1:boxcolor=black@0.6:boxborderw=12:x=(w-text_w)/2:y=(h-text_h)/2"
	}
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y", "-f", "lavfi", "-i", "color=c=black@0:s=512x512:d=1",
		"-frames:v", "1", "-vf", drawtext,
		"-vcodec", "libwebp", outPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	return os.ReadFile(outPath)
}
