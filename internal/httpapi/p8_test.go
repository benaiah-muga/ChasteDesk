// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"chastedesk/internal/agent"
	"chastedesk/internal/db"
	"chastedesk/internal/id"
	"chastedesk/internal/queue"
	"chastedesk/internal/retrieval"

	"github.com/jackc/pgx/v5/pgxpool"
)

type actionScene struct {
	OrgID          string
	CustomerID     string
	ConversationID string
	AgentID        string
}

func startActionTestApp(t *testing.T) *workerTestApp {
	t.Helper()
	app := startWorkerTestApp(t, t.TempDir())
	app.jobs.Work(app.ctx, "execute-action", 2, agent.ExecuteActionHandler(app.pool, retrieval.NewEngine(app.pool)))
	return app
}

func seedActionScene(t *testing.T, pool *pgxpool.Pool, orgID string) actionScene {
	t.Helper()
	ctx := context.Background()
	scene := actionScene{OrgID: orgID, CustomerID: id.UUID(), ConversationID: id.UUID(), AgentID: id.UUID()}
	if _, err := pool.Exec(ctx,
		`insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'c@action.test', 'Cust', '{}'::jsonb)`,
		scene.CustomerID, orgID); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')`,
		scene.ConversationID, orgID, scene.CustomerID); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`insert into agents (id, org_id, name, policy, is_active) values ($1, $2, 'Gate agent', '{}'::jsonb, true)`,
		scene.AgentID, orgID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	return scene
}

func insertPendingAction(t *testing.T, pool *pgxpool.Pool, scene actionScene, note string) string {
	t.Helper()
	ctx := context.Background()
	runID := id.UUID()
	actionID := id.UUID()
	if _, err := pool.Exec(ctx,
		`insert into ai_runs (id, org_id, conversation_id, agent_id, status) values ($1, $2, $3, $4, 'completed')`,
		runID, scene.OrgID, scene.ConversationID, scene.AgentID); err != nil {
		t.Fatalf("insert run: %v", err)
	}
	args, _ := json.Marshal(map[string]any{"note": note})
	if _, err := pool.Exec(ctx,
		`insert into actions (id, org_id, run_id, conversation_id, agent_id, tool_id, args, status)
		 values ($1, $2, $3, $4, $5, 'conversations.add_note', $6::jsonb, 'pending_approval')`,
		actionID, scene.OrgID, runID, scene.ConversationID, scene.AgentID, string(args)); err != nil {
		t.Fatalf("insert action: %v", err)
	}
	return actionID
}

