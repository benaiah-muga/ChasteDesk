// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"chastedesk/internal/id"
)

const bootstrapAdvisoryLock = 747321

const DefaultAgentPolicy = `{
  "purpose": "Answer customer questions from the connected knowledge base and escalate anything sensitive to a human.",
  "knowledgeScopes": ["public"],
  "tools": [
    {"toolId": "knowledge.search", "mode": "autonomous"},
    {"toolId": "customers.lookup", "mode": "autonomous"},
    {"toolId": "conversations.add_note", "mode": "autonomous"},
    {"toolId": "escalate", "mode": "autonomous"}
  ],
  "escalation": {
    "lowConfidenceThreshold": 0.35,
    "keywords": ["refund", "cancel my account", "legal", "complaint"],
    "alwaysEscalateTopics": []
  },
  "model": {"providerId": "", "modelId": ""},
  "budgets": {"maxSteps": 6, "maxToolCalls": 4, "maxInputTokens": 8000, "maxOutputTokens": 2000}
}`

type BootstrapInput struct {
	OrgName      string
	UserName     string
	Email        string
	PasswordHash string
}

func (r *Repo) Bootstrap(ctx context.Context, in BootstrapInput) (orgID, userID string, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock($1)`, bootstrapAdvisoryLock); err != nil {
		return "", "", err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `select exists(select 1 from organization)`).Scan(&exists); err != nil {
		return "", "", err
	}
	if exists {
		return "", "", ErrOrgExists
	}
	userID = id.UUID()
	orgID = id.UUID()
	if err := r.CreateUser(ctx, tx, User{ID: userID, Name: in.UserName, Email: in.Email}); err != nil {
		return "", "", err
	}
	if err := r.CreateCredentialAccount(ctx, tx, id.UUID(), userID, in.PasswordHash); err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `insert into organization (id, name, slug) values ($1, $2, 'main')`, orgID, in.OrgName); err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `insert into member (id, organization_id, user_id, role) values ($1, $2, $3, 'owner')`, id.UUID(), orgID, userID); err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `insert into agents (org_id, name, policy, is_active)
		values ($1, 'Default support agent', $2::jsonb, true)`, orgID, DefaultAgentPolicy); err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `insert into widget_configs (org_id, origins, theme) values ($1, '{}', '{}'::jsonb)`, orgID); err != nil {
		return "", "", err
	}
	if err := writeAuditQ(ctx, tx, &orgID, AuditInput{
		ActorType:  ActorUser,
		ActorID:    &userID,
		Type:       "org.created",
		TargetType: "organization",
		TargetID:   &orgID,
		Payload:    map[string]any{},
	}); err != nil {
		return "", "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return orgID, userID, nil
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
