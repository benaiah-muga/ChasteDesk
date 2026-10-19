// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"encoding/json"
	"time"
)

type ProviderRow struct {
	ID          string  `json:"id"`
	ProviderID  string  `json:"providerId"`
	DisplayName string  `json:"displayName"`
	Enabled     bool    `json:"enabled"`
	KeyLast4    *string `json:"keyLast4"`
	BaseURL     *string `json:"baseUrl"`
}

func (r *Repo) ListProviders(ctx RepoCtx) ([]ProviderRow, error) {
	rows, err := r.pool.Query(ctx.Ctx, `select id::text, provider_id, display_name, enabled, key_last4, base_url
		from provider_configs where org_id = $1 order by provider_id`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderRow{}
	for rows.Next() {
		var p ProviderRow
		if err := rows.Scan(&p.ID, &p.ProviderID, &p.DisplayName, &p.Enabled, &p.KeyLast4, &p.BaseURL); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type AuditRow struct {
	ID         int64           `json:"id"`
	Type       string          `json:"type"`
	ActorType  string          `json:"actorType"`
	ActorID    *string         `json:"actorId"`
	TargetType string          `json:"targetType"`
	TargetID   *string         `json:"targetId"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"createdAt"`
}

func (r *Repo) ListAuditEvents(ctx RepoCtx, typeFilter string, limit int) ([]AuditRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx.Ctx, `select id, type, actor_type, actor_id, target_type, target_id, payload, created_at
		from audit_events
		where org_id = $1 and ($2 = '' or type = $2)
		order by id desc limit $3`, ctx.OrgID, typeFilter, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditRow{}
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.ID, &a.Type, &a.ActorType, &a.ActorID, &a.TargetType, &a.TargetID, &a.Payload, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
