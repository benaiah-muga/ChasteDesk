// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"chastedesk/internal/agent"
	"chastedesk/internal/crypto"
	"chastedesk/internal/id"
	"chastedesk/internal/retrieval"
	"chastedesk/internal/testutil"
)

const testAppSecret = "ssssssssssssssssssssssssssssssss"

func startP9App(t *testing.T) *workerTestApp {
	t.Helper()
	app := startWorkerTestApp(t, t.TempDir())
	engine := retrieval.NewEngine(app.pool)
	app.jobs.Work(app.ctx, "ai-respond", 2, agent.RespondHandler(app.pool, testAppSecret, engine, nil))
	app.jobs.Work(app.ctx, "execute-action", 2, agent.ExecuteActionHandler(app.pool, engine))
	return app
}

func rawRequest(t *testing.T, method, url string, body any, headers map[string]string) testResponse {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return testResponse{status: res.StatusCode, body: data}
}

func widgetTokenFor(t *testing.T, app *workerTestApp, jar *testJar) string {
	t.Helper()
	res := app.request("GET", "/api/settings/widget-token", nil, jar, "")
	if res.status != http.StatusOK {
		t.Fatalf("widget token fetch: %d %s", res.status, res.body)
	}
	token, _ := res.data(t)["token"].(string)
	if token == "" {
		t.Fatalf("empty widget token")
	}
	return token
}

func setWidgetOrigins(t *testing.T, app *workerTestApp, jar *testJar) {
	t.Helper()
	res := app.request("PUT", "/api/settings/widget", map[string]any{
		"origins": []string{"https://shop.example.com"},
	}, jar, "")
	if res.status != http.StatusOK {
		t.Fatalf("widget origins save: %d %s", res.status, res.body)
	}
}

