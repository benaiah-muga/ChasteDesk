// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestBootstrapCreatesDefaultsOnce(t *testing.T) {
	app := startTestApp(t)
	ctx := context.Background()

	first := app.request(http.MethodPost, "/api/bootstrap", map[string]any{
		"orgName": "Bootstrap Org", "userName": "Root", "email": "root@boot.test", "password": "password123",
	}, nil, "")
	if first.status != http.StatusCreated {
		t.Fatalf("bootstrap failed: %d %s", first.status, first.body)
	}
	if _, ok := first.data(t)["orgId"].(string); !ok {
		t.Fatalf("bootstrap must return orgId: %s", first.body)
	}

	repeat := app.request(http.MethodPost, "/api/bootstrap", map[string]any{
		"orgName": "Second Org", "userName": "X", "email": "x@boot.test", "password": "password123",
	}, nil, "")
	if repeat.status != http.StatusConflict {
		t.Fatalf("second bootstrap must 409, got %d", repeat.status)
	}
	if code := repeat.errorCode(t); code != "conflict" {
		t.Fatalf("409 body must use conflict code, got %q", code)
	}

	var orgCount int
	if err := app.pool.QueryRow(ctx, "select count(*)::int from organization").Scan(&orgCount); err != nil {
		t.Fatalf("org count: %v", err)
	}
	if orgCount != 1 {
		t.Errorf("expected exactly one organization, got %d", orgCount)
	}

	rows, err := app.pool.Query(ctx, "select policy from agents")
	if err != nil {
		t.Fatalf("agents query: %v", err)
	}
	var policies []json.RawMessage
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan policy: %v", err)
		}
		policies = append(policies, raw)
	}
	rows.Close()
	if len(policies) != 1 {
		t.Fatalf("bootstrap must seed exactly one default agent, got %d", len(policies))
	}
	var policy struct {
		Purpose string            `json:"purpose"`
		Tools   []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(policies[0], &policy); err != nil {
		t.Fatalf("policy not JSON: %v", err)
	}
	if len(policy.Purpose) <= 10 {
		t.Errorf("default agent policy missing purpose: %q", policy.Purpose)
	}
	if len(policy.Tools) == 0 {
		t.Error("default agent policy missing tools")
	}

	var widgetCount int
	if err := app.pool.QueryRow(ctx, "select count(*)::int from widget_configs").Scan(&widgetCount); err != nil {
		t.Fatalf("widget count: %v", err)
	}
	if widgetCount != 1 {
		t.Errorf("bootstrap must seed a widget config, got %d", widgetCount)
	}

	var auditCount int
	if err := app.pool.QueryRow(ctx, "select count(*)::int from audit_events where type='org.created'").Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("bootstrap must audit org.created exactly once, got %d", auditCount)
	}

	status := app.request(http.MethodGet, "/api/onboarding/status", nil, nil, "")
	if status.status != http.StatusOK {
		t.Fatalf("onboarding status failed: %d", status.status)
	}
	data := status.data(t)
	if orgCreated, ok := data["orgCreated"].(bool); !ok || orgCreated {
		t.Errorf("anonymous onboarding status must report orgCreated false: %s", status.body)
	}
}
