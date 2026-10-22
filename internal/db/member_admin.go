// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"errors"

	"github.com/jackc/pgx/v5"
)

func (r *Repo) MemberRole(ctx RepoCtx, userID string) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx.Ctx, `select role from member where organization_id = $1 and user_id = $2`, ctx.OrgID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return role, err
}

func (r *Repo) CountOwners(ctx RepoCtx) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx.Ctx, `select count(*)::int from member where organization_id = $1 and role = 'owner'`, ctx.OrgID).Scan(&count)
	return count, err
}

func (r *Repo) SetMemberRole(ctx RepoCtx, userID, role string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx, `update member set role = $3 where organization_id = $1 and user_id = $2`, ctx.OrgID, userID, role)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) RemoveMember(ctx RepoCtx, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx, `delete from member where organization_id = $1 and user_id = $2`, ctx.OrgID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) DeleteSessionsForUser(ctx RepoCtx, userID string) (int, error) {
	tag, err := r.pool.Exec(ctx.Ctx, `delete from session where user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