func TestWidgetOriginGate(t *testing.T) {
	app := startP9App(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Widget Org", "owner@widget.test")
	setWidgetOrigins(t, app, jar)
	token := widgetTokenFor(t, app, jar)

	base := app.server.URL + "/api/widget/config?token=" + token
	good := rawRequest(t, "GET", base, nil, map[string]string{"Origin": "https://shop.example.com"})
	if good.status != http.StatusOK {
		t.Fatalf("valid origin+token must pass: %d %s", good.status, good.body)
	}

	bad := rawRequest(t, "GET", base, nil, map[string]string{"Origin": "https://evil.example.net"})
	if bad.status != http.StatusForbidden {
		t.Fatalf("disallowed origin must 403: %d", bad.status)
	}

	noOrigin := rawRequest(t, "GET", base, nil, nil)
	if noOrigin.status != http.StatusForbidden {
		t.Fatalf("absent origin must 403: %d", noOrigin.status)
	}

	tamperedToken := token[:len(token)-3] + "abc"
	tampered := rawRequest(t, "GET", app.server.URL+"/api/widget/config?token="+tamperedToken, nil,
		map[string]string{"Origin": "https://shop.example.com"})
	if tampered.status != http.StatusUnauthorized {
		t.Fatalf("tampered token must 401: %d", tampered.status)
	}

	key, err := crypto.DeriveSubkey(testAppSecret, "widget-"+orgID)
	if err != nil {
		t.Fatalf("derive widget key: %v", err)
	}
	dot := strings.Index(token, ".")
	decoded, err := base64.RawURLEncoding.DecodeString(token[:dot])
	if err != nil {
		t.Fatalf("decode token body: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		t.Fatalf("parse token body: %v", err)
	}
	payload["exp"] = 1000000000
	body, _ := json.Marshal(payload)
	expiredBody := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(expiredBody))
	expiredToken := expiredBody + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	expired := rawRequest(t, "GET", app.server.URL+"/api/widget/config?token="+expiredToken, nil,
		map[string]string{"Origin": "https://shop.example.com"})
	if expired.status != http.StatusUnauthorized {
		t.Fatalf("expired token must 401: %d", expired.status)
	}
}

func TestWidgetFlowGate(t *testing.T) {
	app := startP9App(t)
	mock := testutil.StartMockOpenAI(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Flow Org", "owner@flow2.test")

	save := app.request("POST", "/api/settings/providers", map[string]any{
		"providerId": "openai", "displayName": "Mock", "apiKey": "test-key", "baseUrl": mock.URL,
	}, jar, "")
	if save.status != http.StatusCreated {
		t.Fatalf("provider save: %d %s", save.status, save.body)
	}
	mock.SetScript(testutil.MockOpenAIStep{
		Text: "You can reset your password from the security settings page. [E1]", InputTokens: 9, OutputTokens: 9, Finish: "stop",
	})

	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><head><title>Help</title></head><body><h1>Help topic</h1><p>To reset your password use the security settings page and pick a new credential.</p></body></html>")
	}))
	t.Cleanup(site.Close)

	createSource := app.request("POST", "/api/knowledge/sources", map[string]any{
		"kind": "url", "url": site.URL, "scope": "public",
	}, jar, "")
	if createSource.status != http.StatusCreated {
		t.Fatalf("knowledge source create: %d %s", createSource.status, createSource.body)
	}
	ready := waitFor(t, 40*time.Second, func() bool {
		var count int
		_ = app.pool.QueryRow(context.Background(),
			`select count(*)::int from knowledge_sources where org_id = $1 and status = 'ready'`, orgID).Scan(&count)
		return count > 0
	})
	if !ready {
		t.Fatalf("knowledge source never reached ready")
	}

	setWidgetOrigins(t, app, jar)
	widgetToken := widgetTokenFor(t, app, jar)

	create := rawRequest(t, "POST", app.server.URL+"/api/widget/conversations?token="+widgetToken,
		map[string]any{"firstMessage": "How do I reset my password?"},
		map[string]string{"Origin": "https://shop.example.com"})
	if create.status != http.StatusCreated {
		t.Fatalf("widget conversation create: %d %s", create.status, create.body)
	}
	created := create.jsonBody(t)["data"].(map[string]any)
	conversationID, _ := created["conversationId"].(string)
	convToken, _ := created["token"].(string)

	streamURL := app.server.URL + "/api/widget/conversations/" + conversationID + "/stream?token=" + widgetToken + "&convToken=" + convToken
	reply := readWidgetReply(t, streamURL, 30*time.Second)
	if reply == "" {
		t.Fatalf("no agent reply arrived on the widget stream")
	}
	if !strings.Contains(reply, "security settings") {
		t.Fatalf("reply content mismatch: %s", reply)
	}

	ctx := context.Background()
	var agentMessages int
	if err := app.pool.QueryRow(ctx,
		`select count(*)::int from messages where conversation_id = $1 and role = 'agent_ai' and content like '%reset%'`,
		conversationID).Scan(&agentMessages); err != nil {
		t.Fatalf("count agent messages: %v", err)
	}
	if agentMessages == 0 {
		t.Fatalf("agent reply not persisted")
	}
	var firstResponse *time.Time
	if err := app.pool.QueryRow(ctx,
		`select first_response_at from conversations where id = $1`, conversationID).Scan(&firstResponse); err != nil {
		t.Fatalf("load first_response_at: %v", err)
	}
	if firstResponse == nil {
		t.Fatalf("first_response_at not set")
	}
}

func readWidgetReply(t *testing.T, streamURL string, timeout time.Duration) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	req.Header.Set("Origin", "https://shop.example.com")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	reader := bufio.NewReader(res.Body)
	var frameLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return ""
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			for _, candidate := range frameLines {
				if !strings.HasPrefix(candidate, "data: ") {
					continue
				}
				var data map[string]any
				if err := json.Unmarshal([]byte(strings.TrimPrefix(candidate, "data: ")), &data); err != nil {
					continue
				}
				if content, ok := data["content"].(string); ok && strings.Contains(content, "reset") {
					return content
				}
			}
			frameLines = frameLines[:0]
			continue
		}
		frameLines = append(frameLines, line)
	}
}

