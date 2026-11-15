package ai

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Opencode shells out to the `opencode` CLI the user has installed locally.
// Enable by setting OPENCODE_ENABLED=true in .env; tune the invocation with
// OPENCODE_CMD (comma-separated argv). Default: "opencode,run".
type Opencode struct {
	Argv []string
}

func (o *Opencode) Name() string { return "opencode" }

func (o *Opencode) Chat(ctx context.Context, system string, history []Msg, prompt string) (string, error) {
	if len(o.Argv) == 0 {
		return "", fmt.Errorf("opencode: empty argv (set OPENCODE_CMD)")
	}
	if _, err := exec.LookPath(o.Argv[0]); err != nil {
		return "", fmt.Errorf("opencode: %q not on PATH", o.Argv[0])
	}

	// Prefix the prompt with system + history as a flat text block since the
	// CLI doesn't take structured messages.
	var full strings.Builder
	if system != "" {
		full.WriteString("System: ")
		full.WriteString(system)
		full.WriteString("\n\n")
	}
	for _, m := range history {
		full.WriteString(strings.Title(m.Role))
		full.WriteString(": ")
		full.WriteString(m.Content)
		full.WriteString("\n")
	}
	full.WriteString("User: ")
	full.WriteString(prompt)

	argv := append([]string(nil), o.Argv...)
	argv = append(argv, full.String())

	runCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("opencode: %w: %s", err, strings.TrimSpace(errb.String()))
	}
	reply := strings.TrimSpace(out.String())
	if reply == "" {
		return "", fmt.Errorf("opencode: empty stdout; stderr: %s", strings.TrimSpace(errb.String()))
	}
	return reply, nil
}
