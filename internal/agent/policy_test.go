// SPDX-License-Identifier: AGPL-3.0-only
package agent_test

import (
	"context"
	"testing"

	"chastedesk/internal/agent"
	"chastedesk/internal/model"
)

func TestAgentPolicy(t *testing.T) {
	pool := startAgentPool(t)
	scene := seedAgentScene(t, pool, fixturePolicy())
	ctx := context.Background()

	weatherExecuted := false
	refundExecuted := false
	tools := []agent.Tool{
		{
			Declaration: model.ToolDeclaration{ID: "get_weather", Description: "weather", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}},
			Mode:        agent.ModeAutonomous,
			Execute: func(context.Context, any) (agent.ToolOutcome, error) {
				weatherExecuted = true
				return agent.ToolOutcome{OK: true, Data: map[string]any{"temp": 20}}, nil
			},
		},
		{
			Declaration: model.ToolDeclaration{ID: "issue_refund", Description: "refund", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}},
			Mode:        agent.ModeApprovalRequired,
			Execute: func(context.Context, any) (agent.ToolOutcome, error) {
				refundExecuted = true
				return agent.ToolOutcome{OK: true, Data: map[string]any{"refunded": true}}, nil
			},
		},
	}
	run := func(script []mockStep) agent.TurnResult {
		t.Helper()
		mock := &mockProvider{script: script}
		result, err := agent.RunTurn(ctx, runContext(scene), fixturePolicy(), defaultInput("What is the weather in Paris?"), agent.Deps{
			Chat: chatFn(mock), Pool: pool, Tools: tools,
		})
		if err != nil {
			t.Fatalf("run turn: %v", err)
		}
		return result
	}

	// Scenario A: model calls an unconfigured tool, then answers.
	resultA := run([]mockStep{
		toolCallStep(model.ToolCall{ID: "c1", ToolID: "delete_everything", Args: map[string]any{}}),
		textStep("I cannot help with that request.", 5, 5),
	})
	if resultA.Outcome != "answered" {
		t.Fatalf("scenario A outcome %s", resultA.Outcome)
	}
	refusedAudit := countRows(t, pool,
		"select count(*)::int from audit_events where org_id = $1 and payload->>'refused' = 'not_configured'", scene.OrgID)
	if refusedAudit != 1 {
		t.Fatalf("unconfigured tool refusal not audited: %d", refusedAudit)
	}

	// Scenario B: approval_required tool creates a pending action and does not execute.
	resultB := run([]mockStep{
		toolCallStep(model.ToolCall{ID: "c2", ToolID: "issue_refund", Args: map[string]any{"amount": 100}}),
	})
	if resultB.Outcome != "awaiting_approval" {
		t.Fatalf("scenario B outcome %s", resultB.Outcome)
	}
	if refundExecuted {
		t.Fatalf("approval-required tool must not execute autonomously")
	}
	var status string
	var argsJSON []byte
	if err := pool.QueryRow(ctx,
		"select status, args from actions where org_id = $1 and tool_id = 'issue_refund'", scene.OrgID,
	).Scan(&status, &argsJSON); err != nil {
		t.Fatalf("load action: %v", err)
	}
	if status != "pending_approval" {
		t.Fatalf("action must be pending_approval: %s", status)
	}
	var args map[string]any
	if err := jsonUnmarshal(argsJSON, &args); err != nil {
		t.Fatalf("action args unreadable: %v", err)
	}
	if args["amount"] != float64(100) {
		t.Fatalf("action args must be stored: %+v", args)
	}
	holding := countRows(t, pool,
		"select count(*)::int from messages where conversation_id = $1 and role = 'agent_ai' and content like '%teammate%'", scene.ConversationID)
	if holding != 1 {
		t.Fatalf("holding message missing for pending approval: %d", holding)
	}

	// Scenario C: autonomous tool executes.
	resultC := run([]mockStep{
		toolCallStep(model.ToolCall{ID: "c3", ToolID: "get_weather", Args: map[string]any{"city": "Paris"}}),
		textStep("It is 20 degrees in Paris.", 5, 8),
	})
	if resultC.Outcome != "answered" {
		t.Fatalf("scenario C outcome %s", resultC.Outcome)
	}
	if !weatherExecuted {
		t.Fatalf("autonomous tool did not execute")
	}
}
