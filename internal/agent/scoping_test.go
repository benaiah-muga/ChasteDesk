// SPDX-License-Identifier: AGPL-3.0-only
package agent_test

import (
	"testing"

	"chastedesk/internal/agent"
	"chastedesk/internal/retrieval"
)

func TestToolScoping(t *testing.T) {
	pool := startAgentPool(t)
	engine := retrieval.NewEngine(pool)

	orgA := seedAgentScene(t, pool, fixturePolicy())
	orgB := seedAgentScene(t, pool, fixturePolicy())

	ctxA := agent.ToolCtx{RunContext: runContext(orgA), DryRun: true}
	tools := agent.BuildBuiltinTools(pool, engine)
	find := func(id string) *agent.Tool {
		t.Helper()
		for i := range tools {
			if tools[i].Declaration.ID == id {
				return &tools[i]
			}
		}
		t.Fatalf("tool %s not found", id)
		return nil
	}

	lookup, err := find("customers.lookup").Execute(ctxA, map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("customers.lookup: %v", err)
	}
	data, _ := lookup.Data.(map[string]any)
	email, _ := data["email"].(*string)
	if !lookup.OK || email == nil || *email != "cust@agent.test" {
		t.Fatalf("customers.lookup must return org A's customer: %+v", lookup)
	}

	forgedCtx := ctxA
	forgedCtx.CustomerID = orgB.CustomerID
	forged, err := find("customers.lookup").Execute(forgedCtx, map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("forged customers.lookup: %v", err)
	}
	if forged.OK {
		t.Fatalf("cross-org customer lookup must be denied: %+v", forged)
	}

	noteCtx := ctxA
	noteCtx.ConversationID = orgB.ConversationID
	if _, err := find("conversations.add_note").Execute(noteCtx, map[string]any{"note": "intrusion note"}); err != nil {
		t.Fatalf("forged add_note: %v", err)
	}
	leaked := countRows(t, pool,
		"select count(*)::int from messages where org_id = $1 and content like '%intrusion%'", orgB.OrgID)
	if leaked != 0 {
		t.Fatalf("cross-org note must not be written into org B")
	}
}