func TestWidgetRateLimitGate(t *testing.T) {
	app := startP9App(t)
	jar, _ := app.bootstrapAndSignIn(t, "Rate Widget Org", "owner@rw.test")
	setWidgetOrigins(t, app, jar)
	widgetToken := widgetTokenFor(t, app, jar)

	create := rawRequest(t, "POST", app.server.URL+"/api/widget/conversations?token="+widgetToken,
		map[string]any{}, map[string]string{"Origin": "https://shop.example.com"})
	if create.status != http.StatusCreated {
		t.Fatalf("conversation create: %d %s", create.status, create.body)
	}
	created := create.jsonBody(t)["data"].(map[string]any)
	conversationID, _ := created["conversationId"].(string)
	convToken, _ := created["token"].(string)
	url := app.server.URL + "/api/widget/conversations/" + conversationID + "/messages?token=" + widgetToken + "&convToken=" + convToken

	saw429 := false
	for i := 0; i < 25; i++ {
		res := rawRequest(t, "POST", url, map[string]any{"content": fmt.Sprintf("flood %d", i)},
			map[string]string{"Origin": "https://shop.example.com"})
		if res.status == http.StatusTooManyRequests {
			saw429 = true
			break
		}
		if res.status != http.StatusAccepted {
			t.Fatalf("unexpected send status %d: %s", res.status, res.body)
		}
	}
	if !saw429 {
		t.Fatalf("per-conversation rate limit never triggered")
	}

	var count int
	err := app.pool.QueryRow(context.Background(),
		`select count from rate_limit_windows where key like 'widget:msg-ip:%' order by count desc limit 1`).Scan(&count)
	if err != nil {
		t.Fatalf("per-IP accounting missing: %v", err)
	}
	if count < 20 {
		t.Fatalf("per-IP accounting too low: %d", count)
	}
}

func TestAPIChannelGate(t *testing.T) {
	app := startP9App(t)
	_, orgID := app.bootstrapAndSignIn(t, "API Org", "owner@api.test")
	ctx := context.Background()

	plaintext := "cd_gatelivekey0001"
	sum := sha256.Sum256([]byte(plaintext))
	if _, err := app.pool.Exec(ctx,
		`insert into api_keys (id, org_id, name, key_prefix, key_hash) values ($1, $2, 'Gate key', $3, $4)`,
		id.UUID(), orgID, plaintext[:8], hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("insert api key: %v", err)
	}
	auth := map[string]string{"Authorization": "Bearer " + plaintext}

	create := rawRequest(t, "POST", app.server.URL+"/api/v1/conversations", map[string]any{
		"customer": map[string]any{"email": "external@corp.test", "name": "External Corp"},
		"message":  "Hello from the external system.",
	}, auth)
	if create.status != http.StatusCreated {
		t.Fatalf("api create failed: %d %s", create.status, create.body)
	}
	conversationID, _ := create.jsonBody(t)["data"].(map[string]any)["conversationId"].(string)

	send := rawRequest(t, "POST", app.server.URL+"/api/v1/conversations/"+conversationID+"/messages",
		map[string]any{"content": "Second message from the external system."}, auth)
	if send.status != http.StatusAccepted {
		t.Fatalf("api send failed: %d %s", send.status, send.body)
	}

	wrong := rawRequest(t, "POST", app.server.URL+"/api/v1/conversations", map[string]any{
		"customer": map[string]any{"email": "x@y.test"}, "message": "hi",
	}, map[string]string{"Authorization": "Bearer cd_wrongkey0000000000"})
	if wrong.status != http.StatusUnauthorized {
		t.Fatalf("wrong key must 401: %d", wrong.status)
	}

	revoked := "cd_revokedkey0001"
	revokedSum := sha256.Sum256([]byte(revoked))
	if _, err := app.pool.Exec(ctx,
		`insert into api_keys (id, org_id, name, key_prefix, key_hash) values ($1, $2, 'Revoked', $3, $4)`,
		id.UUID(), orgID, revoked[:8], hex.EncodeToString(revokedSum[:])); err != nil {
		t.Fatalf("insert revoked key: %v", err)
	}
	if _, err := app.pool.Exec(ctx, `delete from api_keys where key_hash = $1`, hex.EncodeToString(revokedSum[:])); err != nil {
		t.Fatalf("delete revoked key: %v", err)
	}
	revokedRes := rawRequest(t, "POST", app.server.URL+"/api/v1/conversations", map[string]any{
		"customer": map[string]any{"email": "x@y.test"}, "message": "hi",
	}, map[string]string{"Authorization": "Bearer " + revoked})
	if revokedRes.status != http.StatusUnauthorized {
		t.Fatalf("deleted key must 401: %d", revokedRes.status)
	}

	var apiConversations int
	if err := app.pool.QueryRow(ctx,
		`select count(*)::int from conversations where org_id = $1 and channel = 'api'`, orgID).Scan(&apiConversations); err != nil {
		t.Fatalf("count conversations: %v", err)
	}
	if apiConversations != 1 {
		t.Fatalf("api conversation not created: %d", apiConversations)
	}
}
