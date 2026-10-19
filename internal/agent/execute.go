// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"chastedesk/internal/db"
	"chastedesk/internal/queue"
	"chastedesk/internal/retrieval"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ExecuteActionData struct {
	OrgID            string `json:"orgId"`
	ActionID         string `json:"actionId"`
	DryRunVerifyFail bool   `json:"dryRunVerifyFail"`
}

func ExecuteActionHandler(pool *pgxpool.Pool, engine *retrieval.Engine) queue.Handler {
	return func(ctx context.Context, job queue.Job) error {
		var data ExecuteActionData
		if err := json.Unmarshal(job.Data, &data); err != nil {
			return err
		}
		return ExecuteApprovedAction(ctx, pool, engine, data)
	}
}

type actionRecord struct {
	ID             string
	OrgID          string
	RunID          string
	ConversationID string
	AgentID        string
	ToolID         string
	Args           []byte
	Status         string
}

func loadActionForUpdate(ctx context.Context, pool *pgxpool.Pool, orgID, actionID string) (actionRecord, error) {
	var action actionRecord
	err := pool.QueryRow(ctx,
		`select id, org_id, run_id, conversation_id, agent_id, tool_id, args, status
		   from actions where id = $1 and org_id = $2 for update`, actionID, orgID,
	).Scan(&action.ID, &action.OrgID, &action.RunID, &action.ConversationID, &action.AgentID, &action.ToolID, &action.Args, &action.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return action, fmt.Errorf("action %s not found", actionID)
	}
	return action, err
}

func auditAction(ctx context.Context, pool *pgxpool.Pool, orgID, agentID, auditType, actionID string, payload map[string]any) error {
	actorID := agentID
	id := actionID
	return db.NewRepo(pool).WriteAuditNoOrg(ctx, &orgID, db.AuditInput{
		ActorType: db.ActorAgent, ActorID: &actorID, Type: auditType, TargetType: "action", TargetID: &id, Payload: payload,
	})
}

func failAction(ctx context.Context, pool *pgxpool.Pool, action actionRecord, result ToolResult) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx,
		`update actions set status = 'failed', result = $2::jsonb, updated_at = now() where id = $1`,
		action.ID, string(encoded)); err != nil {
		return err
	}
	failure := result.Error
	if failure == "" {
		failure = "failed"
	}
	if err := auditAction(ctx, pool, action.OrgID, action.AgentID, "action.failed", action.ID, map[string]any{"error": failure}); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx,
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'system', $3)`,
		action.OrgID, action.ConversationID, "The requested action could not be completed. Escalating to a teammate."); err != nil {
		return err
	}
	_, err = pool.Exec(ctx,
		`update conversations set status = 'awaiting_human', updated_at = now() where id = $1`, action.ConversationID)
	return err
}

func ExecuteApprovedAction(ctx context.Context, pool *pgxpool.Pool, engine *retrieval.Engine, data ExecuteActionData) error {
	action, err := loadActionForUpdate(ctx, pool, data.OrgID, data.ActionID)
	if err != nil {
		return err
	}
	if action.Status != "approved" {
		return fmt.Errorf("action %s is not approved (status %s)", data.ActionID, action.Status)
	}

	var customerID, channel string
	if err := pool.QueryRow(ctx, `select customer_id, channel from conversations where id = $1`, action.ConversationID).Scan(&customerID, &channel); err != nil {
		return err
	}
	toolCtx := ToolCtx{
		RunContext: RunContext{
			OrgID: action.OrgID, ConversationID: action.ConversationID, AgentID: action.AgentID,
			RunID: action.RunID, CustomerID: customerID, Channel: channel,
		},
		DryRun: data.DryRunVerifyFail,
	}

	tools := BuildBuiltinTools(pool, engine)
	var tool *Tool
	for i := range tools {
		if tools[i].Declaration.ID == action.ToolID {
			tool = &tools[i]
		}
	}
	if tool == nil {
		return fmt.Errorf("tool %s has no implementation", action.ToolID)
	}

	var args any
	_ = json.Unmarshal(action.Args, &args)

	if _, err := pool.Exec(ctx, `update actions set status = 'executing', updated_at = now() where id = $1`, action.ID); err != nil {
		return err
	}
	result, execErr := tool.Execute(toolCtx, args)
	if execErr != nil {
		result = ToolResult{OK: false, Error: execErr.Error()}
	}
	if !result.OK {
		return failAction(ctx, pool, action, result)
	}

	verification := Verification{Passed: true, Detail: "no verification required"}
	if data.DryRunVerifyFail {
		verification = Verification{Passed: false, Detail: "forced verification failure (test)"}
	} else if tool.Verify != nil {
		verification, execErr = tool.Verify(toolCtx, args)
		if execErr != nil {
			verification = Verification{Passed: false, Detail: execErr.Error()}
		}
	}

	encodedResult, err := json.Marshal(result)
	if err != nil {
		return err
	}
	encodedVerification, err := json.Marshal(verification)
	if err != nil {
		return err
	}
	if err := auditAction(ctx, pool, action.OrgID, action.AgentID, "action.executed", action.ID, map[string]any{"ok": result.OK}); err != nil {
		return err
	}
	if err := auditAction(ctx, pool, action.OrgID, action.AgentID, "action.verified", action.ID, map[string]any{
		"passed": verification.Passed, "detail": verification.Detail,
	}); err != nil {
		return err
	}
	if verification.Passed {
		if _, err := pool.Exec(ctx,
			`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'agent_ai', $3)`,
			action.OrgID, action.ConversationID, "Done. I have taken care of that for you."); err != nil {
			return err
		}
	} else {
		if _, err := pool.Exec(ctx,
			`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'system', $3)`,
			action.OrgID, action.ConversationID, "The action was executed but could not be verified. A teammate will follow up."); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx,
			`update conversations set status = 'awaiting_human', updated_at = now() where id = $1`, action.ConversationID); err != nil {
			return err
		}
	}
	if _, err := pool.Exec(ctx,
		`update actions set status = 'executed', result = $2::jsonb, verification = $3::jsonb, updated_at = now() where id = $1`,
		action.ID, string(encodedResult), string(encodedVerification)); err != nil {
		return err
	}
	return nil
}
