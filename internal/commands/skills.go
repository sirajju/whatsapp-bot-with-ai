package commands

import (
	"bufio"
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

// Skill runs a script in ./skills/ or lists available ones.
// Unlike /run (which is owner-only), /skill is open to everyone.
// A skill is an executable file in ./skills/ whose second-or-later line is a
// comment of the form   # desc: <one-line description>
// That description is shown by /skills.
func Skill() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		if len(env.Args) == 0 {
			return listSkills(ctx, env)
		}
		name := env.Args[0]
		if strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
			return sendText(ctx, env.Cli, env.Chat, "skill name must be a bare filename")
		}
		path := filepath.Join("skills", name)
		info, err := os.Stat(path)
		if err != nil {
			return sendText(ctx, env.Cli, env.Chat, "no such skill: "+name)
		}
		if info.Mode()&0o111 == 0 {
			return sendText(ctx, env.Cli, env.Chat,
				"skill "+name+" is not executable. On the server: chmod +x skills/"+name)
		}
		_ = sendText(ctx, env.Cli, env.Chat, "⚙️ running "+name+"…")

		runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(runCtx, "./"+path, env.Args[1:]...)
		cmd.Env = append(os.Environ(),
			"WABOT_SENDER="+env.Sender.String(),
			"WABOT_CHAT="+env.Chat.String(),
			"WABOT_IS_OWNER="+boolStr(env.IsOwner),
			"WABOT_IS_GROUP="+boolStr(env.IsGroup),
		)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err = cmd.Run()

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

// Skills lists all skills with their descriptions.
func Skills() router.Handler {
	return func(ctx context.Context, env *router.Env) error {
		return listSkills(ctx, env)
	}
}

func listSkills(ctx context.Context, env *router.Env) error {
	entries, err := os.ReadDir("skills")
	if err != nil {
		if os.IsNotExist(err) {
			return sendText(ctx, env.Cli, env.Chat,
				"no skills dir yet. On the server:\n  mkdir ~/whatsapp-bot-with-ai/skills\nThen drop executable scripts there. "+
					"Add a '# desc: <text>' line near the top so "+env.Cfg.Prefix+"skills can describe them.")
		}
		return sendText(ctx, env.Cli, env.Chat, "skills: "+err.Error())
	}
	type entry struct{ name, desc string }
	var list []entry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Mode()&0o111 == 0 {
			continue
		}
		list = append(list, entry{name: e.Name(), desc: readSkillDesc(filepath.Join("skills", e.Name()))})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].name < list[j].name })
	if len(list) == 0 {
		return sendText(ctx, env.Cli, env.Chat,
			"no executable skills in ./skills/. Drop one in with a shebang and chmod +x.")
	}
	var b strings.Builder
	b.WriteString("🧠 *Skills*\n")
	for _, e := range list {
		desc := e.desc
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Fprintf(&b, "\n• %s%s %s — %s", env.Cfg.Prefix, "skill ", e.name, desc)
	}
	b.WriteString("\n\nrun: " + env.Cfg.Prefix + "skill <name> [args]")
	return sendText(ctx, env.Cli, env.Chat, b.String())
}

// readSkillDesc reads the first `# desc: <text>` line from the script (first 2 KiB).
func readSkillDesc(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 2048), 2048)
	for i := 0; sc.Scan() && i < 40; i++ {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "# desc:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# desc:"))
		}
		if strings.HasPrefix(line, "#desc:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "#desc:"))
		}
	}
	return ""
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
