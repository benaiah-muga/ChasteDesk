// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("db: not found")
var ErrOrgExists = errors.New("db: organization already exists")

type User struct {
	ID    string
	Name  string
	Email string
}

type Credential struct {
	User         User
	PasswordHash string
}

func (r *Repo) CreateUser(ctx context.Context, q Queryer, user User) error {
	_, err := q.Exec(ctx, `insert into "user" (id, name, email) values ($1, $2, $3)`, user.ID, user.Name, user.Email)
	return err
}

func (r *Repo) CreateCredentialAccount(ctx context.Context, q Queryer, accountID, userID, passwordHash string) error {
	_, err := q.Exec(ctx, `insert into account (id, account_id, provider_id, issuer, user_id, password)
		values ($1, $2, 'credential', 'credential', $3, $4)`, accountID, userID, userID, passwordHash)
	return err
}

func (r *Repo) CredentialByEmail(ctx context.Context, email string) (Credential, error) {
	var cred Credential
	var password *string
	err := r.pool.QueryRow(ctx, `select u.id, u.name, u.email, a.password
		from "user" u join account a on a.user_id = u.id and a.provider_id = 'credential'
		where lower(u.email) = lower($1) limit 1`, email).Scan(&cred.User.ID, &cred.User.Name, &cred.User.Email, &password)
	if errors.Is(err, pgx.ErrNoRows) || password == nil {
		return Credential{}, ErrNotFound
	}
	if err != nil {
		return Credential{}, err
	}
	cred.PasswordHash = *password
	return cred, nil
}

func (r *Repo) CreateSession(ctx context.Context, q Queryer, sessionID, tokenHash, userID string, ip, userAgent *string, expiresAt time.Time) error {
	_, err := q.Exec(ctx, `insert into session (id, token, user_id, ip_address, user_agent, expires_at)
		values ($1, $2, $3, $4, $5, $6)`, sessionID, tokenHash, userID, ip, userAgent, expiresAt)
	return err
}

type SessionInfo struct {
	SessionID string
	UserID    string
	Name      string
	Email     string
	ExpiresAt time.Time
}

func (r *Repo) SessionByTokenHash(ctx context.Context, tokenHash string) (SessionInfo, error) {
	var info SessionInfo
	err := r.pool.QueryRow(ctx, `select s.id, s.user_id, u.name, u.email, s.expires_at
		from session s join "user" u on u.id = s.user_id
		where s.token = $1 and s.expires_at > now() limit 1`, tokenHash).
		Scan(&info.SessionID, &info.UserID, &info.Name, &info.Email, &info.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionInfo{}, ErrNotFound
	}
	if err != nil {
		return SessionInfo{}, err
	}
	return info, nil
}

func (r *Repo) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `delete from session where token = $1`, tokenHash)
	return err
}

func (r *Repo) MembershipForUser(ctx context.Context, userID string) (string, string, error) {
	var orgID, role string
	err := r.pool.QueryRow(ctx, `select organization_id, role from member where user_id = $1 limit 1`, userID).
		Scan(&orgID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	return orgID, role, nil
}

func (r *Repo) OrgExists(ctx context.Context) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `select exists(select 1 from organization)`).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repo) UpdateUserName(ctx context.Context, userID, name string) error {
	_, err := r.pool.Exec(ctx, `update "user" set name = $2, updated_at = now() where id = $1`, userID, name)
	return err
}

func (r *Repo) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	tag, err := r.pool.Exec(ctx, `update account set password = $2, updated_at = now()
		where user_id = $1 and provider_id = 'credential'`, userID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) DeleteOtherSessions(ctx context.Context, userID, keepTokenHash string) error {
	_, err := r.pool.Exec(ctx, `delete from session where user_id = $1 and token <> $2`, userID, keepTokenHash)
	return err
}
