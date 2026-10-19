// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"net/http"
	"testing"
)

func TestLoginRateLimitLockout(t *testing.T) {
	app := startTestApp(t)

	boot := app.request(http.MethodPost, "/api/bootstrap", map[string]any{
		"orgName": "Rate Org", "userName": "R", "email": "owner@rate.test", "password": "password123",
	}, nil, "")
	if boot.status != http.StatusCreated {
		t.Fatalf("bootstrap failed: %d %s", boot.status, boot.body)
	}

	first := app.request(http.MethodPost, "/api/auth/sign-in/email", map[string]any{
		"email": "owner@rate.test", "password": "wrong-password",
	}, nil, "")
	if first.status == http.StatusTooManyRequests {
		t.Fatal("first failed login must not be rate limited")
	}

	saw429 := false
	for i := 0; i < 12; i++ {
		res := app.request(http.MethodPost, "/api/auth/sign-in/email", map[string]any{
			"email": "owner@rate.test", "password": "wrong-password",
		}, nil, "")
		if res.status == http.StatusTooManyRequests {
			saw429 = true
			break
		}
	}
	if !saw429 {
		t.Fatal("lockout never triggered after repeated failed logins")
	}

	correctDuringLock := app.request(http.MethodPost, "/api/auth/sign-in/email", map[string]any{
		"email": "owner@rate.test", "password": "password123",
	}, nil, "")
	if correctDuringLock.status != http.StatusTooManyRequests {
		t.Fatalf("correct credentials during lockout must 429, got %d", correctDuringLock.status)
	}
}
