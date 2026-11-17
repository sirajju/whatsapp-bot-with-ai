package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/config"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Presence skill — thin HTTP client for the wifi-presence service
// (github.com/sirajju/wifi-presence). This file reuses only its public API —
// no internal types or code are vendored in, by design.

const presenceDefaultURL = "http://127.0.0.1:8765"

// --- shared transport ------------------------------------------------------

func presenceURL() string {
	if v := strings.TrimRight(os.Getenv("PRESENCE_API_URL"), "/"); v != "" {
		return v
	}
	return presenceDefaultURL
}

var presenceHTTP = &http.Client{Timeout: 5 * time.Second}

func presenceGet(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, presenceURL()+path, nil)
	if err != nil {
		return err
	}
	resp, err := presenceHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("presence: %s — %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func presencePost(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, presenceURL()+path, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := presenceHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("presence: %s — %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

// --- shape of what the service returns -------------------------------------

type presenceDevice struct {
	MAC      string `json:"mac"`
	Label    string `json:"label"`
	Owner    string `json:"owner"`
	Vendor   string `json:"vendor"`
	Hostname string `json:"hostname"`
	LastIP   string `json:"last_ip"`
	LastSeen string `json:"last_seen"`
}

type presenceStatusResp struct {
	LastScan string            `json:"last_scan"`
	Home     []presenceDevice  `json:"home"`
	Away     []presenceDevice  `json:"away"`
	Counts   map[string]int    `json:"counts"`
}

type presenceDeviceFull struct {
	presenceDevice
	FirstSeen string `json:"first_seen"`
	Home      bool   `json:"home"`
}

type presenceEvent struct {
	ID       int64  `json:"id"`
	TS       string `json:"ts"`
	MAC      string `json:"mac"`
	Kind     string `json:"kind"`
	Payload  string `json:"payload"`
	Label    string `json:"label"`
	Hostname string `json:"hostname,omitempty"`
}

// --- !home -----------------------------------------------------------------

func Home() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		var s presenceStatusResp
		if err := presenceGet(ctx, "/api/status", &s); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "presence: "+err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat, renderStatus(&s))
	}
}

func renderStatus(s *presenceStatusResp) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Home: %d  ·  Away: %d\n", s.Counts["home"], s.Counts["away"])
	if ls, err := time.Parse(time.RFC3339, s.LastScan); err == nil {
		fmt.Fprintf(&b, "Last scan: %s ago\n", shortDur(time.Since(ls)))
	}
	b.WriteString("\n")
	if len(s.Home) == 0 {
		b.WriteString("Nobody home.\n")
	} else {
		b.WriteString("— home —\n")
		for _, d := range s.Home {
			fmt.Fprintf(&b, "• %s\n", deviceLine(d))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- !devices --------------------------------------------------------------

func DevicesList() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		var devs []presenceDeviceFull
		if err := presenceGet(ctx, "/api/devices", &devs); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "presence: "+err.Error())
		}
		sort.Slice(devs, func(i, j int) bool {
			if devs[i].Home != devs[j].Home {
				return devs[i].Home
			}
			return devs[i].LastSeen > devs[j].LastSeen
		})
		var b strings.Builder
		fmt.Fprintf(&b, "%d device(s):\n", len(devs))
		for _, d := range devs {
			flag := "·"
			if d.Home {
				flag = "+"
			}
			fmt.Fprintf(&b, "%s %s\n", flag, deviceLine(d.presenceDevice))
		}
		return sendText(ctx, env.Cli, env.Chat, strings.TrimRight(b.String(), "\n"))
	}
}

// --- !label <mac> <name> [--owner <person>] --------------------------------

func LabelDevice() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		if len(env.Args) < 2 {
			return sendText(ctx, env.Cli, env.Chat,
				"usage: "+env.Cfg.Prefix+"label <mac> <name> [owner]\n"+
					"e.g.  "+env.Cfg.Prefix+"label aa:bb:cc:dd:ee:ff \"Siraj phone\" Siraj")
		}
		mac := strings.ToLower(env.Args[0])
		owner := ""
		// If more than 2 args, last word is treated as owner tag.
		label := strings.Join(env.Args[1:], " ")
		if len(env.Args) >= 3 {
			owner = env.Args[len(env.Args)-1]
			label = strings.Join(env.Args[1:len(env.Args)-1], " ")
		}
		payload := map[string]string{"mac": mac, "label": label, "owner": owner}
		if err := presencePost(ctx, "/api/label", payload); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "presence: "+err.Error())
		}
		if owner != "" {
			return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("labeled %s → %q (owner: %s)", mac, label, owner))
		}
		return sendText(ctx, env.Cli, env.Chat, fmt.Sprintf("labeled %s → %q", mac, label))
	}
}

