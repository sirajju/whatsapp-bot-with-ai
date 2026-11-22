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
		"-vn",
		"-c:a", "libopus",
		"-b:a", "32k",
		"-ar", "48000",
		"-ac", "1",
		"-avoid_negative_ts", "make_zero",
		"-f", "opus",
	)
}

// OggOpusFromAny converts arbitrary audio bytes (webm/ogg/wav/m4a) to an
// Opus-in-Ogg container WhatsApp renders and plays as a voice note.
func OggOpusFromAny(ctx context.Context, raw []byte, inExt string) ([]byte, error) {
	return ffmpegTranscode(ctx, raw, inExt, ".ogg",
		"-vn",
		"-c:a", "libopus",
		"-b:a", "32k",
		"-ar", "48000",
		"-ac", "1",
		"-avoid_negative_ts", "make_zero",
		"-f", "opus",
	)
}

// ProbeDurationSeconds asks ffprobe how long an audio file is (seconds,
// rounded down). Returns 0 and no error when ffprobe is absent or the
// file is unreadable — callers decide whether to care.
func ProbeDurationSeconds(ctx context.Context, raw []byte, inExt string) uint32 {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return 0
	}
	dir, err := os.MkdirTemp("", "wabot-probe-*")
	if err != nil {
		return 0
	}
	defer os.RemoveAll(dir)
	inPath := filepath.Join(dir, "in"+inExt)
	if err := os.WriteFile(inPath, raw, 0o644); err != nil {
		return 0
	}
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inPath,
	).Output()
	if err != nil {
		return 0
	}
	var f float64
	if _, err := fmt.Sscanf(string(bytes.TrimSpace(out)), "%f", &f); err != nil {
		return 0
	}
	if f <= 0 {
		return 0
	}
	return uint32(f)
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
