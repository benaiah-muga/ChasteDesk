// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorAgent  ActorType = "agent"
	ActorSystem ActorType = "system"
)

type RepoCtx struct {
	Ctx   context.Context
	OrgID string
}

func WithOrg(ctx context.Context, orgID string) RepoCtx {
	return RepoCtx{Ctx: ctx, OrgID: orgID}
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) Pool() *pgxpool.Pool {
	return r.pool
}

type AuditInput struct {
	ActorType  ActorType
	ActorID    *string
	Type       string
	TargetType string
	TargetID   *string
	Payload    map[string]any
}

func truncateRedacted(value any) string {
	if value == nil {
		return "null"
	}
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	s := string(b)
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

func redactPayload(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload))
	for k, v := range payload {
		switch k {
		case "content", "args", "result":
			out[k] = truncateRedacted(v)
		default:
			out[k] = v
		}
	}
	return out
}

func (r *Repo) WriteAudit(ctx RepoCtx, in AuditInput) error {
	orgID := ctx.OrgID
	return r.WriteAuditNoOrg(ctx.Ctx, &orgID, in)
}

func (r *Repo) WriteAuditNoOrg(ctx context.Context, orgID *string, in AuditInput) error {
	return writeAuditQ(ctx, r.pool, orgID, in)
}

func writeAuditQ(ctx context.Context, q Queryer, orgID *string, in AuditInput) error {
	payload, err := json.Marshal(redactPayload(in.Payload))
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `insert into audit_events (org_id, actor_type, actor_id, type, target_type, target_id, payload)
		values ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
		orgID, string(in.ActorType), in.ActorID, in.Type, in.TargetType, in.TargetID, string(payload))
	return err
}
