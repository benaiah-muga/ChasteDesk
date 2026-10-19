// SPDX-License-Identifier: AGPL-3.0-only
package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"chastedesk/internal/agent"
	"chastedesk/internal/db"
	"chastedesk/internal/id"
	"chastedesk/internal/model"
	"chastedesk/internal/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

type scene struct {
	OrgID          string
	AgentID        string
	CustomerID     string
	ConversationID string
}

func fixturePolicy() agent.AgentPolicy {
	return agent.AgentPolicy{
		Purpose:         "Answer customer questions from the knowledge base and escalate sensitive requests.",
		KnowledgeScopes: []string{"public"},
		Tools: []agent.PolicyTool{
			{ToolID: "knowledge.search", Mode: "autonomous"},
			{ToolID: "customers.lookup", Mode: "autonomous"},
			{ToolID: "conversations.add_note", Mode: "autonomous"},
			{ToolID: "escalate", Mode: "autonomous"},
		},
		Escalation: agent.EscalationPolicy{
			LowConfidenceThreshold: 0.35,
			Keywords:               []string{"refund", "cancel my account"},
			AlwaysEscalateTopics:   []string{},
		},
		Model:   agent.ModelPolicy{ProviderID: "mock", ModelID: "mock-mini"},
		Budgets: agent.BudgetPolicy{MaxSteps: 6, MaxToolCalls: 4, MaxInputTokens: 8000, MaxOutputTokens: 2000},
	}
}

func startAgentPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testutil.StartEphemeralPG(t)
	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func seedAgentScene(t *testing.T, pool *pgxpool.Pool, policy agent.AgentPolicy) scene {
	t.Helper()
	ctx := context.Background()
	s := scene{
		OrgID:          "org-" + id.UUID()[:8],
		AgentID:        id.UUID(),
		CustomerID:     id.UUID(),
		ConversationID: id.UUID(),
	}
	if _, err := pool.Exec(ctx, `insert into organization (id, name, slug) values ($1, 'Agent Org', $2)`,
		s.OrgID, "agent-"+s.OrgID[4:12]); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	encodedPolicy, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into agents (id, org_id, name, policy, is_active) values ($1, $2, 'Support agent', $3::jsonb, true)`,
		s.AgentID, s.OrgID, string(encodedPolicy)); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'cust@agent.test', 'Cora Customer', $3::jsonb)`,
		s.CustomerID, s.OrgID, `{"tier":"growth"}`); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')`,
		s.ConversationID, s.OrgID, s.CustomerID); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', $3)`,
		s.OrgID, s.ConversationID, "I need help with my account."); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	return s
}

func seedChunk(t *testing.T, pool *pgxpool.Pool, orgID, content string) string {
	t.Helper()
	ctx := context.Background()
	sourceID := id.UUID()
	if _, err := pool.Exec(ctx, `insert into knowledge_sources (id, org_id, type, name, scope, status, version)
		values ($1, $2, 'text', 'Help Docs', 'public', 'ready', 1)`, sourceID, orgID); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	chunkID := id.UUID()
	locator := map[string]any{"headings": []string{"Help"}, "charRange": []int{0, len(content)}}
	encodedLocator, _ := json.Marshal(locator)
	if _, err := pool.Exec(ctx, `insert into knowledge_chunks (id, org_id, source_id, version, chunk_index, content, locator, token_count)
		values ($1, $2, $3, 1, 0, $4, $5::jsonb, 50)`, chunkID, orgID, sourceID, content, string(encodedLocator)); err != nil {
		t.Fatalf("insert chunk: %v", err)
	}
	return chunkID
}

type mockStep struct {
	Text         *string
	InputTokens  int
	OutputTokens int
	Finish       string
	ToolCalls    []model.ToolCall
}

type mockProvider struct {
	script []mockStep
}

func (m *mockProvider) chat(_ model.ChatRequest) (model.ChatResult, error) {
	step := m.script[0]
	if len(m.script) > 1 {
		step = m.script[0]
		m.script = m.script[1:]
	}
	input, output := step.InputTokens, step.OutputTokens
	if input == 0 {
		input = 5
	}
	if output == 0 {
		output = 3
	}
	finish := step.Finish
	if finish == "" {
		finish = "stop"
	}
	return model.ChatResult{
		Text:         step.Text,
		ToolCalls:    step.ToolCalls,
		Usage:        model.Usage{InputTokens: input, OutputTokens: output},
		FinishReason: model.FinishReason(finish),
	}, nil
}

func chatFn(m *mockProvider) func(context.Context, model.ChatRequest) (model.ChatResult, error) {
	return func(_ context.Context, req model.ChatRequest) (model.ChatResult, error) {
		return m.chat(req)
	}
}

func text(value string) *string {
	return &value
}

func textStep(value string, inputTokens, outputTokens int) mockStep {
	return mockStep{Text: text(value), InputTokens: inputTokens, OutputTokens: outputTokens, Finish: "stop"}
}

func toolCallStep(calls ...model.ToolCall) mockStep {
	return mockStep{ToolCalls: calls, Finish: "tool_calls"}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return count
}

func jsonUnmarshal(data []byte, target any) error {
	return json.Unmarshal(data, target)
}

func runContext(s scene) agent.RunContext {
	return agent.RunContext{
		OrgID:          s.OrgID,
		ConversationID: s.ConversationID,
		AgentID:        s.AgentID,
		RunID:          id.UUID(),
		CustomerID:     s.CustomerID,
		Channel:        "widget",
	}
}

func defaultInput(message string) agent.TurnInput {
	return agent.TurnInput{
		CustomerMessage:     message,
		History:             []agent.HistoryMessage{},
		Customer:            agent.Customer{Name: text("Cora"), Email: text("cust@agent.test"), Attributes: map[string]any{"tier": "pro"}},
		RecentConversations: []string{},
	}
}
