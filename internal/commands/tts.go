package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/media"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// TTS speaks the given text as a voice note. Tries providers in this order:
//   1. ElevenLabs (if KEY + VOICE_ID set)
//   2. Groq playai-tts (free, quality — kicks in if GROQ_API_KEY is set)
//   3. Microsoft Edge TTS (`edge-tts` CLI, if installed)
// Falls through to the next on a payment/quota/404 error so e.g. an ElevenLabs
// 402 on a library voice doesn't stop the command from working.
func TTS() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"tts <text>")
		}
		text := strings.Join(env.Args, " ")
		_ = sendText(ctx, env.Cli, env.Chat, "🔊 generating voice…")

		var mp3 []byte
		var lastErr error

		// 1. ElevenLabs
		if env.Cfg.ElevenLabsKey != "" && env.Cfg.ElevenLabsVoiceID != "" {
			b, err := elevenlabsTTS(ctx, env.Cfg.ElevenLabsKey, env.Cfg.ElevenLabsVoiceID, text)
			if err == nil {
				mp3 = b
			} else {
				lastErr = fmt.Errorf("elevenlabs: %w", err)
				env.Log.Warn().Err(err).Msg("elevenlabs tts failed, trying fallback")
			}
		}
		// 2. Groq Orpheus TTS (free)
		if mp3 == nil && env.Cfg.GroqKey != "" {
			voice := env.Store.GetTTSVoice(ctx, env.Chat.String())
			if voice == "" {
				voice = "tara"
			}
			b, err := groqTTS(ctx, env.Cfg.GroqKey, text, voice)
			if err == nil {
				mp3 = b
			} else {
				lastErr = fmt.Errorf("groq: %w", err)
				env.Log.Warn().Err(err).Msg("groq tts failed, trying fallback")
			}
		}
		// 3. edge-tts
		if mp3 == nil {
			b, err := edgeTTS(ctx, text, "en-US-AriaNeural")
			if err == nil {
				mp3 = b
			} else {
				lastErr = fmt.Errorf("edge-tts: %w", err)
			}
		}
		if mp3 == nil {
			msg := "no TTS provider worked"
			if lastErr != nil {
				msg = msg + ": " + lastErr.Error()
			}
			return sendText(ctx, env.Cli, env.Chat, msg)
		}
		return sendVoiceNoteFromMP3(ctx, env.Cli, env.Chat, mp3)
	}
}

// Voice uses a previously cloned ElevenLabs voice by name (from the voices table).
func Voice() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if env.Cfg.ElevenLabsKey == "" {
			return sendText(ctx, env.Cli, env.Chat, "set ELEVENLABS_API_KEY to use "+env.Cfg.Prefix+"voice")
		}
		if len(env.Args) < 2 {
			return sendText(ctx, env.Cli, env.Chat,
				"usage: "+env.Cfg.Prefix+"voice <name> <text>   (list with "+env.Cfg.Prefix+"voices)")
		}
		name := strings.ToLower(env.Args[0])
		text := strings.Join(env.Args[1:], " ")

		voiceID, ok := env.Store.GetVoice(ctx, name)
		if !ok {
			return sendText(ctx, env.Cli, env.Chat,
				"no voice named "+name+". List with "+env.Cfg.Prefix+"voices or clone one with "+env.Cfg.Prefix+"clonevoice")
		}
		_ = sendText(ctx, env.Cli, env.Chat, "🔊 generating voice…")
		mp3, err := elevenlabsTTS(ctx, env.Cfg.ElevenLabsKey, voiceID, text)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "voice: "+err.Error())
		}
		return sendVoiceNoteFromMP3(ctx, env.Cli, env.Chat, mp3)
	}
}

// CloneVoice (owner, reply to a voice note) uploads the sample to ElevenLabs
// and remembers its id under the given name.
func CloneVoice() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		if env.Cfg.ElevenLabsKey == "" {
			return sendText(ctx, env.Cli, env.Chat, "set ELEVENLABS_API_KEY to clone voices")
		}
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat,
				"usage: reply to a voice note with "+env.Cfg.Prefix+"clonevoice <name>")
		}
		name := strings.ToLower(env.Args[0])
		raw, err := extractQuotedVoiceNote(ctx, env.Cli, env.Msg)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "clonevoice: "+err.Error())
		}
		_ = sendText(ctx, env.Cli, env.Chat, "🧬 uploading sample & cloning…")

		mp3, err := media.MP3FromAny(ctx, raw, ".ogg")
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "transcode: "+err.Error())
		}
		voiceID, err := elevenlabsClone(ctx, env.Cfg.ElevenLabsKey, name, mp3)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "clone: "+err.Error())
		}
		if err := env.Store.SetVoice(ctx, name, voiceID); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "save: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat,
			"✅ voice *"+name+"* cloned. Use it with "+env.Cfg.Prefix+"voice "+name+" <text>")
	}
}

// OrpheusVoices are the voices available on the free Groq Orpheus TTS model.
var OrpheusVoices = []string{"tara", "leah", "jess", "leo", "dan", "mia", "zac", "zoe"}

// TTSVoice sets (or shows) the default Groq Orpheus voice for this chat.
func TTSVoice() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			current := env.Store.GetTTSVoice(ctx, env.Chat.String())
			if current == "" {
				current = "tara"
			}
			return sendText(ctx, env.Cli, env.Chat,
				fmt.Sprintf("current TTS voice: *%s*\navailable: %s\nchange: %sttsvoice <name>",
					current, strings.Join(OrpheusVoices, ", "), env.Cfg.Prefix))
		}
		name := strings.ToLower(env.Args[0])
		ok := false
		for _, v := range OrpheusVoices {
			if v == name {
				ok = true
				break
			}
		}
		if !ok {
			return sendText(ctx, env.Cli, env.Chat,
				"unknown voice. Available: "+strings.Join(OrpheusVoices, ", "))
		}
		if err := env.Store.SetTTSVoice(ctx, env.Chat.String(), name); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "ttsvoice: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, "🔊 TTS voice set to *"+name+"*")
	}
}