func waitFor(t *testing.T, timeout time.Duration, check func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func TestToolsApprovalLifecycle(t *testing.T) {
	app := startActionTestApp(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Approve Org", "owner@approve.test")
	scene := seedActionScene(t, app.pool, orgID)
	ctx := context.Background()

	toApprove := insertPendingAction(t, app.pool, scene, "note-approve")
	toReject := insertPendingAction(t, app.pool, scene, "note-reject")

	approve := app.request("POST", "/api/actions/decide", map[string]any{
		"actionId": toApprove, "decision": "approve",
	}, jar, "")
	if approve.status != http.StatusOK {
		t.Fatalf("approve failed: %d %s", approve.status, approve.body)
	}
	if status, _ := approve.data(t)["status"].(string); status != "approved" {
		t.Fatalf("approve response status wrong: %s", approve.body)
	}

	executed := waitFor(t, 20*time.Second, func() bool {
		var status string
		_ = app.pool.QueryRow(ctx, "select status from actions where id = $1", toApprove).Scan(&status)
		return status == "executed"
	})
	if !executed {
		t.Fatalf("approved action never reached executed")
	}
	var decidedBy *string
	if err := app.pool.QueryRow(ctx, "select decided_by from actions where id = $1", toApprove).Scan(&decidedBy); err != nil {
		t.Fatalf("load decided_by: %v", err)
	}
	if decidedBy == nil {
		t.Fatalf("decided_by must be recorded")
	}

	reject := app.request("POST", "/api/actions/decide", map[string]any{
		"actionId": toReject, "decision": "reject",
	}, jar, "")
	if reject.status != http.StatusOK {
		t.Fatalf("reject failed: %d %s", reject.status, reject.body)
	}
	if status, _ := reject.data(t)["status"].(string); status != "rejected_before_exec" {
		t.Fatalf("reject response status wrong: %s", reject.body)
	}

	again := app.request("POST", "/api/actions/decide", map[string]any{
		"actionId": toApprove, "decision": "approve",
	}, jar, "")
	if again.status != http.StatusConflict {
		t.Fatalf("double approve must 409, got %d", again.status)
	}

	var audits int
	if err := app.pool.QueryRow(ctx,
		`select count(*)::int from audit_events where org_id = $1 and type in ('action.approved','action.rejected')`,
		orgID).Scan(&audits); err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if audits < 2 {
		t.Fatalf("approve and reject must both be audited: %d", audits)
	}
}

func TestToolsVerificationGate(t *testing.T) {
	app := startActionTestApp(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Verify Org", "owner@verify.test")
	scene := seedActionScene(t, app.pool, orgID)
	ctx := context.Background()

	passing := insertPendingAction(t, app.pool, scene, "verify-pass-note")
	related := app.request("POST", "/api/actions/decide", map[string]any{
		"actionId": passing, "decision": "approve",
	}, jar, "")
	if related.status != http.StatusOK {
		t.Fatalf("approve passing action: %d %s", related.status, related.body)
	}

	failing := insertPendingAction(t, app.pool, scene, "verify-fail-note")
	var userID string
	if err := app.pool.QueryRow(ctx, `select id from "user" where email = 'owner@verify.test'`).Scan(&userID); err != nil {
		t.Fatalf("load owner user: %v", err)
	}
	if _, err := app.pool.Exec(ctx,
		`update actions set status = 'approved', decided_by = $2, decided_at = now() where id = $1`, failing, userID); err != nil {
		t.Fatalf("force failing action approved: %v", err)
	}
	jobID, err := app.jobs.Send(ctx, "execute-action", map[string]any{
		"orgId": orgID, "actionId": failing, "dryRunVerifyFail": true,
	}, queue.SendOptions{SingletonKey: "action:" + failing})
	if err != nil || jobID == nil {
		t.Fatalf("execute-action job send failed: %v", err)
	}

	bothDone := waitFor(t, 25*time.Second, func() bool {
		var count int
		_ = app.pool.QueryRow(ctx,
			`select count(*)::int from actions where id in ($1, $2) and status = 'executed' and verification is not null`,
			passing, failing).Scan(&count)
		return count == 2
	})
	if !bothDone {
		t.Fatalf("both actions must finish processing")
	}

	var doneMessages int
	if err := app.pool.QueryRow(ctx,
		`select count(*)::int from messages where conversation_id = $1 and role = 'agent_ai' and content like '%Done%'`,
		scene.ConversationID).Scan(&doneMessages); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if doneMessages != 1 {
		t.Fatalf("expected exactly one customer-facing success message, got %d", doneMessages)
	}

	var verification []byte
	if err := app.pool.QueryRow(ctx, "select verification from actions where id = $1", failing).Scan(&verification); err != nil {
		t.Fatalf("load verification: %v", err)
	}
	var decoded struct {
		Passed bool `json:"passed"`
	}
	if err := json.Unmarshal(verification, &decoded); err != nil {
		t.Fatalf("decode verification: %v", err)
	}
	if decoded.Passed {
		t.Fatalf("failing action must record failed verification")
	}
}

func TestToolsAuditGate(t *testing.T) {
	app := startActionTestApp(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Audit Org", "owner@audit.test")
	scene := seedActionScene(t, app.pool, orgID)
	ctx := context.Background()

	actionID := insertPendingAction(t, app.pool, scene, "audit-note")
	agentID := scene.AgentID
	related := actionID
	if err := db.NewRepo(app.pool).WriteAuditNoOrg(ctx, &orgID, db.AuditInput{
		ActorType: db.ActorAgent, ActorID: &agentID, Type: "action.requested",
		TargetType: "action", TargetID: &related, Payload: map[string]any{"note": "audit-note"},
	}); err != nil {
		t.Fatalf("write action.requested audit: %v", err)
	}

	decide := app.request("POST", "/api/actions/decide", map[string]any{
		"actionId": actionID, "decision": "approve",
	}, jar, "")
	if decide.status != http.StatusOK {
		t.Fatalf("approve failed: %d %s", decide.status, decide.body)
	}
	if !waitFor(t, 20*time.Second, func() bool {
		var status string
		_ = app.pool.QueryRow(ctx, "select status from actions where id = $1", actionID).Scan(&status)
		return status == "executed"
	}) {
		t.Fatalf("action never reached executed")
	}

	rows, err := app.pool.Query(ctx,
		`select type from audit_events where org_id = $1 and target_id = $2 and type like 'action.%'`, orgID, actionID)
	if err != nil {
		t.Fatalf("query audits: %v", err)
	}
	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan audit: %v", err)
		}
		names[name] = true
	}
	rows.Close()
	for _, expected := range []string{"action.requested", "action.approved", "action.executed", "action.verified"} {
		if !names[expected] {
			t.Fatalf("audit row missing: %s", expected)
		}
	}

	if _, err := app.pool.Exec(ctx, "update audit_events set type = 'tampered' where org_id = $1", orgID); err == nil {
		t.Fatalf("UPDATE on audit_events must fail")
	}
}