// --- alerter goroutine -----------------------------------------------------

// StartPresenceAlerter polls the presence service for new events and forwards
// arrival / departure / new_device / house state alerts to the first configured
// owner. No-op if PRESENCE_ALERTS is anything other than "on".
func StartPresenceAlerter(ctx context.Context, cli *whatsmeow.Client, cfg *config.Config, log zerolog.Logger) {
	if strings.ToLower(os.Getenv("PRESENCE_ALERTS")) != "on" {
		log.Info().Msg("presence alerts disabled (PRESENCE_ALERTS!=on)")
		return
	}
	if len(cfg.Owners) == 0 {
		log.Warn().Msg("presence alerts: no OWNERS configured; skipping")
		return
	}
	ownerJID := types.JID{User: cfg.Owners[0], Server: types.DefaultUserServer}
	go func() {
		// Start at "now" so the first tick doesn't dump history on the owner.
		since := time.Now()
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		log := log.With().Str("component", "presence-alerter").Logger()
		log.Info().Str("owner", ownerJID.String()).Msg("presence alerter started")

		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			sinceStr := since.UTC().Format(time.RFC3339)
			q := url.Values{"since": []string{sinceStr}, "limit": []string{"100"}}
			var evs []presenceEvent
			if err := presenceGet(ctx, "/api/events?"+q.Encode(), &evs); err != nil {
				log.Debug().Err(err).Msg("poll failed")
				continue
			}
			for _, e := range evs {
				msg := formatEvent(e)
				if msg == "" {
					continue
				}
				if err := sendText(ctx, cli, ownerJID, msg); err != nil {
					log.Error().Err(err).Str("kind", e.Kind).Msg("alert send failed")
					continue
				}
				// Advance since to just after this event to dedupe.
				if t, err := time.Parse(time.RFC3339, e.TS); err == nil {
					since = t.Add(time.Second)
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()
}

// --- formatting helpers ----------------------------------------------------

func deviceLine(d presenceDevice) string {
	// Prefer a human-set label; then the device's own hostname (NBNS / rDNS);
	// then the owner tag; then whatever vendor the OUI table could tell us —
	// and only fall back to the MAC when nothing else is available.
	name := d.Label
	if name == "" {
		name = d.Hostname
	}
	if name == "" {
		name = d.Owner
	}
	if name == "" && d.Vendor != "" && d.Vendor != "Randomized" {
		name = d.Vendor
	}
	if name == "" {
		name = d.MAC
	}
	meta := []string{}
	// If we fell back to a name OTHER than the MAC, surface the MAC in the
	// meta so you can still !label it or correlate across sessions.
	if name != d.MAC {
		meta = append(meta, d.MAC)
	}
	if d.Vendor != "" && d.Vendor != name {
		meta = append(meta, d.Vendor)
	}
	if d.LastIP != "" {
		meta = append(meta, d.LastIP)
	}
	if t, err := time.Parse(time.RFC3339, d.LastSeen); err == nil {
		meta = append(meta, shortDur(time.Since(t))+" ago")
	}
	if len(meta) == 0 {
		return name
	}
	return name + " (" + strings.Join(meta, ", ") + ")"
}

func formatEvent(e presenceEvent) string {
	// Prefer the human label; then the device's own hostname (DHCP/NBNS);
	// only fall back to the MAC when nothing else is available.
	name := e.Label
	if name == "" {
		name = e.Hostname
	}
	if name == "" {
		name = e.MAC
	}
	switch e.Kind {
	case "arrived":
		return name + " came home"
	case "departed":
		return name + " left"
	case "new_device":
		vendor := e.Payload
		if vendor == "" {
			vendor = "unknown vendor"
		}
		return "New device joined: " + vendor + " (" + e.MAC + ")\n" +
			"Label it with: !label " + e.MAC + " <name>"
	case "house_empty":
		return "House is empty."
	case "house_occupied":
		return "Someone is home."
	default:
		return ""
	}
}

func shortDur(d time.Duration) string {
	switch {
	case d < 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
