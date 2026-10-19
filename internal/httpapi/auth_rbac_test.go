// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"net/http"
	"testing"
)

func TestRBACMatrix(t *testing.T) {
	app := startTestApp(t)
	ctx := context.Background()

	boot := app.request(http.MethodPost, "/api/bootstrap", map[string]any{
		"orgName": "Rbac Org", "userName": "Boss", "email": "owner@rbac.test", "password": "password123",
	}, nil, "")
	if boot.status != http.StatusCreated {
		t.Fatalf("bootstrap failed: %d %s", boot.status, boot.body)
	}

	ownerJar := newTestJar()
	ownerLogin := app.request(http.MethodPost, "/api/auth/sign-in/email", map[string]any{
		"email": "owner@rbac.test", "password": "password123",
	}, ownerJar, "")
	if ownerLogin.status != http.StatusOK {
		t.Fatalf("owner login failed: %d %s", ownerLogin.status, ownerLogin.body)
	}

	for _, member := range []struct{ name, email string }{
		{"Admin Person", "admin@rbac.test"},
		{"Agent Person", "agent@rbac.test"},
	} {
		signup := app.request(http.MethodPost, "/api/auth/sign-up/email", map[string]any{
			"name": member.name, "email": member.email, "password": "password123",
		}, nil, "")
		if signup.status != http.StatusOK && signup.status != http.StatusCreated {
			t.Fatalf("signup failed for %s: %d %s", member.email, signup.status, signup.body)
		}
	}

	var orgID string
	if err := app.pool.QueryRow(ctx, "select id::text from organization limit 1").Scan(&orgID); err != nil {
		t.Fatalf("org lookup: %v", err)
	}
	for _, member := range []struct{ email, role string }{
		{"admin@rbac.test", "admin"},
		{"agent@rbac.test", "agent"},
	} {
		var userID string
		if err := app.pool.QueryRow(ctx, `select id::text from "user" where email=$1`, member.email).Scan(&userID); err != nil {
			t.Fatalf("user lookup %s: %v", member.email, err)
		}
		if _, err := app.pool.Exec(ctx,
			"insert into member (id, organization_id, user_id, role) values (gen_random_uuid()::text, $1, $2, $3)",
			orgID, userID, member.role); err != nil {
			t.Fatalf("insert member %s: %v", member.email, err)
		}
	}

	adminJar := newTestJar()
	agentJar := newTestJar()
	adminLogin := app.request(http.MethodPost, "/api/auth/sign-in/email", map[string]any{
		"email": "admin@rbac.test", "password": "password123",
	}, adminJar, "")
	agentLogin := app.request(http.MethodPost, "/api/auth/sign-in/email", map[string]any{
		"email": "agent@rbac.test", "password": "password123",
	}, agentJar, "")
	if adminLogin.status != http.StatusOK {
		t.Fatalf("admin login failed: %d %s", adminLogin.status, adminLogin.body)
	}
	if agentLogin.status != http.StatusOK {
		t.Fatalf("agent login failed: %d %s", agentLogin.status, agentLogin.body)
	}

	agentAudit := app.request(http.MethodGet, "/api/audit", nil, agentJar, "")
	if agentAudit.status != http.StatusForbidden {
		t.Errorf("agent audit access must 403, got %d", agentAudit.status)
	}
	agentSettings := app.request(http.MethodGet, "/api/settings/providers", nil, agentJar, "")
	if agentSettings.status != http.StatusForbidden {
		t.Errorf("agent settings access must 403, got %d", agentSettings.status)
	}

	adminAudit := app.request(http.MethodGet, "/api/audit", nil, adminJar, "")
	if adminAudit.status != http.StatusOK {
		t.Errorf("admin audit access must 200, got %d %s", adminAudit.status, adminAudit.body)
	}
	adminSettings := app.request(http.MethodGet, "/api/settings/providers", nil, adminJar, "")
	if adminSettings.status != http.StatusOK {
		t.Errorf("admin settings access must 200, got %d %s", adminSettings.status, adminSettings.body)
	}

	ownerAudit := app.request(http.MethodGet, "/api/audit", nil, ownerJar, "")
	if ownerAudit.status != http.StatusOK {
		t.Errorf("owner audit access must 200, got %d %s", ownerAudit.status, ownerAudit.body)
	}

	anonymous := app.request(http.MethodGet, "/api/audit", nil, nil, "")
	if anonymous.status != http.StatusUnauthorized {
		t.Errorf("anonymous audit access must 401, got %d", anonymous.status)
	}

	forged := app.request(http.MethodGet, "/api/audit", nil, nil, "chastedesk.session=forged-token-value")
	if forged.status != http.StatusUnauthorized {
		t.Errorf("forged session must 401, got %d", forged.status)
	}
}
