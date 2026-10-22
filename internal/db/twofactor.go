// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"chastedesk/internal/crypto"
	"chastedesk/internal/id"
)

type TwoFactorRecord struct {
	Sealed         crypto.SealedSecret
	Enabled        bool
	RecoveryHashes []string
	ConfirmedAt    *time.Time
}

func (r *Repo) TwoFactor(ctx RepoCtx, userID string) (TwoFactorRecord, error) {
	var (
		record   TwoFactorRecord
		codesRaw []byte
	)
	err := r.pool.QueryRow(ctx.Ctx,
		`select secret_cipher, secret_iv, secret_tag, key_version, enabled, recovery_codes, confirmed_at
		   from user_two_factor where user_id = $1`, userID).
		Scan(&record.Sealed.Ciphertext, &record.Sealed.IV, &record.Sealed.AuthTag, &record.Sealed.KeyVersion, &record.Enabled, &codesRaw, &record.ConfirmedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return record, ErrNotFound
	}
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(codesRaw, &record.RecoveryHashes); err != nil {
		return record, err
	}
	return record, nil
}

func (r *Repo) BeginTwoFactor(ctx RepoCtx, userID string, sealed crypto.SealedSecret) error {
	_, err := r.pool.Exec(ctx.Ctx,
		`insert into user_two_factor (user_id, secret_cipher, secret_iv, secret_tag, key_version, enabled, recovery_codes)
		 values ($1, $2, $3, $4, $5, false, '[]'::jsonb)
		 on conflict (user_id) do update set
		   secret_cipher = excluded.secret_cipher,
		   secret_iv = excluded.secret_iv,
		   secret_tag = excluded.secret_tag,
		   key_version = excluded.key_version,
		   enabled = false,
		   recovery_codes = '[]'::jsonb,
		   confirmed_at = null,
		   updated_at = now()
		 where user_two_factor.enabled = false`,
		userID, sealed.Ciphertext, sealed.IV, sealed.AuthTag, sealed.KeyVersion)
	return err
}

func (r *Repo) EnableTwoFactor(ctx RepoCtx, userID string, recoveryHashes []string) (bool, error) {
	encoded, err := json.Marshal(recoveryHashes)
	if err != nil {
		return false, err
	}
	tag, err := r.pool.Exec(ctx.Ctx,
		`update user_two_factor set enabled = true, recovery_codes = $2::jsonb, confirmed_at = now(), updated_at = now()
		 where user_id = $1 and enabled = false`, userID, string(encoded))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) DisableTwoFactor(ctx RepoCtx, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx, `delete from user_two_factor where user_id = $1 and enabled = true`, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) ConsumeRecoveryCode(ctx RepoCtx, userID, hash string) (bool, error) {
	tx, err := r.pool.Begin(ctx.Ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx.Ctx) }()
	var raw []byte
	if err := tx.QueryRow(ctx.Ctx,
		`select recovery_codes from user_two_factor where user_id = $1 and enabled = true for update`, userID).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, err
	}
	var hashes []string
	if err := json.Unmarshal(raw, &hashes); err != nil {
		return false, err
	}
	found := -1
	for index, candidate := range hashes {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(hash)) == 1 {
			found = index
			break
		}
	}
	if found < 0 {
		return false, nil
	}
	hashes = append(hashes[:found], hashes[found+1:]...)
	encoded, err := json.Marshal(hashes)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx.Ctx, `update user_two_factor set recovery_codes = $2::jsonb, updated_at = now() where user_id = $1`, userID, string(encoded)); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx.Ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repo) CreateLoginChallenge(ctx RepoCtx, userID string, ttl time.Duration) (string, error) {
	challengeID := id.UUID()
	_, err := r.pool.Exec(ctx.Ctx,
		`insert into login_challenges (id, user_id, expires_at) values ($1, $2, now() + make_interval(secs => $3))`,
		challengeID, userID, int(ttl.Seconds()))
	return challengeID, err
}

type LoginChallenge struct {
	UserID    string
	Attempts  int
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func (r *Repo) LoginChallenge(ctx RepoCtx, challengeID string) (LoginChallenge, error) {
	var challenge LoginChallenge
	err := r.pool.QueryRow(ctx.Ctx,
		`select user_id, attempts, expires_at, used_at from login_challenges where id = $1`, challengeID).
		Scan(&challenge.UserID, &challenge.Attempts, &challenge.ExpiresAt, &challenge.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return challenge, ErrNotFound
	}
	return challenge, err
}

func (r *Repo) IncrementLoginChallenge(ctx RepoCtx, challengeID string) error {
	_, err := r.pool.Exec(ctx.Ctx, `update login_challenges set attempts = attempts + 1 where id = $1 and used_at is null`, challengeID)
	return err
}

func (r *Repo) UseLoginChallenge(ctx RepoCtx, challengeID string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx,
		`update login_challenges set used_at = now() where id = $1 and used_at is null and expires_at > now()`, challengeID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
