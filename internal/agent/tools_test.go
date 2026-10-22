// SPDX-License-Identifier: AGPL-3.0-only
package agent_test

import (
	"context"
	"testing"

	"chastedesk/internal/agent"
	"chastedesk/internal/model"
	"chastedesk/internal/retrieval"
)

func TestAgentSendsNativeToolCallsBackToProvider(t *testing.T) {
	pool := startAgentPool(t)
	scene := seedAgentScene(t, pool, fixturePolicy())
	seedChunk(t, pool, scene.OrgID, "Unused seats may be credited within 14 days of removal.")

	mock := &mockProvider{script: []mockStep{
		toolCallStep(model.ToolCall{ID: "call_1", ToolID: "knowledge.search", Args: map[string]any{"query": "unused seats"}}),
		textStep("Unused seats may be credited within 14 days of removal. [E1]", 20, 12),
	}}
	tools := []agent.Tool{{
		Declaration: model.ToolDeclaration{ID: "knowledge.search", Description: "Search knowledge.", Parameters: map[string]any{"type": "object"}},
		Mode:        agent.ModeAutonomous,
		Execute: func(agent.ToolCtx, any) (agent.ToolResult, error) {
			return agent.ToolResult{OK: true, Data: map[string]any{"found": true}}, nil
		},
	}}
	result, err := agent.RunTurn(context.Background(), runContext(scene), fixturePolicy(), defaultInput("What happens to unused seats?"), agent.Deps{
		Chat: chatFn(mock), Pool: pool, Retrieval: retrieval.NewEngine(pool), Tools: tools,
	})
	if err != nil {
		t.Fatalf("run turn: %v", err)
	}
	if result.Outcome != "answered" {
		t.Fatalf("outcome should be answered, got %s: %s", result.Outcome, result.Error)
	}
	if len(mock.requests) < 2 {
		t.Fatalf("provider should see a follow-up request after the tool result, got %d", len(mock.requests))
	}
	found := false
	for _, message := range mock.requests[1].Messages {
		if message.Role == "assistant" && message.ToolCallID == "call_1" && message.ToolName == "knowledge.search" {
			found = true
			if message.Content != `{"query":"unused seats"}` {
				t.Fatalf("tool arguments were not preserved: %s", message.Content)
			}
		}
	}
	if !found {
		t.Fatalf("native assistant tool call was not present in the follow-up request: %+v", mock.requests[1].Messages)
	}
}
