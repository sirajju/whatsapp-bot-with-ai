package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/media"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// STS (speech-to-speech) re-voices a quoted voice note in the given cloned voice.
// Usage: reply to a voice note with /sts <voice_name>
func STS() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if env.Cfg.ElevenLabsKey == "" {
			return sendText(ctx, env.Cli, env.Chat, "set ELEVENLABS_API_KEY to use "+env.Cfg.Prefix+"sts")
		}
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat,
				"usage: reply to a voice note with "+env.Cfg.Prefix+"sts <voice_name>")
		}
		name := strings.ToLower(env.Args[0])
		voiceID, ok := env.Store.GetVoice(ctx, name)
		if !ok {
			// Allow default voice too
			if name == "default" && env.Cfg.ElevenLabsVoiceID != "" {
				voiceID = env.Cfg.ElevenLabsVoiceID
			} else {
				return sendText(ctx, env.Cli, env.Chat,
					"no voice named "+name+". List with "+env.Cfg.Prefix+"voices")
			}
		}

		raw, err := extractQuotedVoiceNote(ctx, env.Cli, env.Msg)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "sts: "+err.Error())
		}
		_ = sendText(ctx, env.Cli, env.Chat, "🎙️ re-voicing…")

		mp3In, err := media.MP3FromAny(ctx, raw, ".ogg")
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "transcode in: "+err.Error())
		}
		mp3Out, err := elevenlabsSTS(ctx, env.Cfg.ElevenLabsKey, voiceID, mp3In)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "sts: "+err.Error())
		}
		return sendVoiceNoteFromMP3(ctx, env.Cli, env.Chat, mp3Out)
	}
}

// elevenlabsSTS POSTs audio bytes to ElevenLabs's speech-to-speech endpoint and
// returns the re-voiced MP3.
func elevenlabsSTS(ctx context.Context, key, voiceID string, mp3 []byte) ([]byte, error) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("model_id", "eleven_english_sts_v2")
	part, err := w.CreateFormFile("audio", "input.mp3")
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(mp3); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.elevenlabs.io/v1/speech-to-speech/"+voiceID, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", key)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "audio/mpeg")
	resp, err := httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		errb, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf("elevenlabs http %d: %s", resp.StatusCode, string(errb))
	}
	return io.ReadAll(resp.Body)
}
