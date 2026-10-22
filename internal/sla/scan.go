// SPDX-License-Identifier: AGPL-3.0-only
package sla

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/observability"
)

func ScanOnce(ctx context.Context, pool *pgxpool.Pool, notifier *observability.Notifier) (int, error) {
	rows, err := pool.Query(ctx,
		`with due as (
		   select c.org_id, c.id as conversation_id, 'first_response'::text as kind, c.first_response_due_at as due_at
		     from conversations c
		    where c.status not in ('resolved','closed')
		      and c.first_response_at is null
		      and c.first_response_due_at is not null
		      and c.first_response_due_at < now()
		   union all
		   select c.org_id, c.id, 'resolution', c.resolution_due_at
		     from conversations c
		    where c.status not in ('resolved','closed')
		      and c.resolution_due_at is not null
		      and c.resolution_due_at < now()
		 ), inserted as (
		   insert into sla_breaches (org_id, conversation_id, kind, due_at)
		   select org_id, conversation_id, kind, due_at from due
		   on conflict (conversation_id, kind) do nothing
		   returning id, org_id, conversation_id, kind, due_at
		 )
		 select i.id::text, i.org_id, i.conversation_id::text, i.kind, i.due_at, c.ticket_number
		   from inserted i join conversations c on c.id = i.conversation_id`)
	if err != nil {
		return 0, err
	}
	type breach struct {
		id, orgID, conversationID, kind string
		dueAt                           time.Time
		ticketNumber                    int64
	}
	breaches := []breach{}
	for rows.Next() {
		var item breach
		if err := rows.Scan(&item.id, &item.orgID, &item.conversationID, &item.kind, &item.dueAt, &item.ticketNumber); err != nil {
			rows.Close()
			return 0, err
		}
		breaches = append(breaches, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if len(breaches) == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(breaches))
	for _, item := range breaches {
		ids = append(ids, item.id)
		if notifier != nil {
			notifier.NotifyAsync(ctx, "sla.breach:"+item.orgID+":"+item.kind, "SLA target breached", map[string]any{
				"ticketNumber": item.ticketNumber, "conversationId": item.conversationID,
				"kind": item.kind, "dueAt": item.dueAt.UTC().Format(time.RFC3339),
			})
		}
	}
	if _, err := pool.Exec(ctx, `update sla_breaches set notified_at = now() where id = any($1::uuid[])`, ids); err != nil {
		return len(breaches), fmt.Errorf("mark SLA notices: %w", err)
	}
	return len(breaches), nil
}

func StartMonitor(ctx context.Context, pool *pgxpool.Pool, notifier *observability.Notifier, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	run := func() {
		runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		count, err := ScanOnce(runCtx, pool, notifier)
		if err != nil {
			logger.Warn("SLA scan failed", "error", err)
			return
		}
		if count > 0 {
			logger.Info("SLA breaches detected", "count", count)
		}
	}
	run()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
