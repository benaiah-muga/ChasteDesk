// SPDX-License-Identifier: AGPL-3.0-only
package agent_test

import (
	"context"
	"testing"

	"chastedesk/internal/agent"
	"chastedesk/internal/model"
	"chastedesk/internal/retrieval"
)

func TestAgentEscalation(t *testing.T) {
	pool := startAgentPool(t)
	engine := retrieval.NewEngine(pool)
	ctx := context.Background()

	type scenarioResult struct {
		result             agent.TurnResult
		conversationStatus string
		escalatedAudits    int
	}

	runScenario := func(t *testing.T, customerMessage string, script []mockStep, policy agent.AgentPolicy, withEvidence bool, tools []agent.Tool) scenarioResult {
		t.Helper()
		scene := seedAgentScene(t, pool, policy)
		if withEvidence {
			seedChunk(t, pool, scene.OrgID, "Our refund policy says refunds require human approval within 30 days of purchase.")
		}
		mock := &mockProvider{script: script}
		deps := agent.Deps{Chat: chatFn(mock), Pool: pool, Tools: tools}
		if withEvidence {
			deps.Retrieval = engine
		}
		input := defaultInput(customerMessage)
		input.RecentConversations = []string{"Prior chat about invoices"}
		result, err := agent.RunTurn(ctx, runContext(scene), policy, input, deps)
		if err != nil {
			t.Fatalf("run turn: %v", err)
		}
		var status string
		if err := pool.QueryRow(ctx, "select status from conversations where id = $1", scene.ConversationID).Scan(&status); err != nil {
			t.Fatalf("load conversation: %v", err)
		}
		audits := countRows(t, pool,
			"select count(*)::int from audit_events where org_id = $1 and type = 'run.escalated'", scene.OrgID)
		return scenarioResult{result: result, conversationStatus: status, escalatedAudits: audits}
	}

	stop := func() []mockStep { return []mockStep{textStep("Okay", 5, 3)} }

	// 1. policy keyword
	kw := runScenario(t, "I demand a refund immediately.", stop(), fixturePolicy(), false, []agent.Tool{})
	if kw.result.EscalationReason != agent.ReasonPolicyKeyword {
		t.Fatalf("keyword trigger failed: %s", kw.result.EscalationReason)
	}

	// 2. low confidence: threshold 0.9 (single matching chunk yields ~0.45)
	lowPolicy := fixturePolicy()
	lowPolicy.Escalation.LowConfidenceThreshold = 0.9
	low := runScenario(t, "How do I reset my password?", stop(), lowPolicy, true, []agent.Tool{})
	if low.result.EscalationReason != agent.ReasonLowConfidence {
		t.Fatalf("low confidence trigger failed: %s", low.result.EscalationReason)
	}

	// 3. explicit human request
	explicit := runScenario(t, "I want to talk to a human being right now.", stop(), fixturePolicy(), false, []agent.Tool{})
	if explicit.result.EscalationReason != agent.ReasonExplicitRequest {
		t.Fatalf("explicit request trigger failed: %s", explicit.result.EscalationReason)
	}

	// 4. budget exhaustion: maxSteps 1 consumed by a tool call, then nothing left
	budgetPolicy := fixturePolicy()
	budgetPolicy.Budgets.MaxSteps = 1
	budget := runScenario(t, "What is the weather?", []mockStep{
		toolCallStep(model.ToolCall{ID: "c1", ToolID: "knowledge.search", Args: map[string]any{}}),
	}, budgetPolicy, false, []agent.Tool{})
	if budget.result.EscalationReason != agent.ReasonBudgetExhausted {
		t.Fatalf("budget trigger failed: %s", budget.result.EscalationReason)
	}

	// 5. denied tool: model attempts a disabled tool
	deniedTools := []agent.Tool{{
		Declaration: model.ToolDeclaration{ID: "issue_refund", Description: "refund", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}},
		Mode:        agent.ModeDisabled,
		Execute:     func(agent.ToolCtx, any) (agent.ToolResult, error) { return agent.ToolResult{OK: false}, nil },
	}}
	denied := runScenario(t, "Please process something for me.", []mockStep{
		toolCallStep(model.ToolCall{ID: "c2", ToolID: "issue_refund", Args: map[string]any{"amount": 500}}),
	}, fixturePolicy(), false, deniedTools)
	if denied.result.EscalationReason != agent.ReasonDeniedTool {
		t.Fatalf("denied tool trigger failed: %s", denied.result.EscalationReason)
	}

	// Handoff package completeness on every escalation.
	if kw.result.Handoff == nil {
		t.Fatalf("handoff missing")
	}
	handoff := kw.result.Handoff
	if handoff.Problem == "" {
		t.Fatalf("handoff problem missing")
	}
	if handoff.CustomerContext.Name == nil || *handoff.CustomerContext.Name != "Cora" ||
		handoff.CustomerContext.Email == nil || *handoff.CustomerContext.Email != "cust@agent.test" {
		t.Fatalf("handoff customer context incomplete: %+v", handoff.CustomerContext)
	}
	if handoff.CustomerContext.RecentConversations == nil {
		t.Fatalf("handoff recent conversations missing")
	}
	if handoff.Evidence == nil || handoff.ActionsTaken == nil || handoff.Investigated == nil {
		t.Fatalf("handoff arrays missing")
	}
	if handoff.RecommendedNextStep == "" {
		t.Fatalf("handoff recommended next step missing")
	}
	if kw.conversationStatus != "awaiting_human" {
		t.Fatalf("conversation status must be awaiting_human, got %s", kw.conversationStatus)
	}

	totalEscalatedAudits := 0
	for _, scenario := range []scenarioResult{kw, low, explicit, budget, denied} {
		totalEscalatedAudits += scenario.escalatedAudits
	}
	if totalEscalatedAudits < 5 {
		t.Fatalf("expected at least 5 escalated audits, got %d", totalEscalatedAudits)
	}
}
