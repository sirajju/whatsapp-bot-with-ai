package commands

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/aitools"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Status is the user-facing system info. Mirrors what aitools.get_system_info returns
// but formatted for a chat message. Owner-only.
func Status(start time.Time) router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		host := aitools.SystemInfoText()
		var b strings.Builder
		fmt.Fprintf(&b, "🖥 *wabot status*\n")
		fmt.Fprintf(&b, "uptime: %s\n", time.Since(start).Round(time.Second))
		fmt.Fprintf(&b, "go runtime: %s (%d CPUs)\n", runtime.Version(), runtime.NumCPU())
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		fmt.Fprintf(&b, "bot RSS: %.1f MB (sys %.1f MB)\n", float64(m.Alloc)/1024/1024, float64(m.Sys)/1024/1024)
		b.WriteString("\n```\n")
		b.WriteString(host)
		b.WriteString("\n```")
		return sendText(ctx, env.Cli, env.Chat, b.String())
	}
}

// Sh runs a bash command with full privileges of the bot's user. OWNER ONLY.
// This is deliberately powerful — use with care.
func Sh() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		if len(env.Args) == 0 {
			return sendText(ctx, env.Cli, env.Chat, "usage: "+env.Cfg.Prefix+"sh <command>")
		}
		cmd := strings.Join(env.Args, " ")
		_ = sendText(ctx, env.Cli, env.Chat, "⚙️ "+cmd)

		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		c := exec.CommandContext(runCtx, "bash", "-c", cmd)
		var out bytes.Buffer
		c.Stdout = &out
		c.Stderr = &out
		err := c.Run()

		result := out.String()
		if len(result) > 3500 {
			result = result[:3500] + "\n…(truncated)"
		}
		if strings.TrimSpace(result) == "" {
			result = "(no output)"
		}
		prefix := "✅"
		if err != nil {
			prefix = fmt.Sprintf("❌ %s", err.Error())
		}
		return sendText(ctx, env.Cli, env.Chat,
			fmt.Sprintf("%s\n```\n%s\n```", prefix, result))
	}
}
