// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSetupFlow(t *testing.T) {
	app := startWorkerTestApp(t, t.TempDir())
	jar := newTestJar()

	root := app.getPageRaw(t, "/", jar)
	if root.status != http.StatusSeeOther {
		t.Fatalf("empty install root must redirect to setup, got %d", root.status)
	}

	setup := app.getPage(t, "/setup", jar)
	page := pageBody(t, setup)
	for _, needle := range []string{"Create your desk", `id="orgName"`, `id="userName"`, `id="email"`, `id="password"`, "Create the desk", "Support that shows its work."} {
		if !strings.Contains(page, needle) {
			t.Fatalf("setup page missing %q", needle)
		}
	}

	created := app.postForm(t, "/setup", url.Values{
		"orgName": {"Kestrel Support"}, "userName": {"Amina Okoye"},
		"email": {"owner@setup.test"}, "password": {"password123"},
	}, jar)
	if created.status != http.StatusSeeOther {
		t.Fatalf("setup submit: %d %s", created.status, created.body)
	}

	dashboard := app.getPage(t, "/dashboard", jar)
	dashboardPage := pageBody(t, dashboard)
	for _, needle := range []string{"Overview", "Agent readiness", "Recent conversations", "What needs a human", "ChasteDesk", "Kestrel Support"} {
		if !strings.Contains(dashboardPage, needle) {
			t.Fatalf("dashboard missing %q", needle)
		}
	}

	again := app.getPageRaw(t, "/setup", jar)
	if again.status != http.StatusSeeOther {
		t.Fatalf("setup after organization exists must redirect, got %d", again.status)
	}
	loggedInLogin := app.getPageRaw(t, "/login", jar)
	if loggedInLogin.status != http.StatusSeeOther {
		t.Fatalf("login when signed in must redirect, got %d", loggedInLogin.status)
	}

	app.postForm(t, "/logout", url.Values{}, jar)
	signedOut := app.getPageRaw(t, "/dashboard", jar)
	if signedOut.status != http.StatusSeeOther {
		t.Fatalf("dashboard after logout must redirect, got %d", signedOut.status)
	}
	login := app.getPage(t, "/login", jar)
	if !strings.Contains(pageBody(t, login), "Sign in to your desk") {
		t.Fatal("login page must render after logout")
	}
}

