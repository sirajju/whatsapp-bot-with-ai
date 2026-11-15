package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// STT transcribes a quoted voice note using Groq's whisper-large-v3-turbo
// (OpenAI-compatible endpoint). Falls back to a clear error if GROQ_API_KEY not set.
func STT() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if env.Cfg.GroqKey == "" {
			return sendText(ctx, env.Cli, env.Chat,
				"set GROQ_API_KEY to use "+env.Cfg.Prefix+"stt (uses whisper-large-v3-turbo)")
		}
		raw, err := extractQuotedVoiceNote(ctx, env.Cli, env.Msg)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat,
				"reply to a voice note with "+env.Cfg.Prefix+"stt")
		}
		_ = sendText(ctx, env.Cli, env.Chat, "✍️ transcribing…")

		text, err := groqWhisper(ctx, env.Cfg.GroqKey, raw)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "stt: "+err.Error())
		}
		if text == "" {
			text = "(silence)"
		}
		return sendText(ctx, env.Cli, env.Chat, "📝 "+text)
	}
}

// groqWhisper POSTs audio to Groq's /audio/transcriptions (OpenAI-compatible).
// Groq accepts ogg/opus directly (what WhatsApp voice notes are), no transcode needed.
func groqWhisper(ctx context.Context, key string, audio []byte) (string, error) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("model", "whisper-large-v3-turbo")
	_ = w.WriteField("response_format", "text")
	part, err := w.CreateFormFile("file", "voice.ogg")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.groq.com/openai/v1/audio/transcriptions", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := httpCli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("groq http %d: %s", resp.StatusCode, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
