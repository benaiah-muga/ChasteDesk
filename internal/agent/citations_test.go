// SPDX-License-Identifier: AGPL-3.0-only
package agent_test

import (
	"context"
	"testing"

	"chastedesk/internal/agent"
	"chastedesk/internal/retrieval"
)

func TestAgentCitations(t *testing.T) {
	pool := startAgentPool(t)
	scene := seedAgentScene(t, pool, fixturePolicy())
	seedChunk(t, pool, scene.OrgID, "Password resets happen on the security settings page.")
	engine := retrieval.NewEngine(pool)
	ctx := context.Background()

	run := func(script []mockStep) agent.TurnResult {
		t.Helper()
		mock := &mockProvider{script: script}
		result, err := agent.RunTurn(ctx, runContext(scene), fixturePolicy(), defaultInput("How do I reset my password?"), agent.Deps{
			Chat: chatFn(mock), Pool: pool, Retrieval: engine, Tools: []agent.Tool{},
		})
		if err != nil {
			t.Fatalf("run turn: %v", err)
		}
		return result
	}

	res1 := run([]mockStep{
		textStep("Check the settings page.", 5, 3),
		textStep("Still no citations, sorry!", 5, 3),
	})
	if res1.Outcome != "escalated" || res1.EscalationReason != agent.ReasonMissingCitations {
		t.Fatalf("uncited+retry must escalate missing_citations, got %s/%s", res1.Outcome, res1.EscalationReason)
	}

	res2 := run([]mockStep{
		textStep("Check the settings page. [E7]", 5, 3),
		textStep("See [E42] for details.", 5, 3),
	})
	if res2.Outcome != "escalated" || res2.EscalationReason != agent.ReasonMissingCitations {
		t.Fatalf("hallucinated citations must escalate: %s/%s", res2.Outcome, res2.EscalationReason)
	}

	res3 := run([]mockStep{
		textStep("No answer available. [E1]", 5, 3),
	})
	if res3.Outcome != "answered" {
		t.Fatalf("a properly cited retry must answer: %s", res3.Outcome)
	}
}
