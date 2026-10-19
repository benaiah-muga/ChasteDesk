// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestAuthFlow(t *testing.T) {
	app := startTestApp(t)

	boot := app.request(http.MethodPost, "/api/bootstrap", map[string]any{
		"orgName": "Flow Org", "userName": "Owner", "email": "owner@flow.test", "password": "password123",
	}, nil, "")
	if boot.status != http.StatusCreated {
		t.Fatalf("bootstrap failed: %d %s", boot.status, boot.body)
	}

	jar := newTestJar()
	signup := app.request(http.MethodPost, "/api/auth/sign-up/email", map[string]any{
		"name": "Agent One", "email": "agent@flow.test", "password": "password123",
	}, jar, "")
	if signup.status != http.StatusOK && signup.status != http.StatusCreated {
		t.Fatalf("signup failed: %d %s", signup.status, signup.body)
	}
	foundSessionCookie := false
	for name := range jar.cookies {
		if strings.Contains(name, "session") {
			foundSessionCookie = true
		}
	}
	if !foundSessionCookie {
		t.Fatalf("no session cookie set on signup: %v", jar.cookies)
	}

	session1 := app.request(http.MethodGet, "/api/auth/get-session", nil, jar, "")
	user1, ok := session1.jsonBody(t)["user"].(map[string]any)
	if !ok || user1["email"] != "agent@flow.test" {
		t.Fatalf("get-session did not return signed-up user: %s", session1.body)
	}

	login := app.request(http.MethodPost, "/api/auth/sign-in/email", map[string]any{
		"email": "owner@flow.test", "password": "password123",
	}, nil, "")
	var sessionCookieHeader string
	for _, header := range login.setCookies {
		lower := strings.ToLower(header)
		if strings.Contains(lower, "session") && strings.Contains(lower, "httponly") {
			sessionCookieHeader = lower
		}
	}
	if sessionCookieHeader == "" {
		t.Fatalf("login set-cookie missing HttpOnly session cookie: %v", login.setCookies)
	}
	if !strings.Contains(sessionCookieHeader, "samesite=lax") {
		t.Errorf("session cookie must be SameSite=Lax: %s", sessionCookieHeader)
	}
	if !strings.Contains(sessionCookieHeader, "secure") {
		t.Errorf("session cookie must be Secure when APP_URL is https: %s", sessionCookieHeader)
	}

	ownerJar := newTestJar()
	for _, header := range login.setCookies {
		pair := strings.SplitN(header, ";", 2)[0]
		eq := strings.Index(pair, "=")
		if eq > 0 {
			ownerJar.cookies[strings.TrimSpace(pair[:eq])] = strings.TrimSpace(pair[eq+1:])
		}
	}
	ownerSession := app.request(http.MethodGet, "/api/auth/get-session", nil, ownerJar, "")
	ownerUser, ok := ownerSession.jsonBody(t)["user"].(map[string]any)
	if !ok || ownerUser["email"] != "owner@flow.test" {
		t.Fatalf("owner login session mismatch: %s", ownerSession.body)
	}

	tokenBefore, ok := ownerJar.tokenMatching("session_token")
	if !ok {
		t.Fatalf("no session token captured: %v", ownerJar.cookies)
	}

	logout := app.request(http.MethodPost, "/api/auth/sign-out", map[string]any{}, ownerJar, "")
	if logout.status != http.StatusOK {
		t.Fatalf("sign-out failed: %d", logout.status)
	}

	postLogout := app.request(http.MethodGet, "/api/auth/get-session", nil, nil,
		"__Secure-better-auth.session_token="+tokenBefore)
	if postLogout.status != http.StatusOK {
		t.Fatalf("post-logout get-session status = %d", postLogout.status)
	}
	postBody := postLogout.jsonBody(t)
	if postBody["user"] != nil && postBody["session"] != nil {
		t.Errorf("old session token still valid after sign-out: %s", postLogout.body)
	}

	badLogin := app.request(http.MethodPost, "/api/auth/sign-in/email", map[string]any{
		"email": "owner@flow.test", "password": "wrong-password",
	}, nil, "")
	if badLogin.status != http.StatusUnauthorized && badLogin.status != http.StatusBadRequest && badLogin.status != http.StatusTooManyRequests {
		t.Fatalf("unexpected bad login status %d", badLogin.status)
	}
}
