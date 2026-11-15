package aitools

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
)

func getSystemInfoTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "get_system_info",
			Description: "Returns the host system's current RAM, swap, disk usage, and uptime. No arguments.",
			Parameters:  objectSchema(map[string]any{}),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			return SystemInfoText(), nil
		},
	}
}

func getCurrentTimeTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "get_current_time",
			Description: "Returns the current local server time in RFC3339 format. No arguments.",
			Parameters:  objectSchema(map[string]any{}),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			return time.Now().Format(time.RFC3339), nil
		},
	}
}

// SystemInfoText is public so the /status command can reuse it.
func SystemInfoText() string {
	var b strings.Builder
	b.WriteString("System info:\n")
	b.WriteString(run("uptime"))
	b.WriteString("\n--- memory ---\n")
	b.WriteString(run("free", "-h"))
	b.WriteString("\n--- disk ---\n")
	b.WriteString(run("df", "-h", "/"))
	return strings.TrimSpace(b.String())
}

func run(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("(%s failed: %s)", name, err.Error())
	}
	return strings.TrimSpace(out.String())
}
