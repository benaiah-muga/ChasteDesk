// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type TicketFields struct {
	Number       int64
	Priority     string
	Tags         []string
	SnoozedUntil *time.Time
}

type TicketUpdate struct {
	Priority     *string
	Tags         *[]string
	SnoozedUntil *time.Time
	ClearSnooze  bool
}

func (r *Repo) UpdateTicketFields(ctx RepoCtx, conversationID string, update TicketUpdate) (TicketFields, error) {
	var (
		priorityArg any
		tagsArg     any
		snoozeArg   any
	)
	if update.Priority != nil {
		priorityArg = *update.Priority
	}
	if update.Tags != nil {
		tagsArg = *update.Tags
	}
	if update.SnoozedUntil != nil {
		snoozeArg = *update.SnoozedUntil
	}
	var fields TicketFields
	err := r.pool.QueryRow(ctx.Ctx,
		`update conversations set
		   priority = coalesce($3, priority),
		   tags = coalesce($4::text[], tags),
		   snoozed_until = case when $5 then null when $6::timestamptz is not null then $6 else snoozed_until end,
		   updated_at = now()
		 where id = $1 and org_id = $2
		 returning ticket_number, priority, tags, snoozed_until`,
		conversationID, ctx.OrgID, priorityArg, tagsArg, update.ClearSnooze, snoozeArg).
		Scan(&fields.Number, &fields.Priority, &fields.Tags, &fields.SnoozedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return fields, ErrNotFound
	}
	return fields, err
}

type CannedReply struct {
	ID        string
	Title     string
	Body      string
	CreatedBy *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (r *Repo) ListCannedReplies(ctx RepoCtx) ([]CannedReply, error) {
	rows, err := r.pool.Query(ctx.Ctx,
		`select id::text, title, body, created_by, created_at, updated_at
		   from canned_replies where org_id = $1 order by lower(title)`, ctx.OrgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CannedReply{}
	for rows.Next() {
		var reply CannedReply
		if err := rows.Scan(&reply.ID, &reply.Title, &reply.Body, &reply.CreatedBy, &reply.CreatedAt, &reply.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, reply)
	}
	return out, rows.Err()
}

func (r *Repo) CreateCannedReply(ctx RepoCtx, title, body, createdBy string) (CannedReply, error) {
	var reply CannedReply
	var by *string
	if createdBy != "" {
		by = &createdBy
	}
	err := r.pool.QueryRow(ctx.Ctx,
		`insert into canned_replies (org_id, title, body, created_by)
		 values ($1, $2, $3, $4)
		 returning id::text, title, body, created_by, created_at, updated_at`,
		ctx.OrgID, title, body, by).
		Scan(&reply.ID, &reply.Title, &reply.Body, &reply.CreatedBy, &reply.CreatedAt, &reply.UpdatedAt)
	return reply, err
}

func (r *Repo) UpdateCannedReply(ctx RepoCtx, id, title, body string) (CannedReply, error) {
	var reply CannedReply
	err := r.pool.QueryRow(ctx.Ctx,
		`update canned_replies set title = $3, body = $4, updated_at = now()
		 where id = $1 and org_id = $2
		 returning id::text, title, body, created_by, created_at, updated_at`,
		id, ctx.OrgID, title, body).
		Scan(&reply.ID, &reply.Title, &reply.Body, &reply.CreatedBy, &reply.CreatedAt, &reply.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return reply, ErrNotFound
	}
	return reply, err
}

func (r *Repo) DeleteCannedReply(ctx RepoCtx, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx.Ctx, `delete from canned_replies where id = $1 and org_id = $2`, id, ctx.OrgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
