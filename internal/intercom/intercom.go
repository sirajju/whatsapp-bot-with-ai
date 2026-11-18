// Package intercom bridges WhatsApp voice notes from an owner to a
// WiFi-connected speaker (the wall phone, via Tasker/MQTT).
//
// Flow on receipt of an owner's voice note:
//
//	whatsmeow event   → download audio bytes (opus/ogg)
//	                  → save to <cacheDir>/<uuid>.opus
//	                  → publish JSON to MQTT topic `intercom/play`:
//	                      {"url":"http://<host>:<port>/audio/<uuid>.opus",
//	                       "seconds":42, "sender":"..."}
//	                  → Tasker on the wall phone subscribes, downloads,
//	                    calls Media Play.
//	                  → background janitor deletes files older than TTL.
//
// Disabled by default. Enable with `INTERCOM_ENABLED=on` in the env, plus
// the MQTT broker config. If MQTT isn't reachable or disabled, the module
// starts no HTTP server and the hook is a no-op.
package intercom

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	defaultListen = "0.0.0.0:8766"
	// Nested path so we don't collide with any file someone might scp to
	// /tmp/<project>-* while staging a deployment.
	defaultCacheDir   = "/tmp/wabot/intercom"
	defaultTopic      = "intercom/play"
	defaultTTL        = 10 * time.Minute
	defaultMaxBytes   = 8 << 20 // 8 MB per clip; WhatsApp caps VN to ~16 MB
	defaultMQTTBroker = "tcp://127.0.0.1:1883"
)

// Config is everything the Bridge needs. Pass zero fields to accept defaults.
type Config struct {
	Enabled bool // master switch

	MQTTBroker   string // e.g. tcp://127.0.0.1:1883
	MQTTUser     string
	MQTTPassword string
	MQTTTopic    string // default "intercom/play"
	MQTTClientID string // default "wabot-intercom"

	HTTPListen string // host:port the audio server binds to
	PublicURL  string // what we tell the phone, e.g. http://192.168.1.230:8766
	CacheDir   string
	TTL        time.Duration
	MaxBytes   int

	// Only owners (by JID User-part) may trigger the intercom; wired from
	// cfg.Owners at startup.
	Owners []string
}

// Bridge is the live intercom service — HTTP server + MQTT publisher + cache.
type Bridge struct {
	cfg Config
	log zerolog.Logger

	mu    sync.Mutex
	files map[string]int64 // uuid → saved_at (unix) for TTL cleanup

	httpSrv *http.Server
	mqttCli mqtt.Client
	cancel  context.CancelFunc
}

// playMessage is the JSON we publish to the MQTT topic.
type playMessage struct {
	URL     string `json:"url"`
	Seconds int    `json:"seconds,omitempty"`
	Sender  string `json:"sender,omitempty"`
	ID      string `json:"id"`
	TS      int64  `json:"ts"`
}

