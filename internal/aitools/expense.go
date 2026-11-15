package aitools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"github.com/sirajju/whatsapp-bot-with-ai/internal/ai"
)

// All expense tools shell out to the Python skill at ./skills/expense so
// there's one source of truth for the math and schema. The ChatJID is the
// per-chat namespace key.

const expenseSkillPath = "./skills/expense"

func runExpense(env *ToolEnv, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(env.Ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, expenseSkillPath, args...)
	cmd.Env = append(os.Environ(),
		"WABOT_CHAT="+env.ChatJID,
		"WABOT_SENDER="+env.SenderJID,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	s := strings.TrimSpace(out.String())
	if err != nil {
		if s == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", s)
	}
	return s, nil
}

func numberProp(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

func arrayStringProp(desc string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": desc,
		"items":       map[string]any{"type": "string"},
	}
}

func optFloat(args map[string]any, k string) (float64, bool) {
	v, ok := args[k]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

func requireFloat(args map[string]any, k string) (float64, error) {
	f, ok := optFloat(args, k)
	if !ok {
		return 0, fmt.Errorf("missing required numeric arg %q", k)
	}
	return f, nil
}

func optStringSlice(args map[string]any, k string) []string {
	v, ok := args[k]
	if !ok || v == nil {
		return nil
	}
	a, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(a))
	for _, x := range a {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func fmtAmount(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}

// -------- tools --------

func expenseAddTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "expense_add",
			Description: "Record a shared expense for the current chat. The expense has a name " +
				"(what was bought, e.g. 'veg', 'pizza'), an amount, and optionally who paid " +
				"for it. If paid_by is omitted the expense is in the pool but unassigned. " +
				"Use expense_set_members first to tell the tracker who shares the costs.",
			Parameters: objectSchema(map[string]any{
				"name":    stringProp("The item the money was spent on (e.g. 'veg', 'pizza')."),
				"amount":  numberProp("Cost of the item in the chat's local currency (positive number)."),
				"paid_by": stringProp("Optional: which member paid (must be in the members list)."),
				"note":    stringProp("Optional free-text note."),
			}, "name", "amount"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			name, err := requireString(args, "name")
			if err != nil {
				return "", err
			}
			amount, err := requireFloat(args, "amount")
			if err != nil {
				return "", err
			}
			cli := []string{"add", name, fmtAmount(amount)}
			if p := optString(args, "paid_by"); p != "" {
				cli = append(cli, "("+p+")")
			}
			if n := optString(args, "note"); n != "" {
				cli = append(cli, n)
			}
			return runExpense(env, cli...)
		},
	}
}

func expensePayTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "expense_pay",
			Description: "Record a direct one-to-one settlement between two members " +
				"(not a shared expense): 'from' hands 'to' this amount.",
			Parameters: objectSchema(map[string]any{
				"from":   stringProp("Member who paid."),
				"to":     stringProp("Member who received."),
				"amount": numberProp("Amount paid."),
			}, "from", "to", "amount"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			from, err := requireString(args, "from")
			if err != nil {
				return "", err
			}
			to, err := requireString(args, "to")
			if err != nil {
				return "", err
			}
			amount, err := requireFloat(args, "amount")
			if err != nil {
				return "", err
			}
			return runExpense(env, "pay", from, fmtAmount(amount), to)
		},
	}
}

func expenseSetMembersTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "expense_set_members",
			Description: "Replace the chat's split-participant list with these names. " +
				"The per-person share of the shared pool is computed as pool/len(members).",
			Parameters: objectSchema(map[string]any{
				"members": arrayStringProp("Member names (lowercase, letters/digits/._-, no spaces)."),
			}, "members"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			members := optStringSlice(args, "members")
			if len(members) == 0 {
				return "", fmt.Errorf("members must be a non-empty array of names")
			}
			return runExpense(env, append([]string{"members"}, members...)...)
		},
	}
}

func expenseSetSplitTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "expense_set_split",
			Description: "Fix the split group size (overrides member count). Pass 0 to use " +
				"the number of configured members automatically.",
			Parameters: objectSchema(map[string]any{
				"n": intProp("Group size for splitting (0 = auto)."),
			}, "n"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			n := optInt(args, "n")
			return runExpense(env, "split", strconv.Itoa(n))
		},
	}
}

func expenseTotalTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "expense_total",
			Description: "Report the per-person net balance for the current chat. " +
				"Positive = the group owes that person; negative = they owe the group.",
			Parameters: objectSchema(map[string]any{}),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			return runExpense(env, "total")
		},
	}
}

func expenseListTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "expense_list",
			Description: "List the last N expense entries in the current chat (default 10).",
			Parameters: objectSchema(map[string]any{
				"limit": intProp("How many entries to list (1-50)."),
			}),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			n := optInt(args, "limit")
			if n <= 0 {
				return runExpense(env, "list")
			}
			return runExpense(env, "list", strconv.Itoa(n))
		},
	}
}

func expenseDelTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name:        "expense_delete",
			Description: "Delete one expense entry by its numeric id (from expense_list).",
			Parameters: objectSchema(map[string]any{
				"id": intProp("Entry id to remove."),
			}, "id"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			id := optInt(args, "id")
			if id <= 0 {
				return "", fmt.Errorf("id must be > 0")
			}
			return runExpense(env, "del", strconv.Itoa(id))
		},
	}
}

func expenseSettleTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "expense_settle",
			Description: "Mark a credit (unpaid) expense as paid by a specific member. " +
				"Use when someone settles the shop tab or month-end bill. " +
				"Only works on entries that currently have no paid_by set.",
			Parameters: objectSchema(map[string]any{
				"id":      intProp("Entry id (from expense_list_credit or expense_list)."),
				"paid_by": stringProp("Member who paid it off."),
			}, "id", "paid_by"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			id := optInt(args, "id")
			if id <= 0 {
				return "", fmt.Errorf("id must be > 0")
			}
			payer, err := requireString(args, "paid_by")
			if err != nil {
				return "", err
			}
			return runExpense(env, "settle", strconv.Itoa(id), payer)
		},
	}
}

func expenseListCreditTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "expense_list_credit",
			Description: "List every unpaid (credit) expense in the current chat with id, name, " +
				"amount and total open. These are shared expenses with no payer assigned yet — " +
				"typically a running shop tab to settle at month-end.",
			Parameters: objectSchema(map[string]any{}),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			return runExpense(env, "credit")
		},
	}
}

func expenseSyncGroupTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "expense_sync_group_description",
			Description: "Overwrite the current WhatsApp group's description with the latest " +
				"expense totals (balance summary and credit). Group chats only. Use when the user " +
				"asks to 'post' or 'pin' or 'show in group info' the current balances.",
			Parameters: objectSchema(map[string]any{}),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			if env.Cli == nil {
				return "", fmt.Errorf("whatsmeow client not available")
			}
			jid, err := types.ParseJID(env.ChatJID)
			if err != nil {
				return "", fmt.Errorf("bad chat jid: %w", err)
			}
			if jid.Server != types.GroupServer {
				return "", fmt.Errorf("sync only works in group chats")
			}
			total, err := runExpense(env, "total")
			if err != nil {
				return "", err
			}
			desc := total
			if len(desc) > 1500 {
				desc = desc[:1500] + "…"
			}
			if err := env.Cli.SetGroupDescription(env.Ctx, jid, desc); err != nil {
				return "", fmt.Errorf("set description: %w", err)
			}
			return "group description updated with latest expense totals.", nil
		},
	}
}

func expenseAddBulkTool() Tool {
	return Tool{
		Spec: ai.ToolSpec{
			Name: "expense_add_bulk",
			Description: "Add several shared expenses at once. Use when the user lists multiple " +
				"items in a single message (e.g. 'add samad 200, 343, 3434; wifi 340 paid by sirajju'). " +
				"Guidance for parsing user input: if the user writes amounts joined by '+' or ',' " +
				"under the same name (like 'samad 200+343+100'), emit ONE item per amount so each is " +
				"independently editable. Each item has the same fields as expense_add. Items with no " +
				"paid_by land on credit. Returns a per-item summary.",
			Parameters: objectSchema(map[string]any{
				"items": map[string]any{
					"type":        "array",
					"description": "Expense entries to insert.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":    map[string]any{"type": "string", "description": "What was bought (e.g. 'veg', 'wifi')."},
							"amount":  map[string]any{"type": "number", "description": "Cost."},
							"paid_by": map[string]any{"type": "string", "description": "Optional: member who paid."},
							"note":    map[string]any{"type": "string", "description": "Optional free-text note."},
						},
						"required": []string{"name", "amount"},
					},
				},
			}, "items"),
		},
		Handler: func(env *ToolEnv, args map[string]any) (string, error) {
			raw, ok := args["items"].([]any)
			if !ok || len(raw) == 0 {
				return "", fmt.Errorf("items must be a non-empty array")
			}
			var ok_lines, failed []string
			for i, it := range raw {
				m, isMap := it.(map[string]any)
				if !isMap {
					failed = append(failed, fmt.Sprintf("#%d: not an object", i+1))
					continue
				}
				name := optString(m, "name")
				amt, hasAmt := optFloat(m, "amount")
				if name == "" || !hasAmt || amt <= 0 {
					failed = append(failed, fmt.Sprintf("#%d: need name + positive amount", i+1))
					continue
				}
				cli := []string{"add", name, fmtAmount(amt)}
				if p := optString(m, "paid_by"); p != "" {
					cli = append(cli, "("+p+")")
				}
				if n := optString(m, "note"); n != "" {
					cli = append(cli, n)
				}
				out, err := runExpense(env, cli...)
				if err != nil {
					failed = append(failed, fmt.Sprintf("#%d %s %s: %v", i+1, name, fmtAmount(amt), err))
					continue
				}
				ok_lines = append(ok_lines, "  "+out)
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Added %d/%d entries:\n", len(ok_lines), len(raw))
			for _, l := range ok_lines {
				b.WriteString(l + "\n")
			}
			if len(failed) > 0 {
				b.WriteString("\nFailures:\n")
				for _, f := range failed {
					b.WriteString("  " + f + "\n")
				}
			}
			return strings.TrimSpace(b.String()), nil
		},
	}
}
