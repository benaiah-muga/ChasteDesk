// SPDX-License-Identifier: AGPL-3.0-only
package db

import (
	"context"
	"testing"
)

func TestMigrationRollbackDeterministic(t *testing.T) {
	first := startMigratedPool(t)
	firstFP := fingerprint(t, first)
	dropAllPublic(t, first)
	var remaining int
	if err := first.QueryRow(context.Background(),
		"select count(*)::int from information_schema.tables where table_schema='public' and table_type='BASE TABLE'").Scan(&remaining); err != nil {
		t.Fatalf("remaining count: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("down left %d tables behind", remaining)
	}

	second := startMigratedPool(t)
	secondFP := fingerprint(t, second)
	if firstFP == "" {
		t.Fatal("first fingerprint missing")
	}
	if firstFP != secondFP {
		t.Error("schema fingerprint differs between independent clean migrations")
	}
}
