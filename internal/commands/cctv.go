package commands

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/config"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/intercom"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// CCTV routes:
//
//	!cctv                     → one-shot snapshot (wakes, shoots, sleeps).
//	!cctv on [seconds]        → wake the camera and hold it open for N seconds.
//	                            DMs back the stream URL(s). Default 120s, max
//	                            CCTV_LIVE_MAX (600s). Running `on` again before
//	                            the timer fires replaces the timer.
//	!cctv off                 → cancel the live timer + stop camera immediately.
//
// Owner only. When CCTV_CONTROL_TOPIC is empty or the intercom MQTT isn't
// up, snapshot-mode falls back to "always-on camera" and on/off are no-ops.
func CCTV(ic *intercom.Bridge) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}

		if len(env.Args) == 0 {
			return snapshotHandler(ctx, env, ic)
		}

		switch strings.ToLower(env.Args[0]) {
		case "on":
			seconds := int(env.Cfg.CCTVLiveDefault / time.Second)
			if len(env.Args) >= 2 {
				if n, err := strconv.Atoi(env.Args[1]); err == nil && n > 0 {
					seconds = n
				}
			}
			maxSec := int(env.Cfg.CCTVLiveMax / time.Second)
			if seconds > maxSec {
				seconds = maxSec
			}
			return liveOn(ctx, env, ic, seconds)
		case "off":
			return liveOff(ctx, env, ic)
		default:
			return sendText(ctx, env.Cli, env.Chat,
				"usage: !cctv  |  !cctv on [seconds]  |  !cctv off")
		}
	}
}

// --- snapshot (unchanged behaviour, just factored out) ---------------------

func snapshotHandler(ctx context.Context, env *router.Env, ic *intercom.Bridge) error {
	url := strings.TrimSpace(env.Cfg.CCTVURL)
	if url == "" {
		return sendText(ctx, env.Cli, env.Chat,
			"cctv: CCTV_URL not set in .env (e.g. http://192.168.1.231:8080/shot.jpg)")
	}

	useControl := env.Cfg.CCTVControlTopic != "" && ic != nil
	if useControl && !liveSessionActive() {
		_ = sendText(ctx, env.Cli, env.Chat, "📸 waking camera…")
		if err := ic.Publish(env.Cfg.CCTVControlTopic, []byte(`{"action":"start"}`)); err != nil {
			env.Log.Warn().Err(err).Msg("cctv: mqtt start publish failed, trying URL anyway")
		}
		if err := waitForCamera(ctx, url, env.Cfg); err != nil {
			_ = publishStop(ic, env.Cfg)
			return sendText(ctx, env.Cli, env.Chat, "cctv: "+err.Error())
		}
	} else {
		_ = sendText(ctx, env.Cli, env.Chat, "📸 fetching…")
	}

	body, mime, err := fetchSnap(ctx, url, env.Cfg)

	// If we started the camera ourselves and no live session is running,
	// put it back to sleep.
	if useControl && env.Cfg.CCTVAutoStop && !liveSessionActive() {
		if stopErr := publishStop(ic, env.Cfg); stopErr != nil {
			env.Log.Warn().Err(stopErr).Msg("cctv: mqtt stop publish failed")
		}
	}

	if err != nil {
		return sendText(ctx, env.Cli, env.Chat, "cctv: "+err.Error())
	}
	if err := uploadAndSendImage(ctx, env.Cli, env.Chat, body, mime); err != nil {
		return sendText(ctx, env.Cli, env.Chat, "cctv: upload failed — "+err.Error())
	}
	return nil
}

// --- live session state ---------------------------------------------------

var (
	liveMu     sync.Mutex
	liveCancel context.CancelFunc
	liveDue    time.Time
)

func liveSessionActive() bool {
	liveMu.Lock()
	defer liveMu.Unlock()
	return liveCancel != nil
}

func liveOn(ctx context.Context, env *router.Env, ic *intercom.Bridge, seconds int) error {
	url := strings.TrimSpace(env.Cfg.CCTVURL)
	if url == "" {
		return sendText(ctx, env.Cli, env.Chat, "cctv: CCTV_URL not set")
	}
	if ic == nil || env.Cfg.CCTVControlTopic == "" {
		return sendText(ctx, env.Cli, env.Chat,
			"cctv: on/off requires CCTV_CONTROL_TOPIC + working MQTT — not configured")
	}

	// Cancel any existing timer so we restart cleanly.
	liveMu.Lock()
	if liveCancel != nil {
		liveCancel()
		liveCancel = nil
	}
	liveMu.Unlock()

	_ = sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("📸 waking camera for %ds…", seconds))
	if err := ic.Publish(env.Cfg.CCTVControlTopic, []byte(`{"action":"start"}`)); err != nil {
		return sendText(ctx, env.Cli, env.Chat, "cctv: mqtt start failed — "+err.Error())
	}
	if err := waitForCamera(ctx, url, env.Cfg); err != nil {
		_ = publishStop(ic, env.Cfg)
		return sendText(ctx, env.Cli, env.Chat, "cctv: "+err.Error())
	}

	// Set up the auto-stop timer in a goroutine. It survives this request's
	// ctx cancellation — we use a detached background context.
	sessionCtx, cancel := context.WithCancel(context.Background())
	liveMu.Lock()
	liveCancel = cancel
	liveDue = time.Now().Add(time.Duration(seconds) * time.Second)
	liveMu.Unlock()

	chatJID := env.Chat
	cli := env.Cli
	log := env.Log
	topic := env.Cfg.CCTVControlTopic
	go func() {
		timer := time.NewTimer(time.Duration(seconds) * time.Second)
		defer timer.Stop()
		select {
		case <-sessionCtx.Done():
			// cancelled by `off` — stop is handled there
			return
		case <-timer.C:
			if err := ic.Publish(topic, []byte(`{"action":"stop"}`)); err != nil {
				log.Warn().Err(err).Msg("cctv: mqtt auto-stop failed")
			}
			liveMu.Lock()
			liveCancel = nil
			liveDue = time.Time{}
			liveMu.Unlock()
			_, _ = cli.SendMessage(context.Background(), chatJID, &waE2E.Message{
				Conversation: proto.String("📴 camera off (timer expired)"),
			})
		}
	}()

	msg := buildLiveMessage(env.Cfg, seconds)
	return sendText(ctx, cli, chatJID, msg)
}

