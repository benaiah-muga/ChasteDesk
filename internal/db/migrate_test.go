// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var schemaTables = []string{
	"user", "session", "account", "verification", "organization", "member", "invitation",
	"customers", "conversations", "messages", "knowledge_sources", "knowledge_chunks",
	"agents", "provider_configs", "api_keys", "widget_configs", "ai_runs", "actions",
	"audit_events", "analytics_daily",
}

var orgScopedTables = []string{
	"customers", "conversations", "messages", "knowledge_sources", "knowledge_chunks",
	"agents", "provider_configs", "api_keys", "widget_configs", "ai_runs", "actions",
	"analytics_daily",
}

var schemaEnums = []string{
	"channel", "conversation_status", "message_role", "knowledge_source_type",
	"knowledge_scope", "knowledge_status", "action_status", "run_status", "audit_actor",
}

func TestMigrateFullSchema(t *testing.T) {
	pool := startMigratedPool(t)
	ctx := context.Background()

	for _, table := range schemaTables {
		if !tableExists(t, pool, table) {
			t.Errorf("missing table: %s", table)
		}
	}
	for _, enumName := range schemaEnums {
		var count int
		if err := pool.QueryRow(ctx, "select count(*) from pg_type where typname=$1 and typtype='e'", enumName).Scan(&count); err != nil {
			t.Fatalf("enum check %s: %v", enumName, err)
		}
		if count != 1 {
			t.Errorf("missing enum type: %s", enumName)
		}
	}
	for _, table := range orgScopedTables {
		cols := columns(t, pool, table)
		orgID, ok := cols["org_id"]
		if !ok {
			t.Errorf("missing org_id on %s", table)
			continue
		}
		if orgID.IsNullable != "NO" {
			t.Errorf("org_id must be not null on %s", table)
		}
	}
	conv := columns(t, pool, "conversations")
	for _, col := range []string{"status", "triage", "handoff", "first_response_at", "resolved_at", "channel"} {
		if _, ok := conv[col]; !ok {
			t.Errorf("conversations missing column %s", col)
		}
	}
	chunks := columns(t, pool, "knowledge_chunks")
	if chunks["embedding"].UDTName != "vector" {
		t.Errorf("knowledge_chunks.embedding must be vector, got %q", chunks["embedding"].UDTName)
	}
	if chunks["tsv"].IsGenerated != "ALWAYS" {
		t.Errorf("knowledge_chunks.tsv must be a generated column, got %q", chunks["tsv"].IsGenerated)
	}
	msgs := columns(t, pool, "messages")
	if _, ok := msgs["citations"]; !ok {
		t.Error("messages missing citations column")
	}
	prov := columns(t, pool, "provider_configs")
	if _, ok := prov["key_cipher"]; !ok {
		t.Error("provider_configs missing encrypted key column")
	}
	trigRows, err := pool.Query(ctx,
		`select tgname from pg_trigger where tgrelid='audit_events'::regclass and not tgisinternal`)
	if err != nil {
		t.Fatalf("trigger query: %v", err)
	}
	triggers := map[string]bool{}
	for trigRows.Next() {
		var name string
		if err := trigRows.Scan(&name); err != nil {
			t.Fatalf("scan trigger: %v", err)
		}
		triggers[name] = true
	}
	trigRows.Close()
	if !triggers["audit_events_no_update"] {
		t.Error("missing audit_events_no_update trigger")
	}
	if !triggers["audit_events_no_delete"] {
		t.Error("missing audit_events_no_delete trigger")
	}

	fpA := fingerprint(t, pool)

	if _, err := pool.Exec(ctx, `insert into organization (id, name, slug) values ('org1','Org One','org-one')`); err != nil {
		t.Fatalf("organization insert failed: %v", err)
	}
	var customerID string
	if err := pool.QueryRow(ctx, `insert into customers (org_id, email, name) values ('org1','a@b.c','A') returning id::text`).Scan(&customerID); err != nil {
		t.Fatalf("customer insert failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into conversations (org_id, customer_id, channel) values ('org1',$1,'widget')`, customerID); err != nil {
		t.Fatalf("conversation insert failed: %v", err)
	}
	var convCount int
	if err := pool.QueryRow(ctx, "select count(*)::int from conversations where org_id='org1'").Scan(&convCount); err != nil {
		t.Fatalf("conversation count: %v", err)
	}
	if convCount != 1 {
		t.Errorf("org-scoped conversation insert failed: count = %d", convCount)
	}

	dropAllPublic(t, pool)
	var remaining int
	if err := pool.QueryRow(ctx,
		"select count(*)::int from information_schema.tables where table_schema='public' and table_type='BASE TABLE'").Scan(&remaining); err != nil {
		t.Fatalf("remaining count: %v", err)
	}
	if remaining != 0 {
		t.Errorf("down-drop left %d tables behind", remaining)
	}

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	fpB := fingerprint(t, pool)
	if fpA != fpB {
		t.Errorf("re-migrated schema differs from original fingerprint\na:\n%s\nb:\n%s", fpA, fpB)
	}
	if !tableExists(t, pool, "organization") {
		t.Error("re-migrated database missing tables")
	}
}

func TestAuditEventsImmutable(t *testing.T) {
	pool := startMigratedPool(t)
	ctx := context.Background()
	var id int64
	if err := pool.QueryRow(ctx,
		`insert into audit_events (org_id, actor_type, type, target_type, payload)
		 values ('org_probe', 'system', 'probe.event', 'probe', '{}'::jsonb) returning id`).Scan(&id); err != nil {
		t.Fatalf("insert audit event: %v", err)
	}
	if _, err := pool.Exec(ctx, "update audit_events set type='tampered' where id=$1", id); err == nil {
		t.Error("UPDATE on audit_events did not fail")
	}
	if _, err := pool.Exec(ctx, "delete from audit_events where id=$1", id); err == nil {
		t.Error("DELETE on audit_events did not fail")
	}
	if _, err := pool.Exec(ctx,
		`insert into audit_events (org_id, actor_type, type, target_type, payload)
		 values ('org_probe', 'system', 'probe.event2', 'probe', '{}'::jsonb)`); err != nil {
		t.Errorf("INSERT into audit_events must remain permitted: %v", err)
	}
}

func dropAllPublic(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx,
		`select table_name from information_schema.tables where table_schema='public' and table_type='BASE TABLE'`)
	if err != nil {
		t.Fatalf("dropAll tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	for _, table := range tables {
		if _, err := pool.Exec(ctx, `drop table if exists "`+table+`" cascade`); err != nil {
			t.Fatalf("drop table %s: %v", table, err)
		}
	}
	typeRows, err := pool.Query(ctx,
		`select t.typname from pg_type t join pg_namespace n on n.oid=t.typnamespace
		 where n.nspname='public' and t.typtype='e'`)
	if err != nil {
		t.Fatalf("dropAll types: %v", err)
	}
	var types []string
	for typeRows.Next() {
		var name string
		if err := typeRows.Scan(&name); err != nil {
			t.Fatalf("scan type: %v", err)
		}
		types = append(types, name)
	}
	typeRows.Close()
	for _, name := range types {
		if _, err := pool.Exec(ctx, `drop type if exists "`+name+`" cascade`); err != nil {
			t.Fatalf("drop type %s: %v", name, err)
		}
	}
}
