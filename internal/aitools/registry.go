// Package aitools defines the tool registry the LLM can call via function calling.
package aitools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/config"
	"github.com/sirajju/whatsapp-bot-with-ai/internal/store"
)

// ToolEnv is the execution context handed to a tool handler.
type ToolEnv struct {
	Ctx       context.Context
	Store     *store.Store
	Cfg       *config.Config
	SenderJID string
	ChatJID   string
	Log       zerolog.Logger
}

// Handler is a tool's implementation. It returns a short string the model will read.
type Handler func(env *ToolEnv, args map[string]any) (string, error)

// Tool bundles a spec with its handler.
type Tool struct {
	Spec    ai.ToolSpec
	Handler Handler
}

// Registry owns the set of callable tools.
type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry { return &Registry{tools: make(map[string]Tool)} }

func (r *Registry) Register(t Tool) { r.tools[t.Spec.Name] = t }

func (r *Registry) Specs() []ai.ToolSpec {
	out := make([]ai.ToolSpec, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Spec)
	}
	return out
}

// Invoke decodes args from the model's JSON string and runs the handler.
// Any error is returned as a text string the model can read back — tool failures
// shouldn't crash the conversation.
func (r *Registry) Invoke(env *ToolEnv, name, argsJSON string) string {
	t, ok := r.tools[name]
	if !ok {
		return "error: unknown tool " + name
	}
	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "error: bad arguments JSON: " + err.Error()
		}
	}
	env.Log.Info().Str("tool", name).Msg("ai tool call")
	out, err := t.Handler(env, args)
	if err != nil {
		return "error: " + err.Error()
	}
	return out
}

// RegisterAll wires every built-in tool. Call once at startup.
func (r *Registry) RegisterAll() {
	r.Register(getSystemInfoTool())
	r.Register(getCurrentTimeTool())
	r.Register(addTodoTool())
	r.Register(listTodosTool())
	r.Register(completeTodoTool())
	r.Register(addReminderTool())
	r.Register(listRemindersTool())
	r.Register(getWeatherTool())
	r.Register(translateTool())
	r.Register(webSearchTool())
	r.Register(fileCreateTool())
	r.Register(fileReadTool())
	r.Register(fileListTool())
	// Expense tracker — one tool per operation, backed by skills/expense
	r.Register(expenseAddTool())
	r.Register(expensePayTool())
	r.Register(expenseSetMembersTool())
	r.Register(expenseSetSplitTool())
	r.Register(expenseTotalTool())
	r.Register(expenseListTool())
	r.Register(expenseDelTool())
	r.Register(expenseSettleTool())
	r.Register(expenseListCreditTool())
	r.Register(expenseAddBulkTool())
}

// -------- helpers for building a JSON Schema in Go --------

func objectSchema(properties map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func stringProp(desc string) map[string]any    { return map[string]any{"type": "string", "description": desc} }
func intProp(desc string) map[string]any       { return map[string]any{"type": "integer", "description": desc} }
func boolProp(desc string) map[string]any      { return map[string]any{"type": "boolean", "description": desc} }

// optString returns the first non-nil string value from args[k], or "" if missing.
func optString(args map[string]any, k string) string {
	if v, ok := args[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func optInt(args map[string]any, k string) int {
	if v, ok := args[k]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

// requireString errors if a required arg is missing or empty.
func requireString(args map[string]any, k string) (string, error) {
	s := optString(args, k)
	if s == "" {
		return "", fmt.Errorf("missing required arg %q", k)
	}
	return s, nil
}
