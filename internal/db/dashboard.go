// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"errors"

	"github.com/jackc/pgx/v5"
)

type OverviewCounts struct {
	OpenConversations int
	AwaitingHuman     int
	PendingApprovals  int
	Customers         int
	Sources           int
	ReadySources      int
	FailedSources     int
	EnabledProviders  int
	WidgetOrigins     int
}

func (r *Repo) OverviewCounts(ctx RepoCtx) (OverviewCounts, error) {
	var counts OverviewCounts
	err := r.pool.QueryRow(ctx.Ctx, `select
		(select count(*)::int from conversations where org_id = $1 and status not in ('resolved','closed')),
		(select count(*)::int from conversations where org_id = $1 and status = 'awaiting_human'),
		(select count(*)::int from actions where org_id = $1 and status = 'pending_approval'),
		(select count(*)::int from customers where org_id = $1),
		(select count(*)::int from knowledge_sources where org_id = $1),
		(select count(*)::int from knowledge_sources where org_id = $1 and status = 'ready'),
		(select count(*)::int from knowledge_sources where org_id = $1 and status = 'failed'),
		(select count(*)::int from provider_configs where org_id = $1 and enabled),
		(select coalesce(array_length(origins, 1), 0)::int from widget_configs where org_id = $1)`,
		ctx.OrgID).Scan(
		&counts.OpenConversations, &counts.AwaitingHuman, &counts.PendingApprovals,
		&counts.Customers, &counts.Sources, &counts.ReadySources, &counts.FailedSources,
		&counts.EnabledProviders, &counts.WidgetOrigins,
	)
	return counts, err
}

func (r *Repo) OrgName(ctx RepoCtx) (string, error) {
	var name string
	err := r.pool.QueryRow(ctx.Ctx, `select name from organization where id = $1`, ctx.OrgID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

func (r *Repo) UpdateOrgName(ctx RepoCtx, name string) error {
	tag, err := r.pool.Exec(ctx.Ctx, `update organization set name = $2 where id = $1`, ctx.OrgID, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type StatusCount struct {
	Status string
	Count  int
}

func (r *Repo) ConversationStatusCounts(ctx RepoCtx) ([]StatusCount, error) {
	rows, err := r.pool.Query(ctx.Ctx, `select status::text, count(*)::int from conversations where org_id = $1 group by status`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StatusCount{}
	for rows.Next() {
		var item StatusCount
		if err := rows.Scan(&item.Status, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
