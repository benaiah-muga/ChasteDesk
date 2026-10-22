// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func (a *testApp) scimRequest(method, path, token string, body any) testResponse {
	a.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			a.t.Fatalf("marshal SCIM body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, a.server.URL+path, reader)
	if err != nil {
		a.t.Fatalf("new SCIM request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/scim+json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		a.t.Fatalf("read SCIM body: %v", err)
	}
	return testResponse{status: res.StatusCode, body: data}
}

func scimBody(t *testing.T, response testResponse) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(response.body, &out); err != nil {
		t.Fatalf("SCIM body is not JSON: %v (%s)", err, response.body)
	}
	return out
}

func TestSCIMProvisioning(t *testing.T) {
	app := startWorkerTestApp(t, t.TempDir())
	jar := newTestJar()
	setup := app.request("POST", "/api/app/setup", map[string]any{
		"orgName": "SCIM Org", "userName": "SCIM Owner", "email": "owner@scim.test", "password": "password123",
	}, jar, "")
	if setup.status != http.StatusCreated {
		t.Fatalf("setup: %d %s", setup.status, setup.body)
	}
	var orgID string
	if err := app.pool.QueryRow(t.Context(), `select m.organization_id from member m join "user" u on u.id = m.user_id where u.email = 'owner@scim.test'`).Scan(&orgID); err != nil {
		t.Fatalf("find SCIM org: %v", err)
	}
	tokenResponse := app.request("POST", "/api/app/settings/scim/tokens", map[string]any{"name": "Test IdP"}, jar, "")
	if tokenResponse.status != http.StatusCreated {
		t.Fatalf("SCIM token create: %d %s", tokenResponse.status, tokenResponse.body)
	}
	token, _ := tokenResponse.data(t)["token"].(string)
	if !strings.HasPrefix(token, "cd_scim_") {
		t.Fatalf("SCIM token missing prefix: %s", tokenResponse.body)
	}
	settings := app.request("GET", "/api/app/settings", nil, jar, "")
	scimSettings, _ := settings.data(t)["scim"].(map[string]any)
	if scimSettings["baseUrl"] == nil || len(scimSettings["tokens"].([]any)) != 1 {
		t.Fatalf("SCIM settings missing configuration: %s", settings.body)
	}

	service := app.scimRequest("GET", "/scim/v2/ServiceProviderConfig", token, nil)
	if service.status != http.StatusOK || !strings.Contains(string(service.body), "oauthbearertoken") {
		t.Fatalf("service provider config: %d %s", service.status, service.body)
	}
	unauthorized := app.scimRequest("GET", "/scim/v2/Users", "bad-token", nil)
	if unauthorized.status != http.StatusUnauthorized {
		t.Fatalf("bad SCIM token must fail: %d %s", unauthorized.status, unauthorized.body)
	}

	created := app.scimRequest("POST", "/scim/v2/Users", token, map[string]any{
		"schemas":  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"userName": "robin@scim.test", "externalId": "idp-robin", "active": true,
		"name":  map[string]any{"givenName": "Robin", "familyName": "Vale"},
		"roles": []map[string]any{{"value": "agent"}},
	})
	if created.status != http.StatusCreated {
		t.Fatalf("SCIM user create: %d %s", created.status, created.body)
	}
	scimUser := scimBody(t, created)
	scimID, _ := scimUser["id"].(string)
	if scimID == "" || scimUser["userName"] != "robin@scim.test" {
		t.Fatalf("SCIM user payload: %s", created.body)
	}
	list := app.scimRequest("GET", "/scim/v2/Users?filter="+url.QueryEscape(`userName eq "robin@scim.test"`), token, nil)
	if list.status != http.StatusOK || !strings.Contains(string(list.body), "robin@scim.test") {
		t.Fatalf("SCIM filtered list: %d %s", list.status, list.body)
	}

	var invitationID string
	if err := app.pool.QueryRow(t.Context(), `select id from invitation where organization_id = $1 and email = 'robin@scim.test'`, orgID).Scan(&invitationID); err != nil {
		t.Fatalf("SCIM invitation: %v", err)
	}
	robinJar := newTestJar()
	accepted := app.request("POST", "/api/app/invitations/"+invitationID+"/accept", map[string]any{"name": "Robin Vale", "password": "password123"}, robinJar, "")
	if accepted.status != http.StatusCreated {
		t.Fatalf("activate SCIM user: %d %s", accepted.status, accepted.body)
	}
	signIn := app.request("POST", "/api/auth/sign-in/email", map[string]any{"email": "robin@scim.test", "password": "password123"}, newTestJar(), "")
	if signIn.status != http.StatusOK {
		t.Fatalf("activated SCIM user sign-in: %d %s", signIn.status, signIn.body)
	}

	deactivated := app.scimRequest("PATCH", "/scim/v2/Users/"+scimID, token, map[string]any{
		"schemas":    []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []map[string]any{{"op": "replace", "path": "active", "value": false}},
	})
	if deactivated.status != http.StatusOK || scimBody(t, deactivated)["active"] != false {
		t.Fatalf("SCIM deactivate: %d %s", deactivated.status, deactivated.body)
	}
	robinSession := app.request("GET", "/api/app/session", nil, robinJar, "")
	if robinSession.data(t)["authenticated"] != false {
		t.Fatalf("SCIM deactivation must revoke sessions: %s", robinSession.body)
	}
	var membershipCount int
	if err := app.pool.QueryRow(t.Context(), `select count(*)::int from member where organization_id = $1 and user_id = (select id from "user" where email = 'robin@scim.test')`, orgID).Scan(&membershipCount); err != nil || membershipCount != 0 {
		t.Fatalf("SCIM deactivation must remove membership: count=%d err=%v", membershipCount, err)
	}

	reactivated := app.scimRequest("PATCH", "/scim/v2/Users/"+scimID, token, map[string]any{
		"Operations": []map[string]any{{"op": "replace", "path": "active", "value": true}},
	})
	if reactivated.status != http.StatusOK {
		t.Fatalf("SCIM reactivate: %d %s", reactivated.status, reactivated.body)
	}
	roleChanged := app.scimRequest("PATCH", "/scim/v2/Users/"+scimID, token, map[string]any{
		"Operations": []map[string]any{{"op": "replace", "path": "roles", "value": []map[string]any{{"value": "admin"}}}},
	})
	if roleChanged.status != http.StatusOK || scimBody(t, roleChanged)["roles"].([]any)[0].(map[string]any)["value"] != "admin" {
		t.Fatalf("SCIM role change: %d %s", roleChanged.status, roleChanged.body)
	}
	var role string
	if err := app.pool.QueryRow(t.Context(), `select role from member where organization_id = $1 and user_id = (select id from "user" where email = 'robin@scim.test')`, orgID).Scan(&role); err != nil || role != "admin" {
		t.Fatalf("SCIM role mapping: role=%s err=%v", role, err)
	}
	deleted := app.scimRequest("DELETE", "/scim/v2/Users/"+scimID, token, nil)
	if deleted.status != http.StatusNoContent {
		t.Fatalf("SCIM delete: %d %s", deleted.status, deleted.body)
	}

	revoked := app.request("DELETE", "/api/app/settings/scim/tokens/"+tokenResponse.data(t)["id"].(string), nil, jar, "")
	if revoked.status != http.StatusOK {
		t.Fatalf("SCIM token revoke: %d %s", revoked.status, revoked.body)
	}
	afterRevoke := app.scimRequest("GET", "/scim/v2/Users", token, nil)
	if afterRevoke.status != http.StatusUnauthorized {
		t.Fatalf("revoked SCIM token must fail: %d %s", afterRevoke.status, afterRevoke.body)
	}
}
