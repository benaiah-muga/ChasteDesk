// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"chastedesk/internal/model"
	"chastedesk/internal/retrieval"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ToolCtx struct {
	RunContext
	DryRun bool
}

type ToolResult struct {
	OK          bool
	Data        any
	Error       string
	SideEffects []string
}

type Verification struct {
	Passed bool
	Detail string
}

type Tool struct {
	Declaration model.ToolDeclaration
	Mode        ToolMode
	Execute     func(ctx ToolCtx, args any) (ToolResult, error)
	Verify      func(ctx ToolCtx, args any) (Verification, error)
}

func argsMap(args any) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	if mapped, ok := args.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func requiredString(toolID string, args any, key string) (string, *ToolResult) {
	value, ok := argsMap(args)[key].(string)
	if !ok {
		return "", &ToolResult{OK: false, Error: fmt.Sprintf("invalid args for %s: %s must be a string", toolID, key)}
	}
	return value, nil
}

func optionalString(args any, key string) string {
	value, _ := argsMap(args)[key].(string)
	return value
}

func truncate300(content string) string {
	if len(content) > 300 {
		return content[:300]
	}
	return content
}

func BuildBuiltinTools(pool *pgxpool.Pool, engine *retrieval.Engine) []Tool {
	return []Tool{
		{
			Declaration: model.ToolDeclaration{
				ID:          "knowledge.search",
				Description: "Search the business knowledge base for information relevant to the customer question.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{"query": map[string]any{"type": "string", "description": "The search query"}},
					"required":   []any{"query"},
				},
			},
			Mode: ModeAutonomous,
			Execute: func(ctx ToolCtx, args any) (ToolResult, error) {
				query, failure := requiredString("knowledge.search", args, "query")
				if failure != nil {
					return *failure, nil
				}
				if engine == nil {
					return ToolResult{OK: true, Data: map[string]any{"evidence": []any{}, "note": "no knowledge index available"}, SideEffects: []string{}}, nil
				}
				evidence, err := engine.Search(context.Background(), ctx.OrgID, retrieval.SearchQuery{
					Text: query, Scopes: []string{"public", "internal"}, Limit: 5,
				})
				if err != nil {
					return ToolResult{OK: false, Error: err.Error(), SideEffects: []string{}}, nil
				}
				out := make([]map[string]any, 0, len(evidence))
				for _, item := range evidence {
					out = append(out, map[string]any{
						"source": item.SourceName, "version": item.Version,
						"quote": truncate300(item.Content), "confidence": item.Confidence,
					})
				}
				return ToolResult{OK: true, Data: out, SideEffects: []string{}}, nil
			},
		},
		{
			Declaration: model.ToolDeclaration{
				ID:          "customers.lookup",
				Description: "Look up the customer record for the current conversation.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{"query": map[string]any{"type": "string"}},
					"required":   []any{},
				},
			},
			Mode: ModeAutonomous,
			Execute: func(ctx ToolCtx, args any) (ToolResult, error) {
				if value, present := argsMap(args)["query"]; present {
					if _, ok := value.(string); !ok {
						return ToolResult{OK: false, Error: "invalid args for customers.lookup: query must be a string", SideEffects: []string{}}, nil
					}
				}
				var id, email, name *string
				var attributes []byte
				err := pool.QueryRow(context.Background(),
					`select c.id, c.email, c.name, c.attributes from customers c where c.id = $1 and c.org_id = $2`,
					ctx.CustomerID, ctx.OrgID).Scan(&id, &email, &name, &attributes)
				if err != nil {
					return ToolResult{OK: false, Error: "customer not found in this organization", SideEffects: []string{}}, nil
				}
				var attrs any
				_ = json.Unmarshal(attributes, &attrs)
				return ToolResult{OK: true, Data: map[string]any{"id": id, "email": email, "name": name, "attributes": attrs}, SideEffects: []string{}}, nil
			},
			Verify: func(ctx ToolCtx, args any) (Verification, error) {
				var one int
				err := pool.QueryRow(context.Background(),
					`select 1 from customers where id = $1 and org_id = $2`, ctx.CustomerID, ctx.OrgID).Scan(&one)
				if err != nil {
					return Verification{Passed: false, Detail: "customer record missing"}, nil
				}
				return Verification{Passed: true, Detail: "customer record present"}, nil
			},
		},
		{
			Declaration: model.ToolDeclaration{
				ID:          "conversations.add_note",
				Description: "Add an internal note to the current conversation. Notes are never shown to the customer.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{"note": map[string]any{"type": "string", "description": "The note text"}},
					"required":   []any{"note"},
				},
			},
			Mode: ModeAutonomous,
			Execute: func(ctx ToolCtx, args any) (ToolResult, error) {
				note, failure := requiredString("conversations.add_note", args, "note")
				if failure != nil {
					return *failure, nil
				}
				if _, err := pool.Exec(context.Background(),
					`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'internal_note', $3)`,
					ctx.OrgID, ctx.ConversationID, "[AI note] "+note); err != nil {
					return ToolResult{OK: false, Error: err.Error(), SideEffects: []string{}}, nil
				}
				return ToolResult{OK: true, Data: map[string]any{"added": true}, SideEffects: []string{"internal_note_added"}}, nil
			},
			Verify: func(ctx ToolCtx, args any) (Verification, error) {
				note := optionalString(args, "note")
				var one int
				err := pool.QueryRow(context.Background(),
					`select 1 from messages where conversation_id = $1 and org_id = $2 and role = 'internal_note' and content = $3`,
					ctx.ConversationID, ctx.OrgID, "[AI note] "+note).Scan(&one)
				if err != nil {
					return Verification{Passed: false, Detail: "note not found"}, nil
				}
				return Verification{Passed: true, Detail: "note persisted"}, nil
			},
		},
		{
			Declaration: model.ToolDeclaration{
				ID:          "escalate",
				Description: "Escalate the conversation to a human teammate with a summary and a recommended next step.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"reason":              map[string]any{"type": "string"},
						"summary":             map[string]any{"type": "string"},
						"recommendedNextStep": map[string]any{"type": "string"},
					},
					"required": []any{"reason", "summary", "recommendedNextStep"},
				},
			},
			Mode: ModeAutonomous,
			Execute: func(ctx ToolCtx, args any) (ToolResult, error) {
				reason, failure := requiredString("escalate", args, "reason")
				if failure != nil {
					return *failure, nil
				}
				summary, failure := requiredString("escalate", args, "summary")
				if failure != nil {
					return *failure, nil
				}
				recommendedNextStep, failure := requiredString("escalate", args, "recommendedNextStep")
				if failure != nil {
					return *failure, nil
				}
				return ToolResult{
					OK:          true,
					Data:        map[string]any{"reason": reason, "summary": summary, "recommendedNextStep": recommendedNextStep},
					SideEffects: []string{"escalation_requested"},
				}, nil
			},
		},
	}
}
