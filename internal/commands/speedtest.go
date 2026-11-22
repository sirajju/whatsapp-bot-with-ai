package commands

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Speedtest measures ping latency, download speed, and upload speed using Cloudflare edge CDN.
func Speedtest() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		_ = sendText(ctx, env.Cli, env.Chat, "⚡ Running network speedtest… please wait ~3s.")

		client := &http.Client{Timeout: 10 * time.Second}

		// 1. Latency test
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://speed.cloudflare.com/__down?bytes=0", nil)
		var latency time.Duration
		if err == nil {
			if resp, err := client.Do(req); err == nil {
				_ = resp.Body.Close()
				latency = time.Since(start)
			}
		}

		// 2. Download test (10MB)
		dlStart := time.Now()
		dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://speed.cloudflare.com/__down?bytes=10000000", nil)
		var dlMbps float64
		if err == nil {
			if resp, err := client.Do(dlReq); err == nil {
				n, _ := io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				dlDuration := time.Since(dlStart).Seconds()
				if dlDuration > 0 && n > 0 {
					dlMbps = (float64(n) * 8) / (dlDuration * 1000000)
				}
			}
		}

		// 3. Upload test (3MB)
		upPayload := make([]byte, 3000000)
		_, _ = rand.Read(upPayload)
		upStart := time.Now()
		upReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://speed.cloudflare.com/__up", bytes.NewReader(upPayload))
		var upMbps float64
		if err == nil {
			upReq.Header.Set("Content-Type", "application/octet-stream")
			if resp, err := client.Do(upReq); err == nil {
				_ = resp.Body.Close()
				upDuration := time.Since(upStart).Seconds()
				if upDuration > 0 {
					upMbps = (float64(len(upPayload)) * 8) / (upDuration * 1000000)
				}
			}
		}

		msg := fmt.Sprintf(
			"⚡ *Network Speedtest Results*\n\n"+
				"• *Download*: %.1f Mbps\n"+
				"• *Upload*: %.1f Mbps\n"+
				"• *Latency*: %d ms\n"+
				"• *Server*: 192.168.1.230\n"+
				"• *Edge*: Cloudflare CDN",
			dlMbps, upMbps, latency.Milliseconds(),
		)
		return sendText(ctx, env.Cli, env.Chat, msg)
	}
}
