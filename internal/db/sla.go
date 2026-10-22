// SPDX-License-Identifier: AGPL-3.0-only
package db

type SLAPolicy struct {
	ID                   string
	Priority             string
	FirstResponseMinutes int
	ResolutionMinutes    int
	Enabled              bool
}

type SLAPolicyInput struct {
	Priority             string
	FirstResponseMinutes int
	ResolutionMinutes    int
	Enabled              bool
}

func (r *Repo) ListSLAPolicies(ctx RepoCtx) ([]SLAPolicy, error) {
	rows, err := r.pool.Query(ctx.Ctx,
		`select id::text, priority, first_response_minutes, resolution_minutes, enabled
		   from sla_policies where org_id = $1 order by case priority
		     when 'urgent' then 0 when 'high' then 1 when 'normal' then 2 else 3 end`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SLAPolicy{}
	for rows.Next() {
		var policy SLAPolicy
		if err := rows.Scan(&policy.ID, &policy.Priority, &policy.FirstResponseMinutes, &policy.ResolutionMinutes, &policy.Enabled); err != nil {
			return nil, err
		}
		out = append(out, policy)
	}
	return out, rows.Err()
}

func (r *Repo) SaveSLAPolicies(ctx RepoCtx, policies []SLAPolicyInput) ([]SLAPolicy, error) {
	tx, err := r.pool.Begin(ctx.Ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx.Ctx) }()
	for _, policy := range policies {
		if _, err := tx.Exec(ctx.Ctx,
			`insert into sla_policies (org_id, priority, first_response_minutes, resolution_minutes, enabled)
			 values ($1, $2, $3, $4, $5)
			 on conflict (org_id, priority) do update set
			   first_response_minutes = excluded.first_response_minutes,
			   resolution_minutes = excluded.resolution_minutes,
			   enabled = excluded.enabled,
			   updated_at = now()`,
			ctx.OrgID, policy.Priority, policy.FirstResponseMinutes, policy.ResolutionMinutes, policy.Enabled); err != nil {
			return nil, err
		}
	}
	// Recompute open-ticket targets when a policy changes.
	if _, err := tx.Exec(ctx.Ctx,
		`update conversations c set
		   first_response_due_at = case when p.enabled then c.created_at + make_interval(mins => p.first_response_minutes) else null end,
		   resolution_due_at = case when p.enabled then c.created_at + make_interval(mins => p.resolution_minutes) else null end
		 from sla_policies p
		 where p.org_id = c.org_id and p.priority = c.priority
		   and c.org_id = $1 and c.status not in ('resolved','closed')`, ctx.OrgID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx.Ctx); err != nil {
		return nil, err
	}
	return r.ListSLAPolicies(ctx)
}

func (r *Repo) CountSLABreached(ctx RepoCtx) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx.Ctx,
		`select count(*)::int from conversations
		  where org_id = $1 and status not in ('resolved','closed')
		    and ((first_response_at is null and first_response_due_at < now())
		      or (resolution_due_at < now()))`, ctx.OrgID).Scan(&count)
	return count, err
}
