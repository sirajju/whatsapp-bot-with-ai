// Package media — audio helpers for TTS + voice cloning.
package media

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// OggOpusFromMP3 converts MP3 bytes to an Opus-in-Ogg container so WhatsApp
// renders the result as a voice note.
func OggOpusFromMP3(ctx context.Context, mp3 []byte) ([]byte, error) {
	return ffmpegTranscode(ctx, mp3, ".mp3", ".ogg",
		"-c:a", "libopus", "-b:a", "32k", "-vbr", "on", "-ac", "1")
}

// OggOpusFromAny converts arbitrary audio bytes (webm/ogg/wav/m4a) to an
// Opus-in-Ogg container so WhatsApp renders the result as a voice note.
func OggOpusFromAny(ctx context.Context, raw []byte, inExt string) ([]byte, error) {
	return ffmpegTranscode(ctx, raw, inExt, ".ogg",
		"-c:a", "libopus", "-b:a", "32k", "-vbr", "on", "-ac", "1")
}

// MP3FromAny converts arbitrary audio bytes (ogg/opus/wav/m4a) to a 44.1 kHz mono MP3
// — ElevenLabs's /voices/add endpoint likes compact MP3 samples.
func MP3FromAny(ctx context.Context, raw []byte, inExt string) ([]byte, error) {
	return ffmpegTranscode(ctx, raw, inExt, ".mp3",
		"-ar", "44100", "-ac", "1", "-b:a", "64k")
}

func ffmpegTranscode(ctx context.Context, raw []byte, inExt, outExt string, extra ...string) ([]byte, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not installed")
	}
	dir, err := os.MkdirTemp("", "wabot-aud-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	inPath := filepath.Join(dir, "in"+inExt)
	outPath := filepath.Join(dir, "out"+outExt)
	if err := os.WriteFile(inPath, raw, 0o644); err != nil {
		return nil, err
	}
	args := append([]string{"-y", "-i", inPath}, extra...)
	args = append(args, outPath)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	return os.ReadFile(outPath)
}
