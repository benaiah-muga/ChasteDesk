// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"chastedesk/internal/id"
)

type SCIMTokenRow struct {
	ID         string
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

func (r *Repo) CreateSCIMToken(ctx RepoCtx, name, tokenHash, createdBy string) (string, error) {
	tokenID := id.UUID()
	_, err := r.pool.Exec(ctx.Ctx,
		`insert into scim_tokens (id, org_id, name, token_hash, created_by) values ($1, $2, $3, $4, $5)`,
		tokenID, ctx.OrgID, name, tokenHash, createdBy)
	return tokenID, err
}

func (r *Repo) ListSCIMTokens(ctx RepoCtx) ([]SCIMTokenRow, error) {
	rows, err := r.pool.Query(ctx.Ctx,
		`select id::text, name, created_at, last_used_at from scim_tokens
		 where org_id = $1 and revoked_at is null order by created_at desc`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SCIMTokenRow{}
	for rows.Next() {
		var row SCIMTokenRow
		if err := rows.Scan(&row.ID, &row.Name, &row.CreatedAt, &row.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repo) RevokeSCIMToken(ctx RepoCtx, tokenID string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx,
		`update scim_tokens set revoked_at = now() where id = $1 and org_id = $2 and revoked_at is null`, tokenID, ctx.OrgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) SCIMOrgForToken(ctx context.Context, tokenHash string) (string, string, string, error) {
	var orgID, tokenID, createdBy string
	err := r.pool.QueryRow(ctx,
		`select org_id, id::text, created_by from scim_tokens where token_hash = $1 and revoked_at is null`, tokenHash).Scan(&orgID, &tokenID, &createdBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	return orgID, tokenID, createdBy, err
}

func (r *Repo) TouchSCIMToken(ctx context.Context, tokenID string) {
	_, _ = r.pool.Exec(ctx, `update scim_tokens set last_used_at = now() where id = $1`, tokenID)
}

type SCIMUserRow struct {
	ID           string
	UserID       string
	ExternalID   *string
	UserName     string
	DisplayName  string
	Active       bool
	Role         string
	InvitationID *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (r *Repo) ListSCIMUsers(ctx RepoCtx, start, count int) ([]SCIMUserRow, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx.Ctx, `select count(*)::int from scim_users where org_id = $1`, ctx.OrgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx.Ctx,
		`select id::text, user_id, external_id, user_name, display_name, active, role, invitation_id, created_at, updated_at
		 from scim_users where org_id = $1 order by created_at asc limit $2 offset $3`, ctx.OrgID, count, start)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []SCIMUserRow{}
	for rows.Next() {
		var row SCIMUserRow
		if err := rows.Scan(&row.ID, &row.UserID, &row.ExternalID, &row.UserName, &row.DisplayName, &row.Active, &row.Role, &row.InvitationID, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

func (r *Repo) SCIMUserByID(ctx RepoCtx, scimID string) (SCIMUserRow, error) {
	return r.scimUser(ctx, `id = $2`, scimID)
}

func (r *Repo) SCIMUserByUserName(ctx RepoCtx, userName string) (SCIMUserRow, error) {
	return r.scimUser(ctx, `user_name = $2`, strings.ToLower(strings.TrimSpace(userName)))
}

func (r *Repo) scimUser(ctx RepoCtx, predicate, value string) (SCIMUserRow, error) {
	var row SCIMUserRow
	err := r.pool.QueryRow(ctx.Ctx,
		`select id::text, user_id, external_id, user_name, display_name, active, role, invitation_id, created_at, updated_at
		 from scim_users where org_id = $1 and `+predicate, ctx.OrgID, value).
		Scan(&row.ID, &row.UserID, &row.ExternalID, &row.UserName, &row.DisplayName, &row.Active, &row.Role, &row.InvitationID, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

func (r *Repo) CreateSCIMUser(ctx RepoCtx, userID, userName, displayName, role, externalID, inviterID string) (SCIMUserRow, error) {
	userName = strings.ToLower(strings.TrimSpace(userName))
	var emailExists bool
	if err := r.pool.QueryRow(ctx.Ctx, `select exists(select 1 from "user" where lower(email) = $1)`, userName).Scan(&emailExists); err != nil {
		return SCIMUserRow{}, err
	}
	if emailExists {
		return SCIMUserRow{}, ErrEmailExists
	}
	invitationExpiry := time.Now().Add(7 * 24 * time.Hour)
	invitationID := id.Token(24)
	scimID := id.UUID()
	tx, err := r.pool.Begin(ctx.Ctx)
	if err != nil {
		return SCIMUserRow{}, err
	}
	defer func() { _ = tx.Rollback(ctx.Ctx) }()
	if _, err := tx.Exec(ctx.Ctx, `insert into "user" (id, name, email) values ($1, $2, $3)`, userID, displayName, userName); err != nil {
		return SCIMUserRow{}, err
	}
	if _, err := tx.Exec(ctx.Ctx, `insert into member (id, organization_id, user_id, role) values ($1, $2, $3, $4)`, id.UUID(), ctx.OrgID, userID, role); err != nil {
		return SCIMUserRow{}, err
	}
	if _, err := tx.Exec(ctx.Ctx,
		`insert into invitation (id, organization_id, email, role, status, expires_at, inviter_id) values ($1, $2, $3, $4, 'pending', $5, $6)`,
		invitationID, ctx.OrgID, userName, role, invitationExpiry, inviterID); err != nil {
		return SCIMUserRow{}, err
	}
	var external any
	if externalID != "" {
		external = externalID
	}
	if _, err := tx.Exec(ctx.Ctx,
		`insert into scim_users (id, org_id, user_id, external_id, user_name, display_name, active, role, invitation_id)
		 values ($1, $2, $3, $4, $5, $6, true, $7, $8)`,
		scimID, ctx.OrgID, userID, external, userName, displayName, role, invitationID); err != nil {
		return SCIMUserRow{}, err
	}
	if err := tx.Commit(ctx.Ctx); err != nil {
		return SCIMUserRow{}, err
	}
	return r.SCIMUserByID(ctx, scimID)
}

type SCIMUserUpdate struct {
	UserName    string
	DisplayName string
	Role        string
	Active      bool
	ExternalID  string
	InviterID   string
}

func (r *Repo) UpdateSCIMUser(ctx RepoCtx, scimID string, update SCIMUserUpdate) (SCIMUserRow, error) {
	userName := strings.ToLower(strings.TrimSpace(update.UserName))
	tx, err := r.pool.Begin(ctx.Ctx)
	if err != nil {
		return SCIMUserRow{}, err
	}
	defer func() { _ = tx.Rollback(ctx.Ctx) }()
	var row SCIMUserRow
	if err := tx.QueryRow(ctx.Ctx,
		`select id::text, user_id, external_id, user_name, display_name, active, role, invitation_id, created_at, updated_at
		 from scim_users where org_id = $1 and id = $2 for update`, ctx.OrgID, scimID).
		Scan(&row.ID, &row.UserID, &row.ExternalID, &row.UserName, &row.DisplayName, &row.Active, &row.Role, &row.InvitationID, &row.CreatedAt, &row.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SCIMUserRow{}, ErrNotFound
		}
		return SCIMUserRow{}, err
	}
	var emailExists bool
	if err := tx.QueryRow(ctx.Ctx, `select exists(select 1 from "user" where lower(email) = $1 and id <> $2)`, userName, row.UserID).Scan(&emailExists); err != nil {
		return SCIMUserRow{}, err
	}
	if emailExists {
		return SCIMUserRow{}, ErrEmailExists
	}
	if _, err := tx.Exec(ctx.Ctx, `update "user" set name = $2, email = $3, updated_at = now() where id = $1`, row.UserID, update.DisplayName, userName); err != nil {
		return SCIMUserRow{}, err
	}
	if userName != row.UserName {
		if _, err := tx.Exec(ctx.Ctx, `delete from session where user_id = $1`, row.UserID); err != nil {
			return SCIMUserRow{}, err
		}
		if _, err := tx.Exec(ctx.Ctx, `update invitation set status = 'revoked' where organization_id = $1 and lower(email) = $2 and status = 'pending'`, ctx.OrgID, row.UserName); err != nil {
			return SCIMUserRow{}, err
		}
	}
	if update.Active {
		var exists bool
		if err := tx.QueryRow(ctx.Ctx, `select exists(select 1 from member where organization_id = $1 and user_id = $2)`, ctx.OrgID, row.UserID).Scan(&exists); err != nil {
			return SCIMUserRow{}, err
		}
		if exists {
			if _, err := tx.Exec(ctx.Ctx, `update member set role = $3 where organization_id = $1 and user_id = $2`, ctx.OrgID, row.UserID, update.Role); err != nil {
				return SCIMUserRow{}, err
			}
		} else if _, err := tx.Exec(ctx.Ctx, `insert into member (id, organization_id, user_id, role) values ($1, $2, $3, $4)`, id.UUID(), ctx.OrgID, row.UserID, update.Role); err != nil {
			return SCIMUserRow{}, err
		}
		var hasCredential bool
		if err := tx.QueryRow(ctx.Ctx, `select exists(select 1 from account where user_id = $1 and provider_id = 'credential')`, row.UserID).Scan(&hasCredential); err != nil {
			return SCIMUserRow{}, err
		}
		var pendingInvitation string
		_ = tx.QueryRow(ctx.Ctx, `select id from invitation where organization_id = $1 and lower(email) = $2 and status = 'pending' and expires_at > now() order by expires_at desc limit 1`, ctx.OrgID, userName).Scan(&pendingInvitation)
		if !hasCredential && pendingInvitation == "" {
			invitationID := id.Token(24)
			if _, err := tx.Exec(ctx.Ctx,
				`insert into invitation (id, organization_id, email, role, status, expires_at, inviter_id) values ($1, $2, $3, $4, 'pending', $5, $6)`,
				invitationID, ctx.OrgID, userName, update.Role, time.Now().Add(7*24*time.Hour), update.InviterID); err != nil {
				return SCIMUserRow{}, err
			}
			row.InvitationID = &invitationID
		}
	} else {
		if _, err := tx.Exec(ctx.Ctx, `delete from member where organization_id = $1 and user_id = $2`, ctx.OrgID, row.UserID); err != nil {
			return SCIMUserRow{}, err
		}
		if _, err := tx.Exec(ctx.Ctx, `delete from session where user_id = $1`, row.UserID); err != nil {
			return SCIMUserRow{}, err
		}
		if _, err := tx.Exec(ctx.Ctx, `update invitation set status = 'revoked' where organization_id = $1 and lower(email) = $2 and status = 'pending'`, ctx.OrgID, userName); err != nil {
			return SCIMUserRow{}, err
		}
	}
	var external any
	if update.ExternalID != "" {
		external = update.ExternalID
	}
	if _, err := tx.Exec(ctx.Ctx,
		`update scim_users set external_id = $3, user_name = $4, display_name = $5, active = $6, role = $7, updated_at = now() where org_id = $1 and id = $2`,
		ctx.OrgID, scimID, external, userName, update.DisplayName, update.Active, update.Role); err != nil {
		return SCIMUserRow{}, err
	}
	if err := tx.Commit(ctx.Ctx); err != nil {
		return SCIMUserRow{}, err
	}
	return r.SCIMUserByID(ctx, scimID)
}

func (r *Repo) SCIMUserByExternalID(ctx RepoCtx, externalID string) (SCIMUserRow, error) {
	var row SCIMUserRow
	err := r.pool.QueryRow(ctx.Ctx,
		`select id::text, user_id, external_id, user_name, display_name, active, role, invitation_id, created_at, updated_at
		 from scim_users where org_id = $1 and external_id = $2`, ctx.OrgID, externalID).
		Scan(&row.ID, &row.UserID, &row.ExternalID, &row.UserName, &row.DisplayName, &row.Active, &row.Role, &row.InvitationID, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}
