// SPDX-License-Identifier: AGPL-3.0-only
package sla_test

import (
	"context"
	"testing"

	"chastedesk/internal/db"
	"chastedesk/internal/id"
	"chastedesk/internal/sla"
	"chastedesk/internal/testutil"
)

func TestScanOnceRecordsBreaches(t *testing.T) {
	dsn := testutil.StartEphemeralPG(t)
	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	const orgID = "org-sla"
	if _, err := pool.Exec(ctx, `insert into organization (id, name, slug) values ($1, 'SLA Org', 'sla-org')`, orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	customerID := id.UUID()
	if _, err := pool.Exec(ctx, `insert into customers (id, org_id, email, name) values ($1, $2, 'sla@example.test', 'SLA Customer')`, customerID, orgID); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	conversationID := id.UUID()
	if _, err := pool.Exec(ctx, `insert into conversations (id, org_id, customer_id, channel, status, priority, created_at) values ($1, $2, $3, 'widget', 'awaiting_human', 'normal', now() - interval '2 hours')`, conversationID, orgID, customerID); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := pool.Exec(ctx, `update conversations set first_response_due_at = now() - interval '1 hour', resolution_due_at = now() - interval '1 hour' where id = $1`, conversationID); err != nil {
		t.Fatalf("set due dates: %v", err)
	}
	count, err := sla.ScanOnce(ctx, pool, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two breaches, got %d", count)
	}
	var first, resolution int
	if err := pool.QueryRow(ctx, `select count(*) filter (where kind = 'first_response')::int, count(*) filter (where kind = 'resolution')::int from sla_breaches where org_id = $1`, orgID).Scan(&first, &resolution); err != nil {
		t.Fatalf("count breaches: %v", err)
	}
	if first != 1 || resolution != 1 {
		t.Fatalf("breach records mismatch: first=%d resolution=%d", first, resolution)
	}
	if repeat, err := sla.ScanOnce(ctx, pool, nil); err != nil || repeat != 0 {
		t.Fatalf("scan should be idempotent: count=%d err=%v", repeat, err)
	}
}
