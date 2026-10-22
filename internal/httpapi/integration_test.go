// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/config"
	"chastedesk/internal/db"
	"chastedesk/internal/testutil"
)

type testApp struct {
	t      *testing.T
	pool   *pgxpool.Pool
	server *httptest.Server
}

func startTestApp(t *testing.T) *testApp {
	t.Helper()
	dsn := testutil.StartEphemeralPG(t)
	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := config.Config{
		Port:                0,
		AppURL:              "https://127.0.0.1:4433",
		DatabaseURL:         dsn,
		AppSecret:           strings.Repeat("s", 32),
		LogLevel:            "error",
		EmailInboundAddress: "support@chastedesk.test",
		EmailInboundSecret:  "test-inbound-email-secret",
	}
	srv := httptest.NewServer(New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), pool))
	t.Cleanup(srv.Close)
	return &testApp{t: t, pool: pool, server: srv}
}

type testJar struct {
	cookies map[string]string
}

func newTestJar() *testJar {
	return &testJar{cookies: map[string]string{}}
}

func (j *testJar) absorb(res *http.Response) {
	for _, header := range res.Header.Values("Set-Cookie") {
		pair := strings.SplitN(header, ";", 2)[0]
		eq := strings.Index(pair, "=")
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(pair[:eq])
		value := strings.TrimSpace(pair[eq+1:])
		if value == "" {
			delete(j.cookies, name)
		} else {
			j.cookies[name] = value
		}
	}
}

func (j *testJar) header() string {
	parts := make([]string, 0, len(j.cookies))
	for name, value := range j.cookies {
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, "; ")
}

func (j *testJar) tokenMatching(sub string) (string, bool) {
	for name, value := range j.cookies {
		if strings.Contains(name, sub) {
			return value, true
		}
	}
	return "", false
}

type testResponse struct {
	status     int
	body       []byte
	setCookies []string
}

func (a *testApp) request(method, path string, body any, jar *testJar, rawCookie string) testResponse {
	a.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			a.t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, a.server.URL+path, reader)
	if err != nil {
		a.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if jar != nil {
		req.Header.Set("Cookie", jar.header())
	}
	if rawCookie != "" {
		req.Header.Set("Cookie", rawCookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		a.t.Fatalf("read body: %v", err)
	}
	if jar != nil {
		jar.absorb(res)
	}
	return testResponse{status: res.StatusCode, body: data, setCookies: res.Header.Values("Set-Cookie")}
}

func (r testResponse) jsonBody(t *testing.T) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(r.body, &out); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, string(r.body))
	}
	return out
}

func (r testResponse) data(t *testing.T) map[string]any {
	t.Helper()
	body := r.jsonBody(t)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data envelope: %v", body)
	}
	return data
}

func (r testResponse) errorCode(t *testing.T) string {
	t.Helper()
	body := r.jsonBody(t)
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error envelope: %v", body)
	}
	code, _ := errObj["code"].(string)
	return code
}
