// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type CustomerListRow struct {
	ID        string
	Name      *string
	Email     *string
	CreatedAt time.Time
	LastSeen  *time.Time
	Total     int
	Open      int
}

func (r *Repo) ListCustomers(ctx RepoCtx, query string) ([]CustomerListRow, error) {
	rows, err := r.pool.Query(ctx.Ctx, `select cu.id::text, cu.name, cu.email, cu.created_at,
		max(c.updated_at) as last_seen,
		count(c.id)::int as total,
		count(c.id) filter (where c.status not in ('resolved','closed'))::int as open
		from customers cu
		left join conversations c on c.customer_id = cu.id and c.org_id = cu.org_id
		where cu.org_id = $1
		  and ($2 = '' or cu.name ilike '%' || $2 || '%' or cu.email ilike '%' || $2 || '%')
		group by cu.id, cu.name, cu.email, cu.created_at
		order by last_seen desc nulls last, cu.created_at desc
		limit 100`, ctx.OrgID, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomerListRow{}
	for rows.Next() {
		var row CustomerListRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Email, &row.CreatedAt, &row.LastSeen, &row.Total, &row.Open); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type CustomerRow struct {
	ID         string
	Name       *string
	Email      *string
	CreatedAt  time.Time
	Attributes json.RawMessage
}

func (r *Repo) GetCustomer(ctx RepoCtx, id string) (CustomerRow, error) {
	var customer CustomerRow
	err := r.pool.QueryRow(ctx.Ctx, `select id::text, name, email, created_at, attributes
		from customers where id = $1 and org_id = $2`, id, ctx.OrgID).
		Scan(&customer.ID, &customer.Name, &customer.Email, &customer.CreatedAt, &customer.Attributes)
	if errors.Is(err, pgx.ErrNoRows) {
		return customer, ErrNotFound
	}
	return customer, err
}

type CustomerConversationRow struct {
	ID        string
	Status    string
	Channel   string
	CreatedAt time.Time
	Preview   *string
}

func (r *Repo) CustomerConversations(ctx RepoCtx, customerID string) ([]CustomerConversationRow, error) {
	rows, err := r.pool.Query(ctx.Ctx, `select c.id::text, c.status::text, c.channel::text, c.created_at,
		(select m.content from messages m where m.conversation_id = c.id and m.role = 'customer' order by m.created_at limit 1) as preview
		from conversations c
		where c.org_id = $1 and c.customer_id = $2
		order by c.created_at desc limit 50`, ctx.OrgID, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomerConversationRow{}
	for rows.Next() {
		var row CustomerConversationRow
		if err := rows.Scan(&row.ID, &row.Status, &row.Channel, &row.CreatedAt, &row.Preview); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repo) CustomerOpenCount(ctx RepoCtx, customerID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx.Ctx, `select count(*)::int from conversations
		where org_id = $1 and customer_id = $2 and status not in ('resolved','closed')`, ctx.OrgID, customerID).Scan(&count)
	return count, err
}
