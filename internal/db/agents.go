// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

type AgentRow struct {
	ID     string
	Name   string
	Policy json.RawMessage
}

func (r *Repo) ActiveAgent(ctx RepoCtx) (AgentRow, error) {
	var agent AgentRow
	err := r.pool.QueryRow(ctx.Ctx, `select id::text, name, policy from agents
		where org_id = $1 and is_active = true order by created_at limit 1`, ctx.OrgID).
		Scan(&agent.ID, &agent.Name, &agent.Policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return agent, ErrNotFound
	}
	return agent, err
}

func (r *Repo) UpdateAgentPolicy(ctx RepoCtx, id string, policy json.RawMessage) error {
	tag, err := r.pool.Exec(ctx.Ctx, `update agents set policy = $3::jsonb, updated_at = now()
		where id = $1 and org_id = $2`, id, ctx.OrgID, string(policy))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type ModelOptionRow struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type ProviderModelsRow struct {
	ProviderID  string
	DisplayName string
	Models      []ModelOptionRow
}

func (r *Repo) ListProviderModels(ctx RepoCtx) ([]ProviderModelsRow, error) {
	rows, err := r.pool.Query(ctx.Ctx, `select provider_id, display_name, coalesce(models_cache, '[]'::jsonb)
		from provider_configs where org_id = $1 order by provider_id`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderModelsRow{}
	for rows.Next() {
		var group ProviderModelsRow
		var cache []byte
		if err := rows.Scan(&group.ProviderID, &group.DisplayName, &cache); err != nil {
			return nil, err
		}
		if len(cache) > 0 {
			_ = json.Unmarshal(cache, &group.Models)
		}
		out = append(out, group)
	}
	return out, rows.Err()
}
