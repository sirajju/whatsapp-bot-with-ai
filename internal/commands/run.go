package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/router"
)

// Run executes a script from ./scripts/<name> and returns its stdout/stderr tail.
// Owner only. The scripts directory is relative to the bot's working directory.
// Simple + direct: no AI mediation (proper tool-calling via provider function APIs is a
// separate, bigger feature).
func Run() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if !env.IsOwner {
			return sendText(ctx, env.Cli, env.Chat, "owner only")
		}
		if len(env.Args) == 0 {
			return listScripts(ctx, env)
		}

		name := env.Args[0]
		if strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
			return sendText(ctx, env.Cli, env.Chat, "script name must be a bare filename")
		}
		path := filepath.Join("scripts", name)
		if _, err := os.Stat(path); err != nil {
			return sendText(ctx, env.Cli, env.Chat, "no such script: "+name)
		}

		_ = sendText(ctx, env.Cli, env.Chat, "⚙️ running "+name+"…")

		runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(runCtx, "./"+path, env.Args[1:]...)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		cmd.Env = os.Environ()
		err := cmd.Run()

		result := strings.TrimSpace(out.String())
		if len(result) > 3500 {
			result = result[:3500] + "\n…(truncated)"
		}
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat,
				fmt.Sprintf("❌ %s failed: %s\n```\n%s\n```", name, err.Error(), result))
		}
		if result == "" {
			result = "(no output)"
		}
		return sendText(ctx, env.Cli, env.Chat,
			fmt.Sprintf("✅ %s\n```\n%s\n```", name, result))
	}
}

func listScripts(ctx context.Context, env *router.Env) error {
	entries, err := os.ReadDir("scripts")
	if err != nil {
		if os.IsNotExist(err) {
			return sendText(ctx, env.Cli, env.Chat,
				"no scripts dir yet. On the server: `mkdir ~/whatsapp-bot-with-ai/scripts` then drop executable scripts in there.")
		}
		return sendText(ctx, env.Cli, env.Chat, "scripts: "+err.Error())
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return sendText(ctx, env.Cli, env.Chat, "no scripts in ./scripts/")
	}
	var b strings.Builder
	b.WriteString("Available scripts (owner only):\n")
	for _, n := range names {
		fmt.Fprintf(&b, "• %srun %s\n", env.Cfg.Prefix, n)
	}
	return sendText(ctx, env.Cli, env.Chat, b.String())
}
