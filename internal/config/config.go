// Package config loads runtime settings from the environment (or a .env file).
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Prefix     string
	Owners     []string
	LoginMode  string
	PairPhone  string
	SessionDir string
	LogLevel   string

	RespondInGroups     bool
	GroupTriggerMention bool

	AIPreferred     string
	OllamaURL       string
	OllamaModel     string
	GroqKey         string
	GroqModel       string
	AnthropicKey    string
	AnthropicMdl    string
	OpenAIKey       string
	OpenAIModel     string
	GeminiKey       string
	GeminiModel     string
	NvidiaKey       string
	NvidiaModel     string
	OpenRouterKey   string
	OpenRouterModel string
	OpencodeArgv    []string

	ElevenLabsKey     string
	ElevenLabsVoiceID string

	// Intercom: WhatsApp voice note from owner → MQTT → wall-phone speaker.
	IntercomEnabled      bool
	IntercomMQTTBroker   string
	IntercomMQTTUser     string
	IntercomMQTTPassword string
	IntercomMQTTTopic    string
	IntercomHTTPListen   string
	IntercomPublicURL    string
	IntercomCacheDir     string

	// CCTV: HTTP snapshot URL (e.g. IP Webcam). /cctv fetches it and posts
	// the JPEG as an image message back to the chat. When CCTVControlTopic
	// is set, wabot asks the wall phone (via MQTT) to START IP Webcam just
	// before fetching and STOP it right after, so the camera isn't running
	// idle in the background.
	CCTVURL           string
	CCTVUser          string
	CCTVPassword      string
	CCTVTimeout       time.Duration
	CCTVControlTopic  string
	CCTVStartTimeout  time.Duration
	CCTVAutoStop      bool
	// Live-stream support for !cctv on [seconds] / !cctv off.
	// CCTVStreamURL is the MJPEG endpoint (default: CCTV_URL with "/shot.jpg"
	// swapped for "/video"). CCTVStreamURLExt is an optional second URL for
	// off-LAN access (typically the phone's Tailscale IP).
	CCTVStreamURL      string
	CCTVStreamURLExt   string
	CCTVLiveDefault    time.Duration
	CCTVLiveMax        time.Duration

	// Startup "I'm up" DM to the first OWNER when wabot authenticates with
	// WhatsApp for the first time this process lifetime.
	StartupNotify bool

	// System health monitor — probes /proc/meminfo, / disk, /proc/loadavg,
	// /sys/class/thermal/* on an interval; alerts the first OWNER when a
	// threshold is crossed.
	MonitorEnabled  bool
	MonitorInterval time.Duration
	MonitorMemPct   int
	MonitorDiskPct  int
	MonitorLoadMult float64
	MonitorTempC    float64
	MonitorCooldown time.Duration
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	c := &Config{
		Prefix:              getenv("PREFIX", "!"),
		Owners:              splitCSV(os.Getenv("OWNERS")),
		LoginMode:           strings.ToLower(getenv("LOGIN_MODE", "pairing")),
		PairPhone:           strings.TrimSpace(os.Getenv("PAIR_PHONE")),
		SessionDir:          getenv("SESSION_DIR", "./session"),
		LogLevel:            getenv("LOG_LEVEL", "info"),
		RespondInGroups:     getenv("RESPOND_IN_GROUPS", "true") == "true",
		GroupTriggerMention: getenv("GROUP_TRIGGER_MENTION_ONLY", "false") == "true",

		AIPreferred:     strings.ToLower(os.Getenv("AI_PREFERRED")),
		OllamaURL:       os.Getenv("OLLAMA_URL"),
		OllamaModel:     getenv("OLLAMA_MODEL", "llama3.2"),
		GroqKey:         os.Getenv("GROQ_API_KEY"),
		GroqModel:       getenv("GROQ_MODEL", "openai/gpt-oss-20b"),
		AnthropicKey:    os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicMdl:    getenv("ANTHROPIC_MODEL", "claude-haiku-5-5"),
		OpenAIKey:       os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:     getenv("OPENAI_MODEL", "gpt-4o-mini"),
		GeminiKey:       os.Getenv("GEMINI_API_KEY"),
		GeminiModel:     getenv("GEMINI_MODEL", "gemini-2.0-flash"),
		NvidiaKey:       os.Getenv("NVIDIA_API_KEY"),
		NvidiaModel:     getenv("NVIDIA_MODEL", "meta/llama-3.3-70b-instruct"),
		OpenRouterKey:   os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel: getenv("OPENROUTER_MODEL", "meta-llama/llama-3.3-70b-instruct:free"),
		OpencodeArgv:    parseOpencodeArgv(),

		ElevenLabsKey:     os.Getenv("ELEVENLABS_API_KEY"),
		ElevenLabsVoiceID: os.Getenv("ELEVENLABS_VOICE_ID"),

		IntercomEnabled:      strings.ToLower(os.Getenv("INTERCOM_ENABLED")) == "on",
		IntercomMQTTBroker:   getenv("INTERCOM_MQTT_BROKER", "tcp://127.0.0.1:1883"),
		IntercomMQTTUser:     os.Getenv("INTERCOM_MQTT_USER"),
		IntercomMQTTPassword: os.Getenv("INTERCOM_MQTT_PASSWORD"),
		IntercomMQTTTopic:    getenv("INTERCOM_MQTT_TOPIC", "intercom/play"),
		IntercomHTTPListen:   getenv("INTERCOM_HTTP_LISTEN", "0.0.0.0:8766"),
		IntercomPublicURL:    os.Getenv("INTERCOM_PUBLIC_URL"),
		IntercomCacheDir:     getenv("INTERCOM_CACHE_DIR", "/tmp/wabot-intercom"),

		CCTVURL:          strings.TrimSpace(os.Getenv("CCTV_URL")),
		CCTVUser:         os.Getenv("CCTV_USER"),
		CCTVPassword:     os.Getenv("CCTV_PASSWORD"),
		CCTVTimeout:      parseDurationOr(os.Getenv("CCTV_TIMEOUT"), 15*time.Second),
		CCTVControlTopic: strings.TrimSpace(os.Getenv("CCTV_CONTROL_TOPIC")),
		CCTVStartTimeout: parseDurationOr(os.Getenv("CCTV_START_TIMEOUT"), 20*time.Second),
		CCTVAutoStop:     strings.ToLower(os.Getenv("CCTV_AUTO_STOP")) != "off",

		CCTVStreamURL:    strings.TrimSpace(os.Getenv("CCTV_STREAM_URL")),
		CCTVStreamURLExt: strings.TrimSpace(os.Getenv("CCTV_STREAM_URL_EXT")),
		CCTVLiveDefault:  parseDurationOr(os.Getenv("CCTV_LIVE_DEFAULT"), 120*time.Second),
		CCTVLiveMax:      parseDurationOr(os.Getenv("CCTV_LIVE_MAX"), 600*time.Second),

		StartupNotify: strings.ToLower(os.Getenv("STARTUP_NOTIFY")) != "off",

		MonitorEnabled:  strings.ToLower(os.Getenv("MONITOR_ENABLED")) == "on",
		MonitorInterval: parseDurationOr(os.Getenv("MONITOR_INTERVAL"), 60*time.Second),
		MonitorMemPct:   parseIntOr(os.Getenv("MONITOR_MEM_PCT"), 15),
		MonitorDiskPct:  parseIntOr(os.Getenv("MONITOR_DISK_PCT"), 10),
		MonitorLoadMult: parseFloatOr(os.Getenv("MONITOR_LOAD_MULT"), 2.0),
		MonitorTempC:    parseFloatOr(os.Getenv("MONITOR_TEMP_C"), 75.0),
		MonitorCooldown: parseDurationOr(os.Getenv("MONITOR_COOLDOWN"), 30*time.Minute),
	}

	if c.LoginMode == "pairing" && c.PairPhone == "" {
		return nil, errors.New("LOGIN_MODE=pairing requires PAIR_PHONE (digits only, country code included)")
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func parseDurationOr(s string, def time.Duration) time.Duration {
	if s = strings.TrimSpace(s); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
	}
	return def
}

func parseIntOr(s string, def int) int {
	if s = strings.TrimSpace(s); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return def
}

func parseFloatOr(s string, def float64) float64 {
	if s = strings.TrimSpace(s); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return def
}

// parseOpencodeArgv returns the comma-separated OPENCODE_CMD (default "opencode,run")
// only when OPENCODE_ENABLED=true. Empty slice disables the provider.
func parseOpencodeArgv() []string {
	if strings.ToLower(os.Getenv("OPENCODE_ENABLED")) != "true" {
		return nil
	}
	raw := getenv("OPENCODE_CMD", "opencode,run")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
