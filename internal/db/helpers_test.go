// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/testutil"
)

type columnInfo struct {
	DataType    string
	UDTName     string
	IsNullable  string
	IsGenerated string
}

func startMigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testutil.StartEphemeralPG(t)
	pool, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(),
		"select exists(select 1 from information_schema.tables where table_schema='public' and table_name=$1)", name).Scan(&exists)
	if err != nil {
		t.Fatalf("tableExists(%s): %v", name, err)
	}
	return exists
}

func columns(t *testing.T, pool *pgxpool.Pool, table string) map[string]columnInfo {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`select column_name, data_type, udt_name, is_nullable, is_generated
		 from information_schema.columns
		 where table_schema='public' and table_name=$1`, table)
	if err != nil {
		t.Fatalf("columns(%s): %v", table, err)
	}
	defer rows.Close()
	out := map[string]columnInfo{}
	for rows.Next() {
		var name string
		var c columnInfo
		if err := rows.Scan(&name, &c.DataType, &c.UDTName, &c.IsNullable, &c.IsGenerated); err != nil {
			t.Fatalf("scan column %s: %v", table, err)
		}
		out[name] = c
	}
	if rows.Err() != nil {
		t.Fatalf("rows(%s): %v", table, rows.Err())
	}
	return out
}

func fingerprint(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`select table_name from information_schema.tables
		 where table_schema='public' and table_type='BASE TABLE' order by table_name`)
	if err != nil {
		t.Fatalf("fingerprint tables: %v", err)
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
	parts := make([]string, 0, len(tables))
	for _, table := range tables {
		cols := columns(t, pool, table)
		var colParts []string
		names := make([]string, 0, len(cols))
		for name := range cols {
			names = append(names, name)
		}
		sortStrings(names)
		for _, name := range names {
			c := cols[name]
			colParts = append(colParts, fmt.Sprintf("%s|%s|%s|%s|%s", name, c.DataType, c.UDTName, c.IsNullable, c.IsGenerated))
		}
		parts = append(parts, table+":"+joinStrings(colParts, ";"))
	}
	return joinStrings(parts, "\n")
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func joinStrings(s []string, sep string) string {
	out := ""
	for i, part := range s {
		if i > 0 {
			out += sep
		}
		out += part
	}
	return out
}
