package aitools

import (
	"fmt"
	"strings"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
)

func addTodoTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "add_todo",
			Description: "Add a new todo item for the current user. Returns confirmation with the new item's text.",
			Parameters: objectSchema(map[string]any{
				"text": stringProp("The todo text — what the user wants to remember to do."),
			}, "text"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			text, err := requireString(args, "text")
			if err != nil {
				return "", err
			}
			if _, err := env.Store.AddTodo(env.Ctx, env.SenderJID, text); err != nil {
				return "", err
			}
			return "added todo: " + text, nil
		},
	}
}

func listTodosTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "list_todos",
			Description: "List the current user's pending todos. Pass include_done=true to also see completed ones.",
			Parameters: objectSchema(map[string]any{
				"include_done": boolProp("If true, include already-completed todos."),
			}),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			includeDone := false
			if v, ok := args["include_done"].(bool); ok {
				includeDone = v
			}
			todos, err := env.Store.ListTodos(env.Ctx, env.SenderJID, includeDone)
			if err != nil {
				return "", err
			}
			if len(todos) == 0 {
				return "No todos.", nil
			}
			var b strings.Builder
			b.WriteString("User's todos:\n")
			for i, t := range todos {
				status := "[ ]"
				if t.Done {
					status = "[x]"
				}
				fmt.Fprintf(&b, "%d. %s %s\n", i+1, status, t.Text)
			}
			return strings.TrimSpace(b.String()), nil
		},
	}
}

func completeTodoTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "complete_todo",
			Description: "Mark a pending todo as done, by its 1-indexed position in the current pending list.",
			Parameters: objectSchema(map[string]any{
				"index": intProp("1-based index from list_todos."),
			}, "index"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			idx := optInt(args, "index")
			if idx < 1 {
				return "", fmt.Errorf("index must be >= 1")
			}
			text, err := env.Store.MarkTodoDone(env.Ctx, env.SenderJID, idx)
			if err != nil {
				return "", err
			}
			return "marked done: " + text, nil
		},
	}
}