// New builds a Bridge and starts its HTTP + MQTT. Returns a non-nil Bridge
// with Enabled=false if cfg.Enabled is false — Handle() then becomes a no-op
// so callers can wire it unconditionally.
func New(cfg Config, log zerolog.Logger) (*Bridge, error) {
	log = log.With().Str("component", "intercom").Logger()
	if !cfg.Enabled {
		log.Info().Msg("intercom disabled (INTERCOM_ENABLED!=on)")
		return &Bridge{cfg: cfg, log: log}, nil
	}

	// Apply defaults.
	if cfg.MQTTBroker == "" {
		cfg.MQTTBroker = defaultMQTTBroker
	}
	if cfg.MQTTTopic == "" {
		cfg.MQTTTopic = defaultTopic
	}
	if cfg.MQTTClientID == "" {
		cfg.MQTTClientID = "wabot-intercom"
	}
	if cfg.HTTPListen == "" {
		cfg.HTTPListen = defaultListen
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = defaultCacheDir
	}
	if cfg.TTL == 0 {
		cfg.TTL = defaultTTL
	}
	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = defaultMaxBytes
	}
	if cfg.PublicURL == "" {
		// Guess from HTTPListen — works for same-host clients only.
		cfg.PublicURL = "http://" + cfg.HTTPListen
	}
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")

	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("intercom: mkdir cache: %w", err)
	}

	b := &Bridge{
		cfg:   cfg,
		log:   log,
		files: make(map[string]int64),
	}

	// Connect MQTT (synchronous so a misconfig fails loudly at startup, not
	// silently at the first voice note).
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.MQTTBroker).
		SetClientID(cfg.MQTTClientID).
		SetAutoReconnect(true).
		SetConnectTimeout(5 * time.Second).
		SetKeepAlive(30 * time.Second)
	if cfg.MQTTUser != "" {
		opts.SetUsername(cfg.MQTTUser).SetPassword(cfg.MQTTPassword)
	}
	b.mqttCli = mqtt.NewClient(opts)
	if tok := b.mqttCli.Connect(); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		return nil, fmt.Errorf("intercom: mqtt connect: %v", tok.Error())
	}
	log.Info().Str("broker", cfg.MQTTBroker).Str("topic", cfg.MQTTTopic).Msg("mqtt connected")

	// HTTP audio server.
	mux := http.NewServeMux()
	mux.HandleFunc("/audio/", b.serveAudio)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok\n"))
	})
	b.httpSrv = &http.Server{
		Addr:              cfg.HTTPListen,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.HTTPListen)
	if err != nil {
		b.mqttCli.Disconnect(100)
		return nil, fmt.Errorf("intercom: listen %s: %w", cfg.HTTPListen, err)
	}
	go func() {
		if err := b.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			b.log.Error().Err(err).Msg("audio server exited")
		}
	}()
	log.Info().Str("listen", cfg.HTTPListen).Str("public", cfg.PublicURL).Msg("audio server up")

	// TTL janitor.
	jctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel
	go b.janitor(jctx)

	return b, nil
}

// Publish sends an MQTT message on the shared intercom client — exposed so
// other wabot modules (e.g. the CCTV command) can share the broker
// connection without opening a second client. No-op when the bridge is
// disabled or not connected.
func (b *Bridge) Publish(topic string, payload []byte) error {
	if b == nil || !b.cfg.Enabled || b.mqttCli == nil || !b.mqttCli.IsConnected() {
		return fmt.Errorf("intercom MQTT not available")
	}
	tok := b.mqttCli.Publish(topic, 1, false, payload)
	if !tok.WaitTimeout(3 * time.Second) {
		return fmt.Errorf("mqtt publish timed out")
	}
	return tok.Error()
}

// Stop cleans up HTTP + MQTT. Safe to call when New errored (no-op).
func (b *Bridge) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	if b.httpSrv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = b.httpSrv.Shutdown(shutdownCtx)
	}
	if b.mqttCli != nil && b.mqttCli.IsConnected() {
		b.mqttCli.Disconnect(200)
	}
}

// Handle is called from the router hook for every message. Returns true when
// it consumed the message — the caller should NOT dispatch further hooks /
// commands. False means "not for me, keep going".
//
// It triggers when:
//   - intercom is enabled,
//   - the message carries an audio attachment,
//   - sender is in the Owners list,
//   - no caption, or caption begins with "!say" / "!intercom".
//
// Group messages are ignored — the intercom is a 1:1 pipe to the wall phone.
func (b *Bridge) Handle(ctx context.Context, cli *whatsmeow.Client, evt *events.Message) bool {
	if b == nil || !b.cfg.Enabled {
		return false
	}
	a := evt.Message.GetAudioMessage()
	if a == nil {
		return false
	}
	if evt.Info.IsGroup {
		return false
	}
	if !b.isOwner(evt) {
		return false
	}
	// Allow a "!say" caption to be explicit; empty caption also triggers.
	if cap := extractCaption(evt.Message); cap != "" {
		t := strings.TrimSpace(strings.ToLower(cap))
		if !strings.HasPrefix(t, "!say") && !strings.HasPrefix(t, "!intercom") {
			return false
		}
	}

	go b.forward(ctx, cli, evt, a)
	return true
}

func (b *Bridge) isOwner(evt *events.Message) bool {
	candidates := []string{evt.Info.Sender.User}
	if alt := evt.Info.SenderAlt; !alt.IsEmpty() && alt.User != "" {
		candidates = append(candidates, alt.User)
	}
	for _, c := range candidates {
		for _, o := range b.cfg.Owners {
			if c == o {
				return true
			}
		}
	}
	return false
}