func TestProductSurfaces(t *testing.T) {
	app := startWorkerTestApp(t, t.TempDir())
	jar, orgID := app.bootstrapAndSignIn(t, "Product Org", "owner@product.test")

	provider := app.request("POST", "/api/settings/providers", map[string]any{
		"providerId": "openai", "displayName": "OpenAI", "apiKey": "sk-product-test-key",
	}, jar, "")
	if provider.status != http.StatusCreated {
		t.Fatalf("provider save: %d %s", provider.status, provider.body)
	}
	setWidgetOrigins(t, app, jar)

	customerID := seedInboxCustomer(t, app, orgID, "ada@product.test", "Ada Lovelace", `{"plan":"Scale"}`)
	conversationID := seedInboxConversation(t, app, orgID, customerID, "awaiting_human", `{"problem":"Billing email needs to change.","recommendedNextStep":"Walk through Settings then Billing.","investigated":[],"evidence":[],"actionsTaken":[]}`)
	seedInboxMessage(t, app, orgID, conversationID, "customer", "How do I change the billing email?")

	ctx := context.Background()
	var agentID string
	if err := app.pool.QueryRow(ctx, `select id from agents where org_id = $1 limit 1`, orgID).Scan(&agentID); err != nil {
		t.Fatalf("agent id: %v", err)
	}
	runID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2"
	if _, err := app.pool.Exec(ctx, `insert into ai_runs (id, org_id, conversation_id, agent_id, status) values ($1, $2, $3, $4, 'awaiting_approval')`, runID, orgID, conversationID, agentID); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := app.pool.Exec(ctx, `insert into actions (org_id, run_id, conversation_id, agent_id, tool_id, args, status) values ($1, $2, $3, $4, 'conversations.add_note', '{"note":"confirm plan"}', 'pending_approval')`, orgID, runID, conversationID, agentID); err != nil {
		t.Fatalf("seed action: %v", err)
	}

	pages := []struct {
		path string
		want []string
	}{
		{"/dashboard", []string{"Overview", "Recent conversations", "Agent readiness", "Quick actions"}},
		{"/dashboard/inbox", []string{"Conversations", "Ada Lovelace", "awaiting human"}},
		{"/dashboard/inbox/" + conversationID, []string{"Why the AI escalated", "Approve", "Draft with copilot", "ada@product.test"}},
		{"/dashboard/customers", []string{"Customers", "Ada Lovelace", "1 conversation"}},
		{"/dashboard/customers/" + customerID, []string{"Ada Lovelace", "Scale", "Conversations"}},
		{"/dashboard/approvals", []string{"Approvals", "Waiting on a human", "conversations.add_note", "Open conversation"}},
		{"/dashboard/knowledge", []string{"Knowledge", "Add source", "No sources yet."}},
		{"/dashboard/analytics", []string{"Analytics", "No analytics yet"}},
		{"/dashboard/audit", []string{"Audit log", "Events", "append-only"}},
		{"/dashboard/settings", []string{"AI providers", "Save provider", "Chat widget", "data-org="}},
		{"/dashboard/settings?section=agent", []string{"Support agent", "Tools and approvals", "Escalation", "Save policy"}},
		{"/dashboard/settings?section=widget", []string{"Chat widget", "Allowed origins", "Embed snippet"}},
		{"/dashboard/settings?section=api-keys", []string{"API keys", "Create API key"}},
		{"/dashboard/settings?section=workspace", []string{"Workspace", "Members", "AGPL-3.0-only"}},
		{"/dashboard/settings?section=account", []string{"Account", "Password", "Save account"}},
		{"/dashboard/account", []string{"Account", "Password", "Save account"}},
	}
	for _, page := range pages {
		body := pageBody(t, app.getPage(t, page.path, jar))
		for _, needle := range page.want {
			if !strings.Contains(body, needle) {
				t.Fatalf("%s missing %q", page.path, needle)
			}
		}
	}

	agentPage := pageBody(t, app.getPage(t, "/dashboard/settings?section=agent", jar))
	if strings.Contains(agentPage, `selected="false"`) || strings.Contains(agentPage, `checked="false"`) {
		t.Fatal("boolean attributes must not render as selected=\"false\"/checked=\"false\"")
	}

	policy := app.postForm(t, "/dashboard/settings/agent", url.Values{
		"purpose":                     {"Answer billing questions and escalate refunds."},
		"publicScope":                 {"1"},
		"threshold":                   {"0.4"},
		"keywords":                    {"refund, chargeback"},
		"providerId":                  {"openai"},
		"modelId":                     {"gpt-4.1-mini"},
		"tool:knowledge.search":       {"autonomous"},
		"tool:customers.lookup":       {"autonomous"},
		"tool:conversations.add_note": {"approval_required"},
		"tool:escalate":               {"autonomous"},
		"maxSteps":                    {"5"},
		"maxToolCalls":                {"3"},
		"maxInputTokens":              {"6000"},
		"maxOutputTokens":             {"1200"},
	}, jar)
	if policy.status != http.StatusSeeOther {
		t.Fatalf("agent save: %d %s", policy.status, policy.body)
	}
	var rawPolicy []byte
	if err := app.pool.QueryRow(ctx, `select policy from agents where org_id = $1 limit 1`, orgID).Scan(&rawPolicy); err != nil {
		t.Fatalf("read policy: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(rawPolicy, &decoded); err != nil {
		t.Fatalf("decode policy: %v", err)
	}
	if decoded["purpose"] != "Answer billing questions and escalate refunds." {
		t.Fatalf("policy purpose not saved: %v", decoded["purpose"])
	}
	if !strings.Contains(string(rawPolicy), "approval_required") {
		t.Fatalf("policy tool mode not saved: %s", rawPolicy)
	}

	invite := app.postForm(t, "/dashboard/settings/invite", url.Values{"email": {"robin@product.test"}, "role": {"agent"}}, jar)
	if invite.status != http.StatusCreated || !strings.Contains(string(invite.body), "Invite link ready.") || !strings.Contains(string(invite.body), "/invite/") {
		t.Fatalf("invite create response invalid: %d %s", invite.status, invite.body)
	}
	var invitationID string
	if err := app.pool.QueryRow(ctx, `select id from invitation where organization_id = $1 and email = 'robin@product.test'`, orgID).Scan(&invitationID); err != nil {
		t.Fatalf("invitation row: %v", err)
	}
	invitePage := pageBody(t, app.getPage(t, "/invite/"+invitationID, newTestJar()))
	for _, needle := range []string{"robin@product.test", "Product Org", "Accept invitation", "Agent"} {
		if !strings.Contains(invitePage, needle) {
			t.Fatalf("invite page missing %q", needle)
		}
	}
	robinJar := newTestJar()
	accepted := app.postForm(t, "/invite/"+invitationID, url.Values{"name": {"Robin Vale"}, "password": {"password123"}}, robinJar)
	if accepted.status != http.StatusSeeOther {
		t.Fatalf("invite accept: %d %s", accepted.status, accepted.body)
	}
	robinDashboard := pageBody(t, app.getPage(t, "/dashboard", robinJar))
	if !strings.Contains(robinDashboard, "Robin Vale") {
		t.Fatal("accepted member must have a working session")
	}
	robinAccount := pageBody(t, app.getPage(t, "/dashboard/account", robinJar))
	if !strings.Contains(robinAccount, "Save account") || !strings.Contains(robinAccount, "Password") {
		t.Fatal("agent must be able to reach account settings")
	}
	var memberRole string
	if err := app.pool.QueryRow(ctx, `select m.role from member m join "user" u on u.id = m.user_id where m.organization_id = $1 and u.email = 'robin@product.test'`, orgID).Scan(&memberRole); err != nil {
		t.Fatalf("member role: %v", err)
	}
	if memberRole != "agent" {
		t.Fatalf("invited member role = %q, want agent", memberRole)
	}
	reused := app.postForm(t, "/invite/"+invitationID, url.Values{"name": {"Robin Again"}, "password": {"password123"}}, newTestJar())
	if reused.status != http.StatusNotFound {
		t.Fatalf("used invitation must be refused, got %d", reused.status)
	}

	keyPage := app.postForm(t, "/dashboard/settings/api-keys", url.Values{"name": {"Warehouse sync"}}, jar)
	if keyPage.status != http.StatusCreated || !strings.Contains(string(keyPage.body), "Copy this key now.") || !strings.Contains(string(keyPage.body), "cd_") {
		t.Fatalf("api key create response invalid: %d %s", keyPage.status, keyPage.body)
	}
	var keyID, keyHash string
	if err := app.pool.QueryRow(ctx, `select id::text, key_hash from api_keys where org_id = $1`, orgID).Scan(&keyID, &keyHash); err != nil {
		t.Fatalf("api key row: %v", err)
	}
	if len(keyHash) != 64 {
		t.Fatalf("api key must be stored as a sha256 hex hash, got %q", keyHash)
	}
	revoked := app.postForm(t, "/dashboard/settings/api-keys/revoke", url.Values{"keyId": {keyID}}, jar)
	if revoked.status != http.StatusSeeOther {
		t.Fatalf("api key revoke: %d %s", revoked.status, revoked.body)
	}

	account := app.postForm(t, "/dashboard/settings/account", url.Values{
		"name": {"Owner Renamed"}, "currentPassword": {"password123"}, "newPassword": {"newpassword123"},
	}, jar)
	if account.status != http.StatusSeeOther {
		t.Fatalf("account save: %d %s", account.status, account.body)
	}
	newLogin := app.request("POST", "/api/auth/sign-in/email", map[string]any{"email": "owner@product.test", "password": "newpassword123"}, newTestJar(), "")
	if newLogin.status != http.StatusOK {
		t.Fatalf("new password must sign in: %d %s", newLogin.status, newLogin.body)
	}
	oldLogin := app.request("POST", "/api/auth/sign-in/email", map[string]any{"email": "owner@product.test", "password": "password123"}, newTestJar(), "")
	if oldLogin.status != http.StatusUnauthorized {
		t.Fatalf("old password must fail: %d %s", oldLogin.status, oldLogin.body)
	}

	workspace := app.postForm(t, "/dashboard/settings/workspace", url.Values{"orgName": {"Renamed Product Org"}}, jar)
	if workspace.status != http.StatusSeeOther {
		t.Fatalf("workspace save: %d %s", workspace.status, workspace.body)
	}
	var orgName string
	if err := app.pool.QueryRow(ctx, `select name from organization where id = $1`, orgID).Scan(&orgName); err != nil {
		t.Fatalf("org name: %v", err)
	}
	if orgName != "Renamed Product Org" {
		t.Fatalf("workspace name not saved: %q", orgName)
	}
}

func TestSecurityHeadersAndOriginChecks(t *testing.T) {
	app := startWorkerTestApp(t, t.TempDir())
	jar, _ := app.bootstrapAndSignIn(t, "Security Org", "owner@security.test")

	req, err := http.NewRequest(http.MethodGet, app.server.URL+"/dashboard", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Cookie", jar.header())
	res, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("dashboard status: %d", res.StatusCode)
	}
	for header, want := range map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"X-Frame-Options":         "DENY",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
	} {
		if !strings.Contains(res.Header.Get(header), want) {
			t.Fatalf("%s = %q, want %q", header, res.Header.Get(header), want)
		}
	}

	crossOrigin, err := http.NewRequest(http.MethodPost, app.server.URL+"/dashboard/settings/agent",
		strings.NewReader("purpose=evil"))
	if err != nil {
		t.Fatalf("cross origin request: %v", err)
	}
	crossOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	crossOrigin.Header.Set("Cookie", jar.header())
	crossOrigin.Header.Set("Origin", "https://evil.example")
	blocked, err := noRedirectClient.Do(crossOrigin)
	if err != nil {
		t.Fatalf("cross origin post: %v", err)
	}
	defer blocked.Body.Close()
	if blocked.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin POST must be refused, got %d", blocked.StatusCode)
	}

	ready := app.request("GET", "/api/ready", nil, jar, "")
	if ready.status != http.StatusOK {
		t.Fatalf("ready endpoint: %d %s", ready.status, ready.body)
	}

	setWidgetOrigins(t, app, jar)
	token := widgetTokenFor(t, app, jar)
	preflight, err := http.NewRequest(http.MethodOptions, app.server.URL+"/api/widget/config?token="+url.QueryEscape(token), nil)
	if err != nil {
		t.Fatalf("preflight request: %v", err)
	}
	preflight.Header.Set("Origin", "https://shop.example.com")
	preflight.Header.Set("Access-Control-Request-Method", "GET")
	preflightRes, err := noRedirectClient.Do(preflight)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	defer preflightRes.Body.Close()
	if preflightRes.StatusCode != http.StatusNoContent || preflightRes.Header.Get("Access-Control-Allow-Origin") != "https://shop.example.com" {
		t.Fatalf("widget preflight invalid: %d %q", preflightRes.StatusCode, preflightRes.Header.Get("Access-Control-Allow-Origin"))
	}

	asset, err := http.Get(app.server.URL + "/static/htmx.min.js")
	if err != nil {
		t.Fatalf("static request: %v", err)
	}
	defer asset.Body.Close()
	assetBody, _ := io.ReadAll(asset.Body)
	if asset.StatusCode != http.StatusOK || len(assetBody) < 20000 || !strings.Contains(asset.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("htmx asset invalid: %d %q %d bytes", asset.StatusCode, asset.Header.Get("Content-Type"), len(assetBody))
	}
}
