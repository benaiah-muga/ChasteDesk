// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"chastedesk/internal/id"
)

var ErrEmailExists = errors.New("db: email already registered")

type InvitationRow struct {
	ID        string
	Email     string
	Role      string
	Status    string
	ExpiresAt time.Time
}

func (r *Repo) ListPendingInvitations(ctx RepoCtx) ([]InvitationRow, error) {
	rows, err := r.pool.Query(ctx.Ctx, `select id, email, role, status, expires_at
		from invitation where organization_id = $1 and status = 'pending' and expires_at > now()
		order by expires_at asc`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InvitationRow{}
	for rows.Next() {
		var row InvitationRow
		if err := rows.Scan(&row.ID, &row.Email, &row.Role, &row.Status, &row.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repo) CreateInvitation(ctx RepoCtx, email, role, inviterID string, expiresAt time.Time) (string, error) {
	invitationID := id.Token(24)
	_, err := r.pool.Exec(ctx.Ctx, `insert into invitation (id, organization_id, email, role, status, expires_at, inviter_id)
		values ($1, $2, $3, $4, 'pending', $5, $6)`, invitationID, ctx.OrgID, email, role, expiresAt, inviterID)
	return invitationID, err
}

func (r *Repo) RevokeInvitation(ctx RepoCtx, invitationID string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx, `update invitation set status = 'revoked'
		where id = $1 and organization_id = $2 and status = 'pending'`, invitationID, ctx.OrgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) PendingInvitation(ctx context.Context, invitationID string) (InvitationRow, string, error) {
	var row InvitationRow
	var orgName string
	err := r.pool.QueryRow(ctx, `select i.id, i.email, i.role, i.status, i.expires_at, o.name
		from invitation i join organization o on o.id = i.organization_id
		where i.id = $1`, invitationID).Scan(&row.ID, &row.Email, &row.Role, &row.Status, &row.ExpiresAt, &orgName)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, "", ErrNotFound
	}
	if err != nil {
		return row, "", err
	}
	if row.Status != "pending" || row.ExpiresAt.Before(time.Now()) {
		return row, "", ErrNotFound
	}
	return row, orgName, nil
}

func (r *Repo) AcceptInvitation(ctx context.Context, invitationID, name, passwordHash string) (string, string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var (
		orgID     string
		email     string
		role      string
		expiresAt time.Time
		status    string
	)
	err = tx.QueryRow(ctx, `select organization_id, email, role, expires_at, status
		from invitation where id = $1 for update`, invitationID).Scan(&orgID, &email, &role, &expiresAt, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if status != "pending" || expiresAt.Before(time.Now()) {
		return "", "", ErrNotFound
	}
	var existingUserID string
	lookupErr := tx.QueryRow(ctx, `select id from "user" where lower(email) = lower($1)`, email).Scan(&existingUserID)
	userID := existingUserID
	if errors.Is(lookupErr, pgx.ErrNoRows) {
		userID = id.UUID()
		if err := r.CreateUser(ctx, tx, User{ID: userID, Name: name, Email: email}); err != nil {
			return "", "", err
		}
	} else if lookupErr != nil {
		return "", "", lookupErr
	} else {
		var hasCredential bool
		if err := tx.QueryRow(ctx, `select exists(select 1 from account where user_id = $1 and provider_id = 'credential')`, userID).Scan(&hasCredential); err != nil {
			return "", "", err
		}
		if hasCredential {
			return "", "", ErrEmailExists
		}
		if _, err := tx.Exec(ctx, `update "user" set name = $2, updated_at = now() where id = $1`, userID, name); err != nil {
			return "", "", err
		}
	}
	if err := r.CreateCredentialAccount(ctx, tx, id.UUID(), userID, passwordHash); err != nil {
		return "", "", err
	}
	var hasMembership bool
	if err := tx.QueryRow(ctx, `select exists(select 1 from member where organization_id = $1 and user_id = $2)`, orgID, userID).Scan(&hasMembership); err != nil {
		return "", "", err
	}
	if hasMembership {
		if _, err := tx.Exec(ctx, `update member set role = $3 where organization_id = $1 and user_id = $2`, orgID, userID, role); err != nil {
			return "", "", err
		}
	} else if _, err := tx.Exec(ctx, `insert into member (id, organization_id, user_id, role) values ($1, $2, $3, $4)`,
		id.UUID(), orgID, userID, role); err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `update invitation set status = 'accepted' where id = $1`, invitationID); err != nil {
		return "", "", err
	}
	if err := writeAuditQ(ctx, tx, &orgID, AuditInput{
		ActorType: ActorUser, ActorID: &userID, Type: "user.invited",
		TargetType: "user", TargetID: &userID, Payload: map[string]any{"role": role, "accepted": true},
	}); err != nil {
		return "", "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return userID, orgID, nil
}
