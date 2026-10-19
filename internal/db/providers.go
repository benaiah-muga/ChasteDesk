// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"chastedesk/internal/crypto"
)

type ProviderRow struct {
	ID          string  `json:"id"`
	ProviderID  string  `json:"providerId"`
	DisplayName string  `json:"displayName"`
	Enabled     bool    `json:"enabled"`
	KeyLast4    *string `json:"keyLast4"`
	BaseURL     *string `json:"baseUrl"`
	ModelCount  int     `json:"modelCount"`
}

type ProviderConfig struct {
	ID          string
	ProviderID  string
	DisplayName string
	Enabled     bool
	BaseURL     *string
	KeyCipher   *string
	KeyIV       *string
	KeyTag      *string
	KeyVersion  *int
	KeyLast4    *string
}

type ProviderUpsert struct {
	ProviderID  string
	DisplayName string
	BaseURL     *string
	Sealed      *crypto.SealedSecret
}

func (r *Repo) UpsertProviderConfig(ctx RepoCtx, in ProviderUpsert) error {
	var existing string
	err := r.pool.QueryRow(ctx.Ctx, `select id::text from provider_configs where org_id = $1 and provider_id = $2`, ctx.OrgID, in.ProviderID).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		if in.Sealed != nil {
			_, err = r.pool.Exec(ctx.Ctx, `insert into provider_configs (org_id, provider_id, display_name, base_url, key_cipher, key_iv, key_tag, key_version, key_last4, enabled)
				values ($1, $2, $3, $4, $5, $6, $7, $8, $9, true)`,
				ctx.OrgID, in.ProviderID, in.DisplayName, in.BaseURL,
				in.Sealed.Ciphertext, in.Sealed.IV, in.Sealed.AuthTag, in.Sealed.KeyVersion, in.Sealed.Last4)
		} else {
			_, err = r.pool.Exec(ctx.Ctx, `insert into provider_configs (org_id, provider_id, display_name, base_url, enabled)
				values ($1, $2, $3, $4, true)`, ctx.OrgID, in.ProviderID, in.DisplayName, in.BaseURL)
		}
		return err
	}
	if err != nil {
		return err
	}
	if in.Sealed != nil {
		_, err = r.pool.Exec(ctx.Ctx, `update provider_configs set display_name = $3, base_url = $4,
			key_cipher = $5, key_iv = $6, key_tag = $7, key_version = $8, key_last4 = $9, enabled = true, updated_at = now()
			where id = $1 and org_id = $2`,
			existing, ctx.OrgID, in.DisplayName, in.BaseURL,
			in.Sealed.Ciphertext, in.Sealed.IV, in.Sealed.AuthTag, in.Sealed.KeyVersion, in.Sealed.Last4)
	} else {
		_, err = r.pool.Exec(ctx.Ctx, `update provider_configs set display_name = $3, base_url = $4, enabled = true, updated_at = now()
			where id = $1 and org_id = $2`, existing, ctx.OrgID, in.DisplayName, in.BaseURL)
	}
	return err
}

func (r *Repo) GetProviderConfigByID(ctx RepoCtx, id string) (ProviderConfig, error) {
	cfg := ProviderConfig{}
	err := r.pool.QueryRow(ctx.Ctx, `select id::text, provider_id, display_name, enabled, base_url, key_cipher, key_iv, key_tag, key_version, key_last4
		from provider_configs where id = $1 and org_id = $2`, id, ctx.OrgID).
		Scan(&cfg.ID, &cfg.ProviderID, &cfg.DisplayName, &cfg.Enabled, &cfg.BaseURL, &cfg.KeyCipher, &cfg.KeyIV, &cfg.KeyTag, &cfg.KeyVersion, &cfg.KeyLast4)
	if errors.Is(err, pgx.ErrNoRows) {
		return cfg, ErrNotFound
	}
	return cfg, err
}

func (r *Repo) UpdateModelsCache(ctx RepoCtx, id string, models json.RawMessage) error {
	if models == nil {
		models = json.RawMessage(`[]`)
	}
	_, err := r.pool.Exec(ctx.Ctx, `update provider_configs set models_cache = $3::jsonb, updated_at = now() where id = $1 and org_id = $2`, id, ctx.OrgID, string(models))
	return err
}

func (r *Repo) DeleteProviderConfig(ctx RepoCtx, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx, `delete from provider_configs where id = $1 and org_id = $2`, id, ctx.OrgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) ListProviders(ctx RepoCtx) ([]ProviderRow, error) {
	rows, err := r.pool.Query(ctx.Ctx, `select id::text, provider_id, display_name, enabled, key_last4, base_url,
		jsonb_array_length(coalesce(models_cache, '[]'::jsonb))
		from provider_configs where org_id = $1 order by provider_id`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderRow{}
	for rows.Next() {
		var p ProviderRow
		if err := rows.Scan(&p.ID, &p.ProviderID, &p.DisplayName, &p.Enabled, &p.KeyLast4, &p.BaseURL, &p.ModelCount); err != nil {
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

func (r *Repo) ListAuditEvents(ctx RepoCtx, typeFilter, q string, limit int) ([]AuditRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx.Ctx, `select id, type, actor_type, actor_id, target_type, target_id, payload, created_at
		from audit_events
		where org_id = $1 and ($2 = '' or type = $2)
		  and ($3 = '' or type ilike '%' || $3 || '%' or target_type ilike '%' || $3 || '%')
		order by id desc limit $4`, ctx.OrgID, typeFilter, q, limit)
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

func (r *Repo) ListAuditEventsFiltered(ctx RepoCtx, typeFilter, actor, q string, beforeID int64, limit int) ([]AuditRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 5000 {
		limit = 5000
	}
	rows, err := r.pool.Query(ctx.Ctx, `select id, type, actor_type, actor_id, target_type, target_id, payload, created_at
		from audit_events
		where org_id = $1
		  and ($2 = '' or type = $2)
		  and ($3 = '' or actor_type::text = $3 or actor_id ilike '%' || $3 || '%')
		  and ($4 = '' or type ilike '%' || $4 || '%' or target_type ilike '%' || $4 || '%' or target_id ilike '%' || $4 || '%')
		  and ($5 = 0 or id < $5)
		order by id desc limit $6`, ctx.OrgID, typeFilter, actor, q, beforeID, limit)
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

func (r *Repo) AuditTypes(ctx RepoCtx) ([]string, error) {
	rows, err := r.pool.Query(ctx.Ctx, `select distinct type from audit_events where org_id = $1 order by type`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var eventType string
		if err := rows.Scan(&eventType); err != nil {
			return nil, err
		}
		out = append(out, eventType)
	}
	return out, rows.Err()
}
