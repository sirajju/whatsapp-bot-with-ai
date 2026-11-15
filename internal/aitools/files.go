package aitools

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
)

// Files tools are sandboxed per-user to ./ai-sandbox/<hash(sender)>/.
// Hard limits:
//   - no path separators in name
//   - filename extension must match an allowlist
//   - max file size: 64 KiB
//   - max 50 files per user

const (
	maxFileBytes  = 64 * 1024
	maxUserFiles  = 50
	sandboxRoot   = "ai-sandbox"
)

var allowedExt = map[string]bool{
	"":     true, // no extension
	".txt": true, ".md": true, ".json": true, ".yaml": true, ".yml": true,
	".csv": true, ".log": true, ".ini": true, ".conf": true,
	".py":  true, ".sh": true, ".js": true, ".ts": true, ".go": true, ".html": true, ".css": true,
}

func userDir(senderJID string) string {
	h := sha1.Sum([]byte(senderJID))
	return filepath.Join(sandboxRoot, fmt.Sprintf("%x", h[:8]))
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("empty filename")
	}
	if strings.ContainsAny(name, "/\\") || name == "." || name == ".." || strings.Contains(name, "..") {
		return fmt.Errorf("bad filename: no slashes or ..")
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedExt[ext] {
		return fmt.Errorf("extension %q not allowed", ext)
	}
	return nil
}

func fileCreateTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "file_create",
			Description: "Create or overwrite a file in the user's personal sandbox directory. " +
				"Files are limited to 64 KiB and must have a plain-text extension (txt, md, json, yaml, csv, log, py, sh, js, ts, go, html, css). " +
				"No directories — the filename must be flat.",
			Parameters: objectSchema(map[string]any{
				"name":    stringProp("Flat filename, no path separators. Must have an allowed extension."),
				"content": stringProp("The file contents. UTF-8 text."),
			}, "name", "content"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			name, err := requireString(args, "name")
			if err != nil {
				return "", err
			}
			content, err := requireString(args, "content")
			if err != nil {
				return "", err
			}
			if err := validateName(name); err != nil {
				return "", err
			}
			if len(content) > maxFileBytes {
				return "", fmt.Errorf("content exceeds %d bytes", maxFileBytes)
			}
			dir := userDir(env.SenderJID)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
			// enforce per-user file count limit on NEW files
			if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
				entries, _ := os.ReadDir(dir)
				if len(entries) >= maxUserFiles {
					return "", fmt.Errorf("sandbox full (%d files); delete some first", maxUserFiles)
				}
			}
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return "", err
			}
			return fmt.Sprintf("wrote %s (%d bytes)", name, len(content)), nil
		},
	}
}

func fileReadTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "file_read",
			Description: "Read a file previously created in the user's personal sandbox. Returns its contents, truncated to 32 KiB.",
			Parameters: objectSchema(map[string]any{
				"name": stringProp("Flat filename to read."),
			}, "name"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			name, err := requireString(args, "name")
			if err != nil {
				return "", err
			}
			if err := validateName(name); err != nil {
				return "", err
			}
			path := filepath.Join(userDir(env.SenderJID), name)
			data, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			if len(data) > 32*1024 {
				data = append(data[:32*1024], []byte("\n…(truncated)")...)
			}
			return string(data), nil
		},
	}
}

func fileListTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "file_list",
			Description: "List files in the user's personal sandbox with sizes.",
			Parameters:  objectSchema(map[string]any{}),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			dir := userDir(env.SenderJID)
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					return "No files yet.", nil
				}
				return "", err
			}
			var names []string
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				info, err := e.Info()
				if err != nil {
					continue
				}
				names = append(names, fmt.Sprintf("%s (%d bytes)", e.Name(), info.Size()))
			}
			sort.Strings(names)
			if len(names) == 0 {
				return "No files yet.", nil
			}
			return "Files:\n" + strings.Join(names, "\n"), nil
		},
	}
}
