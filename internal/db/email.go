// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type EmailOutboxRow struct {
	ID             string
	OrgID          string
	ConversationID *string
	SourceMessage  *string
	Kind           string
	ToEmail        string
	Subject        string
	TextBody       string
	HTMLBody       string
	EmailMessageID string
	InReplyTo      string
	References     string
	Attempts       int
}

func (r *Repo) EnqueueEmail(ctx RepoCtx, kind, toEmail, subject, textBody, htmlBody string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx.Ctx,
		`insert into email_outbox (org_id, kind, to_email, subject, text_body, html_body)
		 values ($1, $2, $3, $4, $5, nullif($6, '')) returning id::text`,
		ctx.OrgID, kind, toEmail, subject, textBody, htmlBody).Scan(&id)
	return id, err
}

func (r *Repo) ClaimEmailOutbox(ctx RepoCtx, limit int) ([]EmailOutboxRow, error) {
	rows, err := r.pool.Query(ctx.Ctx,
		`with claimed as (
		   update email_outbox set status = 'sending', attempts = attempts + 1
		    where id in (
		      select id from email_outbox
		       where org_id = $1 and status = 'queued'
		         and (next_attempt_at is null or next_attempt_at <= now())
		       order by created_at asc limit $2 for update skip locked
		    )
		    returning id, org_id, conversation_id::text, source_message_id::text, kind, to_email, subject, text_body, coalesce(html_body, '') as html_body, coalesce(email_message_id, '') as email_message_id, coalesce(in_reply_to, '') as in_reply_to, coalesce(references_header, '') as references_header, attempts
		 )
		 select id::text, org_id, conversation_id, source_message_id, kind, to_email, subject, text_body, html_body, email_message_id, in_reply_to, references_header, attempts from claimed`,
		ctx.OrgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmailOutboxRow{}
	for rows.Next() {
		var row EmailOutboxRow
		if err := rows.Scan(&row.ID, &row.OrgID, &row.ConversationID, &row.SourceMessage, &row.Kind, &row.ToEmail, &row.Subject, &row.TextBody, &row.HTMLBody, &row.EmailMessageID, &row.InReplyTo, &row.References, &row.Attempts); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repo) ClaimEmailOutboxAll(ctx context.Context, limit int) ([]EmailOutboxRow, error) {
	rows, err := r.pool.Query(ctx,
		`with claimed as (
		   update email_outbox set status = 'sending', attempts = attempts + 1
		    where id in (
		      select id from email_outbox
		       where status = 'queued' and (next_attempt_at is null or next_attempt_at <= now())
		       order by created_at asc limit $1 for update skip locked
		    )
		    returning id, org_id, conversation_id::text, source_message_id::text, kind, to_email, subject, text_body, coalesce(html_body, '') as html_body, coalesce(email_message_id, '') as email_message_id, coalesce(in_reply_to, '') as in_reply_to, coalesce(references_header, '') as references_header, attempts
		 )
		 select id::text, org_id, conversation_id, source_message_id, kind, to_email, subject, text_body, html_body, email_message_id, in_reply_to, references_header, attempts from claimed`,
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmailOutboxRow{}
	for rows.Next() {
		var row EmailOutboxRow
		if err := rows.Scan(&row.ID, &row.OrgID, &row.ConversationID, &row.SourceMessage, &row.Kind, &row.ToEmail, &row.Subject, &row.TextBody, &row.HTMLBody, &row.EmailMessageID, &row.InReplyTo, &row.References, &row.Attempts); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repo) SetEmailOutboxMessageID(ctx RepoCtx, id, messageID string) error {
	_, err := r.pool.Exec(ctx.Ctx, `update email_outbox set email_message_id = $2 where id = $1 and org_id = $3`, id, messageID, ctx.OrgID)
	return err
}

func (r *Repo) MarkEmailSent(ctx RepoCtx, id, messageID string) error {
	_, err := r.pool.Exec(ctx.Ctx,
		`update email_outbox set status = 'sent', email_message_id = $2, sent_at = now(), last_error = null where id = $1 and org_id = $3`,
		id, messageID, ctx.OrgID)
	return err
}

func (r *Repo) MarkEmailFailed(ctx RepoCtx, id, message string, retryAfter time.Duration) error {
	_, err := r.pool.Exec(ctx.Ctx,
		`update email_outbox set status = case when attempts >= 5 then 'failed' else 'queued' end,
		   last_error = $2, next_attempt_at = case when attempts >= 5 then null else now() + make_interval(secs => $3) end
		 where id = $1 and org_id = $4`, id, truncateDBText(message), int(retryAfter.Seconds()), ctx.OrgID)
	return err
}

func (r *Repo) EmailOutboxCounts(ctx RepoCtx) (queued, sent, failed int, err error) {
	err = r.pool.QueryRow(ctx.Ctx,
		`select count(*) filter (where status = 'queued')::int,
		        count(*) filter (where status = 'sent')::int,
		        count(*) filter (where status = 'failed')::int
		   from email_outbox where org_id = $1`, ctx.OrgID).Scan(&queued, &sent, &failed)
	return
}

func truncateDBText(value string) string {
	if len(value) <= 1000 {
		return value
	}
	return value[:1000]
}

type OrganizationInfo struct {
	ID   string
	Name string
}

func (r *Repo) FirstOrganization(ctx context.Context) (OrganizationInfo, error) {
	var org OrganizationInfo
	err := r.pool.QueryRow(ctx, `select id, name from organization order by created_at asc limit 1`).Scan(&org.ID, &org.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return org, ErrNotFound
	}
	return org, err
}
