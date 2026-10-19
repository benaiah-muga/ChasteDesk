// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type ActionRow struct {
	ID             string          `json:"id"`
	OrgID          string          `json:"orgId"`
	RunID          string          `json:"runId"`
	ConversationID string          `json:"conversationId"`
	AgentID        string          `json:"agentId"`
	ToolID         string          `json:"toolId"`
	Args           json.RawMessage `json:"args"`
	Status         string          `json:"status"`
	Result         json.RawMessage `json:"result,omitempty"`
	Verification   json.RawMessage `json:"verification,omitempty"`
	DecidedBy      *string         `json:"decidedBy,omitempty"`
	DecidedAt      *time.Time      `json:"decidedAt,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
}

func scanActionRow(row pgx.Row) (ActionRow, error) {
	var action ActionRow
	err := row.Scan(&action.ID, &action.OrgID, &action.RunID, &action.ConversationID, &action.AgentID,
		&action.ToolID, &action.Args, &action.Status, &action.Result, &action.Verification,
		&action.DecidedBy, &action.DecidedAt, &action.CreatedAt)
	return action, err
}

const actionColumns = `id, org_id, run_id, conversation_id, agent_id, tool_id, args, status, result, verification, decided_by, decided_at, created_at`

func (r *Repo) ListActions(ctx RepoCtx, status string) ([]ActionRow, error) {
	query := `select ` + actionColumns + ` from actions where org_id = $1`
	args := []any{ctx.OrgID}
	if status != "" {
		query += ` and status = $2`
		args = append(args, status)
	}
	query += ` order by created_at desc limit 100`
	rows, err := r.pool.Query(ctx.Ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActionRow{}
	for rows.Next() {
		action, err := scanActionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, action)
	}
	return out, rows.Err()
}

func (r *Repo) GetAction(ctx RepoCtx, id string) (ActionRow, error) {
	action, err := scanActionRow(r.pool.QueryRow(ctx.Ctx,
		`select `+actionColumns+` from actions where id = $1 and org_id = $2`, id, ctx.OrgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ActionRow{}, ErrNotFound
	}
	return action, err
}

func (r *Repo) DecideAction(ctx RepoCtx, id, status, decidedBy string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx,
		`update actions set status = $3, decided_by = $4, decided_at = now(), updated_at = now()
		  where id = $1 and org_id = $2 and status = 'pending_approval'`,
		id, ctx.OrgID, status, decidedBy)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

type ApprovalRow struct {
	ID             string
	ConversationID string
	ToolID         string
	Args           json.RawMessage
	Status         string
	CreatedAt      time.Time
	DecidedAt      *time.Time
	CustomerName   *string
	CustomerEmail  *string
}

func (r *Repo) ListApprovals(ctx RepoCtx, status string, limit int) ([]ApprovalRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	query := `select a.id::text, a.conversation_id::text, a.tool_id, a.args, a.status::text, a.created_at, a.decided_at,
		cu.name, cu.email
		from actions a
		join conversations c on c.id = a.conversation_id and c.org_id = a.org_id
		join customers cu on cu.id = c.customer_id
		where a.org_id = $1`
	args := []any{ctx.OrgID}
	if status == "pending_approval" {
		query += ` and a.status = 'pending_approval' order by a.created_at asc limit $2`
	} else {
		query += ` and a.status <> 'pending_approval' order by coalesce(a.decided_at, a.updated_at) desc limit $2`
	}
	args = append(args, limit)
	rows, err := r.pool.Query(ctx.Ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ApprovalRow{}
	for rows.Next() {
		var row ApprovalRow
		if err := rows.Scan(&row.ID, &row.ConversationID, &row.ToolID, &row.Args, &row.Status, &row.CreatedAt, &row.DecidedAt, &row.CustomerName, &row.CustomerEmail); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
