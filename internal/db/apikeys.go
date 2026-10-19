// SPDX-License-Identifier: AGPL-3.0-only
package db

import "time"

type APIKeyRow struct {
	ID         string
	Name       string
	KeyPrefix  string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

func (r *Repo) ListAPIKeys(ctx RepoCtx) ([]APIKeyRow, error) {
	rows, err := r.pool.Query(ctx.Ctx, `select id::text, name, key_prefix, created_at, last_used_at
		from api_keys where org_id = $1 order by created_at desc`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKeyRow{}
	for rows.Next() {
		var row APIKeyRow
		if err := rows.Scan(&row.ID, &row.Name, &row.KeyPrefix, &row.CreatedAt, &row.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repo) CreateAPIKey(ctx RepoCtx, name, prefix, hash string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx.Ctx, `insert into api_keys (org_id, name, key_prefix, key_hash)
		values ($1, $2, $3, $4) returning id::text`, ctx.OrgID, name, prefix, hash).Scan(&id)
	return id, err
}

func (r *Repo) DeleteAPIKey(ctx RepoCtx, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx, `delete from api_keys where id = $1 and org_id = $2`, id, ctx.OrgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
