package commands

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// CCTV fetches a JPEG snapshot from a MJPEG/HTTP camera (currently IP Webcam
// on the wall phone) and posts it back as an image message. Owner only.
//
// Config:
//
//	CCTV_URL         - full URL to the snapshot endpoint (e.g.
//	                   http://192.168.1.231:8080/shot.jpg). Required.
//	CCTV_USER        - optional HTTP basic-auth user.
//	CCTV_PASSWORD    - optional HTTP basic-auth password.
//	CCTV_TIMEOUT     - optional, default 15s.
func CCTV() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		url := strings.TrimSpace(env.Cfg.CCTVURL)
		if url == "" {
			return sendText(ctx, env.Cli, env.Chat,
				"cctv: CCTV_URL not set in .env (e.g. http://192.168.1.231:8080/shot.jpg)")
		}

		_ = sendText(ctx, env.Cli, env.Chat, "📸 fetching…")

		timeout := env.Cfg.CCTVTimeout
		if timeout == 0 {
			timeout = 15 * time.Second
		}
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "cctv: bad URL — "+err.Error())
		}
		if env.Cfg.CCTVUser != "" {
			req.SetBasicAuth(env.Cfg.CCTVUser, env.Cfg.CCTVPassword)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "cctv: fetch failed — "+err.Error())
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return sendText(ctx, env.Cli, env.Chat,
				fmt.Sprintf("cctv: camera returned HTTP %d", resp.StatusCode))
		}
		// Cap at ~4 MB — an IP Webcam JPEG at 1280×720/50 is ~100–300 kB.
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "cctv: read failed — "+err.Error())
		}
		if len(body) < 1024 {
			return sendText(ctx, env.Cli, env.Chat,
				fmt.Sprintf("cctv: suspiciously small response (%d bytes) — check URL", len(body)))
		}

		mime := resp.Header.Get("Content-Type")
		if mime == "" {
			mime = "image/jpeg"
		}
		if err := uploadAndSendImage(ctx, env.Cli, env.Chat, body, mime); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "cctv: upload failed — "+err.Error())
		}
		return nil
	}
}
