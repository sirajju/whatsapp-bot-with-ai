// Package config loads runtime settings from the environment (or a .env file).
package config

import (
	"errors"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Prefix     string
	Owners     []string
	LoginMode  string // "pairing" or "qr"
	PairPhone  string
	SessionDir string
	LogLevel   string

	RespondInGroups      bool
	GroupTriggerMention  bool

	OllamaURL    string
	OllamaModel  string
	GroqKey      string
	GroqModel    string
	AnthropicKey string
	AnthropicMdl string
	OpenAIKey    string
	OpenAIModel  string
	GeminiKey    string
	GeminiModel  string
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

		OllamaURL:    os.Getenv("OLLAMA_URL"),
		OllamaModel:  getenv("OLLAMA_MODEL", "llama3.2"),
		GroqKey:      os.Getenv("GROQ_API_KEY"),
		GroqModel:    getenv("GROQ_MODEL", "llama-3.3-70b-versatile"),
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicMdl: getenv("ANTHROPIC_MODEL", "claude-haiku-5-5"),
		OpenAIKey:    os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:  getenv("OPENAI_MODEL", "gpt-4o-mini"),
		GeminiKey:    os.Getenv("GEMINI_API_KEY"),
		GeminiModel:  getenv("GEMINI_MODEL", "gemini-2.0-flash"),
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