// forward downloads the audio, saves it, publishes the URL via MQTT, and
// reacts to the WhatsApp message so the owner sees it was picked up.
func (b *Bridge) forward(ctx context.Context, cli *whatsmeow.Client, evt *events.Message, a *waE2E.AudioMessage) {
	dlCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	raw, err := cli.Download(dlCtx, a)
	if err != nil {
		b.log.Error().Err(err).Msg("intercom: download failed")
		_ = react(ctx, cli, evt, "⚠️")
		return
	}
	if len(raw) > b.cfg.MaxBytes {
		b.log.Warn().Int("bytes", len(raw)).Int("limit", b.cfg.MaxBytes).Msg("intercom: clip too large")
		_ = react(ctx, cli, evt, "📏")
		return
	}

	id := newID()
	ext := ".opus"
	if mt := strings.ToLower(a.GetMimetype()); strings.Contains(mt, "mp4") {
		ext = ".m4a"
	}
	path := filepath.Join(b.cfg.CacheDir, id+ext)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		b.log.Error().Err(err).Msg("intercom: write cache failed")
		_ = react(ctx, cli, evt, "⚠️")
		return
	}
	b.mu.Lock()
	b.files[id+ext] = time.Now().Unix()
	b.mu.Unlock()

	// Prefer the phone-number JID for display. On modern WhatsApp
	// evt.Info.Sender.User is often a LID (looks like a random number);
	// the phone number lives in SenderAlt.User when the primary is a LID.
	senderDisplay := evt.Info.Sender.User
	if alt := evt.Info.SenderAlt; !alt.IsEmpty() && alt.User != "" {
		senderDisplay = alt.User
	}
	msg := playMessage{
		URL:     fmt.Sprintf("%s/audio/%s%s", b.cfg.PublicURL, id, ext),
		Seconds: int(a.GetSeconds()),
		Sender:  senderDisplay,
		ID:      id,
		TS:      time.Now().Unix(),
	}
	payload, _ := json.Marshal(msg)

	tok := b.mqttCli.Publish(b.cfg.MQTTTopic, 1, false, payload)
	if !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		b.log.Error().Err(tok.Error()).Msg("intercom: mqtt publish failed")
		_ = react(ctx, cli, evt, "⚠️")
		return
	}

	b.log.Info().Str("id", id).Int("bytes", len(raw)).Int("sec", msg.Seconds).Msg("intercom: forwarded")
	_ = react(ctx, cli, evt, "📣")
}

// serveAudio streams a cached clip. Only known UUIDs are served.
func (b *Bridge) serveAudio(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/audio/")
	if name == "" || strings.ContainsAny(name, "/\\") {
		http.NotFound(w, r)
		return
	}
	b.mu.Lock()
	_, known := b.files[name]
	b.mu.Unlock()
	if !known {
		http.NotFound(w, r)
		return
	}
	ct := "audio/ogg"
	if strings.HasSuffix(name, ".m4a") {
		ct = "audio/mp4"
	}
	w.Header().Set("Content-Type", ct)
	http.ServeFile(w, r, filepath.Join(b.cfg.CacheDir, name))
}

// janitor deletes cached files older than TTL. Runs every TTL/2.
func (b *Bridge) janitor(ctx context.Context) {
	tick := time.NewTicker(b.cfg.TTL / 2)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			cutoff := time.Now().Add(-b.cfg.TTL).Unix()
			b.mu.Lock()
			for name, saved := range b.files {
				if saved < cutoff {
					_ = os.Remove(filepath.Join(b.cfg.CacheDir, name))
					delete(b.files, name)
				}
			}
			b.mu.Unlock()
		}
	}
}

// --- helpers ---------------------------------------------------------------

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func extractCaption(m *waE2E.Message) string {
	if m == nil {
		return ""
	}
	if a := m.GetAudioMessage(); a != nil {
		// Audio messages have no caption field in WhatsApp; check extended text.
	}
	if ext := m.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	return m.GetConversation()
}

// react posts a one-emoji reaction to the triggering message so the owner
// sees acknowledgement without a reply. Non-fatal if it fails.
func react(ctx context.Context, cli *whatsmeow.Client, evt *events.Message, emoji string) error {
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := cli.SendMessage(rctx, evt.Info.Chat, cli.BuildReaction(evt.Info.Chat, evt.Info.Sender, evt.Info.ID, emoji))
	return err
}
