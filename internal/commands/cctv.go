package commands

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/config"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/intercom"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// CCTV fetches a JPEG snapshot from a MJPEG/HTTP camera (IP Webcam on the
// wall phone) and posts it back as an image message. Owner only.
//
// Two modes based on whether CCTV_CONTROL_TOPIC is set in .env:
//
//   - Always-on camera (no control topic): just GET the snapshot URL.
//
//   - On-demand camera (control topic set, phone-side cam-control.sh
//     subscribing): publish {"action":"start"} on MQTT, wait until the
//     snapshot URL responds, grab the shot, publish {"action":"stop"}.
//     Keeps the phone's camera + network idle except for the few
//     seconds we need it.
//
// If the intercom MQTT isn't up when control is configured, we fall back
// to the always-on path.
func CCTV(ic *intercom.Bridge) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		url := strings.TrimSpace(env.Cfg.CCTVURL)
		if url == "" {
			return sendText(ctx, env.Cli, env.Chat,
				"cctv: CCTV_URL not set in .env (e.g. http://192.168.1.231:8080/shot.jpg)")
		}

		useControl := env.Cfg.CCTVControlTopic != "" && ic != nil
		if useControl {
			_ = sendText(ctx, env.Cli, env.Chat, "📸 waking camera…")
			if err := ic.Publish(env.Cfg.CCTVControlTopic, []byte(`{"action":"start"}`)); err != nil {
				// Camera might already be up from a previous call; fall through.
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

		if useControl && env.Cfg.CCTVAutoStop {
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
}

// waitForCamera polls the snapshot URL with HEAD requests until it answers
// 2xx or the start-timeout elapses.
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

// fetchSnap downloads the JPEG and returns the bytes + MIME.
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
