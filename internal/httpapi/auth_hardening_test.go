// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chastedesk/internal/config"
)

func TestProductionSignUpDisabled(t *testing.T) {
	server := &Server{cfg: config.Config{AppEnv: "production"}}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/sign-up/email", strings.NewReader(`{"name":"A","email":"a@example.test","password":"password123"}`))
	response := httptest.NewRecorder()
	server.handleSignUp(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("production sign-up status = %d, want 403", response.Code)
	}
}
