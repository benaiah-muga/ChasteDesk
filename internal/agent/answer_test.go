// SPDX-License-Identifier: AGPL-3.0-only
package agent_test

import (
	"context"
	"testing"

	"chastedesk/internal/agent"
	"chastedesk/internal/retrieval"
)

func TestAgentAnswer(t *testing.T) {
	pool := startAgentPool(t)
	scene := seedAgentScene(t, pool, fixturePolicy())
	chunkID := seedChunk(t, pool, scene.OrgID, "To reset your password, open the security settings page and choose a new credential. You can also use the forgot-password link on the sign in screen.")

	mock := &mockProvider{script: []mockStep{
		textStep("You can reset your password from the security settings page. [E1]", 20, 12),
	}}
	rc := runContext(scene)
	engine := retrieval.NewEngine(pool)

	result, err := agent.RunTurn(context.Background(), rc, fixturePolicy(), defaultInput("How do I reset my password?"), agent.Deps{
		Chat: chatFn(mock), Pool: pool, Retrieval: engine, Tools: []agent.Tool{},
	})
	if err != nil {
		t.Fatalf("run turn: %v", err)
	}
	if result.Outcome != "answered" {
		t.Fatalf("outcome should be answered, got %s: %s", result.Outcome, result.Error)
	}
	if len(result.Citations) != 1 || result.Citations[0].Marker != "E1" {
		t.Fatalf("answer must carry exactly one [E1] citation: %+v", result.Citations)
	}
	if result.Citations[0].ChunkID != chunkID {
		t.Fatalf("citation must point at the seeded chunk")
	}
	if result.Usage.Steps < 1 || result.Usage.InputTokens <= 0 {
		t.Fatalf("usage must be recorded: %+v", result.Usage)
	}

	var status, outcome string
	var stepsJSON, usageJSON []byte
	if err := pool.QueryRow(context.Background(),
		"select status, outcome, steps, usage from ai_runs where id = $1", rc.RunID,
	).Scan(&status, &outcome, &stepsJSON, &usageJSON); err != nil {
		t.Fatalf("load run: %v", err)
	}
	if status != "completed" || outcome != "answered" {
		t.Fatalf("run must be completed/answered: %s/%s", status, outcome)
	}
	var steps []map[string]any
	if err := jsonUnmarshal(stepsJSON, &steps); err != nil || len(steps) == 0 {
		t.Fatalf("run steps missing: %v", err)
	}
	var usage map[string]any
	if err := jsonUnmarshal(usageJSON, &usage); err != nil {
		t.Fatalf("run usage unreadable: %v", err)
	}
	if inputTokens, ok := usage["inputTokens"].(float64); !ok || inputTokens <= 0 {
		t.Fatalf("run usage missing: %s", usageJSON)
	}

	var content string
	var citationsJSON []byte
	if err := pool.QueryRow(context.Background(),
		"select content, citations from messages where conversation_id = $1 and role = 'agent_ai'", scene.ConversationID,
	).Scan(&content, &citationsJSON); err != nil {
		t.Fatalf("assistant message must be persisted: %v", err)
	}
	var citations []map[string]any
	if err := jsonUnmarshal(citationsJSON, &citations); err != nil || len(citations) == 0 {
		t.Fatalf("message citations must reference the chunk: %v", err)
	}
	if citations[0]["chunkId"] != chunkID {
		t.Fatalf("message citations must reference the chunk: %+v", citations[0])
	}

	audits := countRows(t, pool,
		"select count(*)::int from audit_events where org_id = $1 and type in ('run.started','run.completed')", scene.OrgID)
	if audits != 2 {
		t.Fatalf("expected run.started + run.completed audits, got %d", audits)
	}
}