// Voices lists all saved cloned voices (ElevenLabs).
func Voices() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		names, err := env.Store.ListVoices(ctx)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "voices: "+err.Error())
		}
		if len(names) == 0 {
			return sendText(ctx, env.Cli, env.Chat,
				"no cloned voices. Reply to a voice note with "+env.Cfg.Prefix+"clonevoice <name>")
		}
		return sendText(ctx, env.Cli, env.Chat, "cloned voices:\n• "+strings.Join(names, "\n• "))
	}
}

// ---------- providers ----------

func elevenlabsTTS(ctx context.Context, key, voiceID, text string) ([]byte, error) {
	buf, _ := json.Marshal(map[string]any{
		"text":     text,
		"model_id": "eleven_turbo_v2_5",
	})
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.elevenlabs.io/v1/text-to-speech/"+voiceID, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", key)
	req.Header.Set("Content-Type", "application/json")
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

// elevenlabsClone uploads an mp3 sample via multipart/form-data and returns the new voice_id.
func elevenlabsClone(ctx context.Context, key, name string, mp3 []byte) (string, error) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("name", name)
	_ = w.WriteField("description", "Cloned from a WhatsApp voice note via wabot")
	part, err := w.CreateFormFile("files", "sample.mp3")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(mp3); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.elevenlabs.io/v1/voices/add", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("xi-api-key", key)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := httpCli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("elevenlabs http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var r struct {
		VoiceID string `json:"voice_id"`
	}
	if err := json.Unmarshal(data, &r); err != nil || r.VoiceID == "" {
		return "", fmt.Errorf("clone: no voice_id in response")
	}
	return r.VoiceID, nil
}

// groqTTS uses Groq's Canopy Labs Orpheus model (free, English).
// Voices on orpheus-v1-english: tara (default), leah, jess, leo, dan, mia, zac, zoe.
// Produces WAV which sendVoiceNoteFromMP3 re-encodes to ogg/opus through ffmpeg.
func groqTTS(ctx context.Context, key, text, voice string) ([]byte, error) {
	body := map[string]any{
		"model":           "canopylabs/orpheus-v1-english",
		"voice":           voice,
		"input":           text,
		"response_format": "wav",
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.groq.com/openai/v1/audio/speech", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		errb, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(errb))
	}
	return io.ReadAll(resp.Body)
}

// edgeTTS shells to the `edge-tts` CLI (free Microsoft Edge TTS; `pipx install edge-tts`).
func edgeTTS(ctx context.Context, text, voice string) ([]byte, error) {
	if _, err := exec.LookPath("edge-tts"); err != nil {
		return nil, fmt.Errorf("edge-tts not installed and ELEVENLABS_API_KEY not set. Install: pipx install edge-tts")
	}
	dir, err := os.MkdirTemp("", "wabot-tts-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	outPath := filepath.Join(dir, "out.mp3")
	cmd := exec.CommandContext(ctx, "edge-tts",
		"--voice", voice, "--text", text, "--write-media", outPath)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("edge-tts: %w: %s", err, errb.String())
	}
	return os.ReadFile(outPath)
}

// ---------- sending / extracting ----------

func sendVoiceNoteFromMP3(ctx context.Context, cli *whatsmeow.Client, chat types.JID, mp3 []byte) error {
	ogg, err := media.OggOpusFromMP3(ctx, mp3)
	if err != nil {
		return fmt.Errorf("ogg encode: %w", err)
	}
	up, err := cli.Upload(ctx, ogg, whatsmeow.MediaAudio)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	seconds := media.ProbeDurationSeconds(ctx, ogg, ".ogg")
	if seconds == 0 {
		seconds = 1
	}
	now := time.Now().Unix()
	flen := up.FileLength
	if flen == 0 {
		flen = uint64(len(ogg))
	}
	ptt := true
	mime := "audio/ogg; codecs=opus"
	_, err = cli.SendMessage(ctx, chat, &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:               &up.URL,
			FileEncSHA256:     up.FileEncSHA256,
			FileSHA256:        up.FileSHA256,
			MediaKey:          up.MediaKey,
			Mimetype:          &mime,
			DirectPath:        &up.DirectPath,
			FileLength:        &flen,
			PTT:               &ptt,
			Seconds:           &seconds,
			StreamingSidecar:  up.StreamingSidecar,
			MediaKeyTimestamp: &now,
		},
	})
	return err
}

// extractQuotedVoiceNote pulls raw audio bytes from either the current message's
// audio attachment or its quoted message's audio attachment.
func extractQuotedVoiceNote(ctx context.Context, cli *whatsmeow.Client, evt *events.Message) ([]byte, error) {
	if a := evt.Message.GetAudioMessage(); a != nil {
		return cli.Download(ctx, a)
	}
	ext := evt.Message.GetExtendedTextMessage()
	if ext == nil {
		return nil, fmt.Errorf("reply to a voice note (or send one with the command as caption)")
	}
	q := ext.GetContextInfo().GetQuotedMessage()
	if q == nil {
		return nil, fmt.Errorf("no quoted message")
	}
	if a := q.GetAudioMessage(); a != nil {
		return cli.Download(ctx, a)
	}
	return nil, fmt.Errorf("quoted message is not audio")
}

func boolPtr(b bool) *bool { return &b }
