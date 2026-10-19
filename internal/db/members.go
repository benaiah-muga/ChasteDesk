// SPDX-License-Identifier: AGPL-3.0-only
package db

import "time"

type MemberRow struct {
	UserID   string
	Name     string
	Email    string
	Role     string
	JoinedAt time.Time
}

func (r *Repo) ListMembers(ctx RepoCtx) ([]MemberRow, error) {
	rows, err := r.pool.Query(ctx.Ctx, `select u.id, u.name, u.email, m.role, m.created_at
		from member m join "user" u on u.id = m.user_id
		where m.organization_id = $1 order by m.created_at asc`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemberRow{}
	for rows.Next() {
		var row MemberRow
		if err := rows.Scan(&row.UserID, &row.Name, &row.Email, &row.Role, &row.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
