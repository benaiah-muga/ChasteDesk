// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type KnowledgeSourceRow struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	Name           string    `json:"name"`
	Scope          string    `json:"scope"`
	Status         string    `json:"status"`
	Version        int       `json:"version"`
	Error          *string   `json:"error"`
	EmbeddingModel *string   `json:"embeddingModel"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type KnowledgeSourceDetail struct {
	KnowledgeSourceRow
	SourceConfig json.RawMessage `json:"sourceConfig"`
}

func (r *Repo) ListKnowledgeSources(ctx RepoCtx) ([]KnowledgeSourceRow, error) {
	rows, err := r.pool.Query(ctx.Ctx, `select id::text, type::text, name, scope::text, status::text, version, error, embedding_model, updated_at
		from knowledge_sources where org_id = $1 order by updated_at desc`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KnowledgeSourceRow{}
	for rows.Next() {
		var row KnowledgeSourceRow
		if err := rows.Scan(&row.ID, &row.Type, &row.Name, &row.Scope, &row.Status, &row.Version, &row.Error, &row.EmbeddingModel, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repo) GetKnowledgeSource(ctx RepoCtx, id string) (KnowledgeSourceDetail, error) {
	detail := KnowledgeSourceDetail{}
	row := r.pool.QueryRow(ctx.Ctx, `select id::text, type::text, name, scope::text, status::text, version, error, embedding_model, updated_at, source_config
		from knowledge_sources where id = $1 and org_id = $2`, id, ctx.OrgID)
	err := row.Scan(&detail.ID, &detail.Type, &detail.Name, &detail.Scope, &detail.Status, &detail.Version, &detail.Error, &detail.EmbeddingModel, &detail.UpdatedAt, &detail.SourceConfig)
	if errors.Is(err, pgx.ErrNoRows) {
		return detail, ErrNotFound
	}
	return detail, err
}

func (r *Repo) CreateKnowledgeSource(ctx RepoCtx, sourceType, name, scope string, config map[string]any) (string, error) {
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	var id string
	err = r.pool.QueryRow(ctx.Ctx, `insert into knowledge_sources (org_id, type, name, scope, status, source_config)
		values ($1, $2, $3, $4, 'pending', $5::jsonb) returning id::text`, ctx.OrgID, sourceType, name, scope, string(encoded)).Scan(&id)
	return id, err
}

func (r *Repo) UpdateKnowledgeSourceScope(ctx RepoCtx, id, scope string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx, `update knowledge_sources set scope = $3, updated_at = now() where id = $1 and org_id = $2`, id, ctx.OrgID, scope)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) DeleteKnowledgeSource(ctx RepoCtx, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx, `delete from knowledge_sources where id = $1 and org_id = $2`, id, ctx.OrgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) SetKnowledgeSourcePending(ctx RepoCtx, id string) error {
	_, err := r.pool.Exec(ctx.Ctx, `update knowledge_sources set status = 'pending', updated_at = now() where id = $1 and org_id = $2`, id, ctx.OrgID)
	return err
}