func liveOff(ctx context.Context, env *router.Env, ic *intercom.Bridge) error {
	if ic == nil || env.Cfg.CCTVControlTopic == "" {
		return sendText(ctx, env.Cli, env.Chat,
			"cctv: off requires CCTV_CONTROL_TOPIC + working MQTT")
	}
	liveMu.Lock()
	cancelled := false
	if liveCancel != nil {
		liveCancel()
		liveCancel = nil
		liveDue = time.Time{}
		cancelled = true
	}
	liveMu.Unlock()

	if err := ic.Publish(env.Cfg.CCTVControlTopic, []byte(`{"action":"stop"}`)); err != nil {
		return sendText(ctx, env.Cli, env.Chat, "cctv: mqtt stop failed — "+err.Error())
	}
	msg := "📴 camera off"
	if !cancelled {
		msg += " (no live session was running — stop sent anyway)"
	}
	return sendText(ctx, env.Cli, env.Chat, msg)
}

// buildLiveMessage composes the MJPEG URL reply with optional external URL.
func buildLiveMessage(cfg *config.Config, seconds int) string {
	lan := resolveStreamURL(cfg.CCTVStreamURL, cfg.CCTVURL)
	lan = embedBasicAuth(lan, cfg.CCTVUser, cfg.CCTVPassword)

	var b strings.Builder
	fmt.Fprintf(&b, "📸 *camera up for %d s*\n", seconds)
	fmt.Fprintf(&b, "LAN: %s", lan)
	if cfg.CCTVStreamURLExt != "" {
		ext := embedBasicAuth(cfg.CCTVStreamURLExt, cfg.CCTVUser, cfg.CCTVPassword)
		fmt.Fprintf(&b, "\nExternal: %s", ext)
	}
	fmt.Fprintf(&b, "\nSend `!cctv off` to stop early.")
	return b.String()
}

// resolveStreamURL falls back to deriving an MJPEG URL from the snapshot
// endpoint ("/shot.jpg" → "/video") when CCTV_STREAM_URL isn't set.
func resolveStreamURL(streamURL, fallbackShotURL string) string {
	if streamURL != "" {
		return streamURL
	}
	if strings.Contains(fallbackShotURL, "/shot.jpg") {
		return strings.Replace(fallbackShotURL, "/shot.jpg", "/video", 1)
	}
	return fallbackShotURL
}

// embedBasicAuth injects user:pass@ into a URL so tapping it from WhatsApp
// opens the browser pre-authed, no credential prompt. Noop when user is empty.
func embedBasicAuth(url, user, pass string) string {
	if user == "" {
		return url
	}
	i := strings.Index(url, "://")
	if i < 0 {
		return url
	}
	return url[:i+3] + user + ":" + pass + "@" + url[i+3:]
}

// --- shared snapshot helpers ---------------------------------------------

func waitForCamera(ctx context.Context, url string, cfg *config.Config) error {
	deadline := time.Now().Add(cfg.CCTVStartTimeout)
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return fmt.Errorf("bad URL: %w", err)
	}
	if cfg.CCTVUser != "" {
		req.SetBasicAuth(cfg.CCTVUser, cfg.CCTVPassword)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := client.Do(req.Clone(ctx))
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode/100 == 2 {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("camera didn't come up within %s", cfg.CCTVStartTimeout)
}

func fetchSnap(ctx context.Context, url string, cfg *config.Config) ([]byte, string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, cfg.CCTVTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("bad URL — %w", err)
	}
	if cfg.CCTVUser != "" {
		req.SetBasicAuth(cfg.CCTVUser, cfg.CCTVPassword)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch failed — %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, "", fmt.Errorf("camera returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, "", fmt.Errorf("read failed — %w", err)
	}
	if len(body) < 1024 {
		return nil, "", fmt.Errorf("suspiciously small response (%d bytes) — check URL", len(body))
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "image/jpeg"
	}
	return body, mime, nil
}

func publishStop(ic *intercom.Bridge, cfg *config.Config) error {
	if ic == nil || cfg.CCTVControlTopic == "" {
		return nil
	}
	return ic.Publish(cfg.CCTVControlTopic, []byte(`{"action":"stop"}`))
}

// Silence unused-import until we inevitably reach for whatsmeow types elsewhere
// in this file (the goroutine already uses types.JID transitively via env.Chat
// parameter type, but the compiler's unused-import check needs us to touch
// whatsmeow.Client explicitly — hence this guard).
var _ = (*whatsmeow.Client)(nil)
var _ = types.JID{}
