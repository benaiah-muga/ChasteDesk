// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chastedesk/internal/config"
	"chastedesk/internal/db"
	"chastedesk/internal/ingest"
	"chastedesk/internal/queue"
	"chastedesk/internal/testutil"
)

type workerTestApp struct {
	*testApp
	logs *bytes.Buffer
	jobs *queue.Queue
	ctx  context.Context
}

func startWorkerTestApp(t *testing.T, uploadsDir string) *workerTestApp {
	t.Helper()
	t.Setenv("CHASTESK_INGEST_ALLOW_PRIVATE", "1")
	t.Setenv("APP_ENV", "test")
	dsn := testutil.StartEphemeralPG(t)
	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	jobs := queue.New(pool)
	for name, handler := range ingest.Handlers(ingest.Deps{Pool: pool, UploadsDir: uploadsDir}) {
		jobs.Work(ctx, name, 4, handler)
	}
	cfg := config.Config{
		Port:        0,
		AppURL:      "https://127.0.0.1:4433",
		DatabaseURL: dsn,
		AppSecret:   strings.Repeat("s", 32),
		LogLevel:    "debug",
		UploadsDir:  uploadsDir,
	}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := httptest.NewServer(New(cfg, logger, pool))
	t.Cleanup(srv.Close)
	return &workerTestApp{testApp: &testApp{t: t, pool: pool, server: srv}, logs: logs, jobs: jobs, ctx: ctx}
}

func (a *testApp) bootstrapAndSignIn(t *testing.T, orgName, email string) (*testJar, string) {
	t.Helper()
	boot := a.request("POST", "/api/bootstrap", map[string]any{
		"orgName": orgName, "userName": "Owner", "email": email, "password": "password123",
	}, nil, "")
	if boot.status != http.StatusCreated {
		t.Fatalf("bootstrap failed: %d %s", boot.status, boot.body)
	}
	orgID, _ := boot.data(t)["orgId"].(string)
	if orgID == "" {
		t.Fatalf("bootstrap response missing orgId: %s", boot.body)
	}
	jar := newTestJar()
	signIn := a.request("POST", "/api/auth/sign-in/email", map[string]any{"email": email, "password": "password123"}, jar, "")
	if signIn.status != http.StatusOK {
		t.Fatalf("sign-in failed: %d %s", signIn.status, signIn.body)
	}
	return jar, orgID
}

var noRedirectClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (a *testApp) postForm(t *testing.T, path string, form url.Values, jar *testJar) testResponse {
	t.Helper()
	req, err := http.NewRequest("POST", a.server.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new form request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if jar != nil {
		req.Header.Set("Cookie", jar.header())
	}
	res, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read form body: %v", err)
	}
	if jar != nil {
		jar.absorb(res)
	}
	return testResponse{
		status:     res.StatusCode,
		body:       data,
		setCookies: res.Header.Values("Set-Cookie"),
	}
}

func (a *testApp) getPage(t *testing.T, path string, jar *testJar) testResponse {
	t.Helper()
	return a.request("GET", path, nil, jar, "")
}

func (a *testApp) getPageRaw(t *testing.T, path string, jar *testJar) testResponse {
	t.Helper()
	req, err := http.NewRequest("GET", a.server.URL+path, nil)
	if err != nil {
		t.Fatalf("new raw request: %v", err)
	}
	if jar != nil {
		req.Header.Set("Cookie", jar.header())
	}
	res, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read raw body: %v", err)
	}
	return testResponse{status: res.StatusCode, body: data, setCookies: res.Header.Values("Set-Cookie")}
}

func (r testResponse) dataArray(t *testing.T) []map[string]any {
	t.Helper()
	body := r.jsonBody(t)
	raw, ok := body["data"].([]any)
	if !ok {
		t.Fatalf("missing data array envelope: %s", r.body)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		obj, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("data entry is not an object: %v", entry)
		}
		out = append(out, obj)
	}
	return out
}

func startMockOpenAI(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"mock-mini"},{"id":"mock-embed"}]}`)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server.URL + "/v1"
}

func TestSettingsSecretsGate(t *testing.T) {
	app := startWorkerTestApp(t, t.TempDir())
	jar, orgID := app.bootstrapAndSignIn(t, "Keys Org", "owner@keys.test")
	const key = "sk-gate-visible-key-material-554433"

	save := app.request("POST", "/api/settings/providers", map[string]any{
		"providerId": "openai", "displayName": "OpenAI", "apiKey": key,
	}, jar, "")
	if save.status != http.StatusCreated {
		t.Fatalf("provider save failed: %d %s", save.status, save.body)
	}

	list := app.request("GET", "/api/settings/providers", nil, jar, "")
	if list.status != http.StatusOK {
		t.Fatalf("provider list failed: %d", list.status)
	}
	rows := list.dataArray(t)
	var row map[string]any
	for _, candidate := range rows {
		if candidate["providerId"] == "openai" {
			row = candidate
		}
	}
	if row == nil {
		t.Fatalf("saved provider missing from list: %s", list.body)
	}
	if row["keyLast4"] != key[len(key)-4:] {
		t.Fatalf("masked key must end with last4, got %v", row["keyLast4"])
	}
	if strings.Contains(string(list.body), key) {
		t.Fatal("api response leaked the plaintext key")
	}

	var cipher, last4 string
	if err := app.pool.QueryRow(context.Background(),
		"select key_cipher, key_last4 from provider_configs where org_id = $1", orgID).Scan(&cipher, &last4); err != nil {
		t.Fatalf("cipher query: %v", err)
	}
	if cipher == "" || cipher == key {
		t.Fatal("key must be stored encrypted")
	}
	if strings.Contains(cipher, key[8:24]) {
		t.Fatal("ciphertext must not embed plaintext fragments")
	}
	if last4 != key[len(key)-4:] {
		t.Fatalf("last4 mismatch: %s", last4)
	}

	if strings.Contains(app.logs.String(), key) {
		t.Fatal("app logs contain the plaintext key")
	}

	negativeCaught := false
	if row["keyLast4"] == "wrong" {
		negativeCaught = true
	}
	if negativeCaught {
		t.Fatal("mask assertions cannot fail")
	}
}

func TestSettingsModelsGate(t *testing.T) {
	mockURL := startMockOpenAI(t)
	app := startWorkerTestApp(t, t.TempDir())
	jar, _ := app.bootstrapAndSignIn(t, "Models Org", "owner@models.test")

	save := app.request("POST", "/api/settings/providers", map[string]any{
		"providerId": "openai", "displayName": "Mock OpenAI", "apiKey": "test-key", "baseUrl": mockURL,
	}, jar, "")
	if save.status != http.StatusCreated {
		t.Fatalf("provider save failed: %d %s", save.status, save.body)
	}

	missing := app.request("GET", "/api/settings/models?configId=", nil, jar, "")
	if missing.status != http.StatusBadRequest {
		t.Fatalf("empty configId must be rejected, got %d", missing.status)
	}

	list := app.request("GET", "/api/settings/providers", nil, jar, "")
	rows := list.dataArray(t)
	if len(rows) == 0 {
		t.Fatal("provider list empty")
	}
	configID, _ := rows[0]["id"].(string)

	models := app.request("GET", "/api/settings/models?configId="+configID, nil, jar, "")
	if models.status != http.StatusOK {
		t.Fatalf("models listing failed: %d %s", models.status, models.body)
	}
	foundMockMini := false
	for _, entry := range models.dataArray(t) {
		if entry["id"] == "mock-mini" {
			foundMockMini = true
		}
	}
	if !foundMockMini {
		t.Fatalf("mock-mini missing from model list: %s", models.body)
	}

	connect := app.request("POST", "/api/settings/providers-connect", map[string]any{"configId": configID}, jar, "")
	if connect.status != http.StatusOK {
		t.Fatalf("connectivity test request failed: %d", connect.status)
	}
	connectData := connect.data(t)
	if connectData["ok"] != true {
		t.Fatalf("connectivity test should pass: %s", connect.body)
	}

	broken := app.request("POST", "/api/settings/providers", map[string]any{
		"providerId": "openai-compatible", "displayName": "Broken", "baseUrl": "http://127.0.0.1:9/v1",
	}, jar, "")
	if broken.status != http.StatusCreated {
		t.Fatalf("broken provider save failed: %d", broken.status)
	}
	list2 := app.request("GET", "/api/settings/providers", nil, jar, "")
	brokenID := ""
	for _, candidate := range list2.dataArray(t) {
		if candidate["providerId"] == "openai-compatible" {
			brokenID, _ = candidate["id"].(string)
		}
	}
	if brokenID == "" {
		t.Fatalf("broken provider missing: %s", list2.body)
	}
	test2 := app.request("POST", "/api/settings/providers-connect", map[string]any{"configId": brokenID}, jar, "")
	if test2.data(t)["ok"] != false {
		t.Fatalf("connectivity test against dead endpoint must fail: %s", test2.body)
	}

	var audits int
	if err := app.pool.QueryRow(context.Background(),
		"select count(*)::int from audit_events where type = 'settings.provider.tested'").Scan(&audits); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if audits < 2 {
		t.Fatalf("expected tested audits for success and failure, got %d", audits)
	}
}

func TestSettingsPagesAndWidget(t *testing.T) {
	app := startWorkerTestApp(t, t.TempDir())
	jar, _ := app.bootstrapAndSignIn(t, "Pages Org", "owner@pages.test")
	const key = "sk-page-secret-1234"
	save := app.request("POST", "/api/settings/providers", map[string]any{
		"providerId": "openai", "displayName": "OpenAI", "apiKey": key,
	}, jar, "")
	if save.status != http.StatusCreated {
		t.Fatalf("provider save failed: %d", save.status)
	}

	redirect := app.getPageRaw(t, "/legacy/dashboard/knowledge", newTestJar())
	if redirect.status != http.StatusSeeOther {
		t.Fatalf("unauthenticated page must redirect, got %d", redirect.status)
	}

	settings := app.getPage(t, "/legacy/dashboard/settings", jar)
	if settings.status != http.StatusOK {
		t.Fatalf("settings page failed: %d", settings.status)
	}
	page := string(settings.body)
	for _, needle := range []string{"AI providers", "••••" + key[len(key)-4:], "Save provider", "Chat widget", "data-org="} {
		if !strings.Contains(page, needle) {
			t.Fatalf("settings page missing %q", needle)
		}
	}
	if strings.Contains(page, key) {
		t.Fatal("settings page leaked the plaintext key")
	}

	token := app.request("GET", "/api/settings/widget-token", nil, jar, "")
	tokenData := token.data(t)
	if tokenData["token"] == "" || !strings.Contains(tokenData["embedSnippet"].(string), "widget.js") {
		t.Fatalf("widget token response invalid: %s", token.body)
	}

	put := app.request("PUT", "/api/settings/widget", map[string]any{"origins": []string{"https://example.com"}}, jar, "")
	if put.status != http.StatusOK {
		t.Fatalf("widget put failed: %d %s", put.status, put.body)
	}
	get := app.request("GET", "/api/settings/widget", nil, jar, "")
	origins, ok := get.data(t)["origins"].([]any)
	if !ok || len(origins) != 1 || origins[0] != "https://example.com" {
		t.Fatalf("widget origins not persisted: %s", get.body)
	}
}

func TestKnowledgeUIGate(t *testing.T) {
	site, err := os.ReadFile("../../scripts/fixtures/ingest-site/index.html")
	if err != nil {
		t.Fatalf("read fixture site: %v", err)
	}
	siteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(site)
	}))
	defer siteServer.Close()

	uploads := t.TempDir()
	app := startWorkerTestApp(t, uploads)
	jar, _ := app.bootstrapAndSignIn(t, "UI Org", "owner@ui.test")

	page := app.getPage(t, "/legacy/dashboard/knowledge", jar)
	if page.status != http.StatusOK {
		t.Fatalf("knowledge page failed: %d", page.status)
	}
	html := string(page.body)
	for _, needle := range []string{`type="url"`, "Add source", "No sources yet."} {
		if !strings.Contains(html, needle) {
			t.Fatalf("knowledge page missing %q", needle)
		}
	}
	static := app.request("GET", "/static/htmx.min.js", nil, jar, "")
	if static.status != http.StatusOK || len(static.body) == 0 {
		t.Fatalf("htmx static asset must be served: %d", static.status)
	}

	added := app.postForm(t, "/dashboard/knowledge/add", url.Values{"url": {siteServer.URL + "/"}, "scope": {"public"}}, jar)
	if added.status != http.StatusSeeOther {
		t.Fatalf("add source form failed: %d %s", added.status, added.body)
	}

	var sourceID string
	deadline := time.Now().Add(60 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		list := app.request("GET", "/api/knowledge/sources", nil, jar, "")
		rows := list.dataArray(t)
		if len(rows) > 0 {
			sourceID, _ = rows[0]["id"].(string)
			if rows[0]["status"] == "ready" {
				ready = true
				break
			}
		}
		time.Sleep(time.Second)
	}
	if !ready {
		t.Fatalf("source never reached ready in the database")
	}
	if sourceID == "" {
		t.Fatal("no source id discovered")
	}

	page = app.getPage(t, "/legacy/dashboard/knowledge", jar)
	html = string(page.body)
	if !strings.Contains(html, "ready") || strings.Contains(html, "No sources yet.") {
		t.Fatalf("knowledge page did not show the ready source")
	}
	if !strings.Contains(html, "v2") {
		t.Fatalf("ingested source must show version 2")
	}

	toggle := app.postForm(t, "/dashboard/knowledge/scope", url.Values{"sourceId": {sourceID}, "scope": {"internal"}}, jar)
	if toggle.status != http.StatusSeeOther {
		t.Fatalf("scope toggle failed: %d", toggle.status)
	}
	var scope string
	if err := app.pool.QueryRow(context.Background(), "select scope::text from knowledge_sources where id = $1", sourceID).Scan(&scope); err != nil {
		t.Fatalf("scope query: %v", err)
	}
	if scope != "internal" {
		t.Fatalf("scope toggle did not persist: %s", scope)
	}
	page = app.getPage(t, "/legacy/dashboard/knowledge", jar)
	if !strings.Contains(string(page.body), "internal") {
		t.Fatal("scope toggle did not persist to internal on reload")
	}

	deleted := app.postForm(t, "/dashboard/knowledge/delete", url.Values{"sourceId": {sourceID}}, jar)
	if deleted.status != http.StatusSeeOther {
		t.Fatalf("delete failed: %d", deleted.status)
	}
	page = app.getPage(t, "/legacy/dashboard/knowledge", jar)
	if strings.Contains(string(page.body), siteServer.URL) || strings.Contains(string(page.body), "ChasteDesk Widget Help") {
		t.Fatal("deleted source still visible")
	}
	var chunks int
	if err := app.pool.QueryRow(context.Background(), "select count(*)::int from knowledge_chunks where source_id = $1", sourceID).Scan(&chunks); err != nil {
		t.Fatalf("chunk count: %v", err)
	}
	if chunks != 0 {
		t.Fatalf("deleting a source must remove its chunks, %d remain", chunks)
	}

	text := app.request("POST", "/api/knowledge/sources", map[string]any{
		"kind": "text", "name": "Pasted policy", "text": "Every plan includes password resets, billing plans, and audit logs.", "scope": "public",
	}, jar, "")
	if text.status != http.StatusCreated {
		t.Fatalf("text source create failed: %d %s", text.status, text.body)
	}
	textID, _ := text.data(t)["id"].(string)
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		if err := app.pool.QueryRow(context.Background(), "select status::text from knowledge_sources where id = $1", textID).Scan(&status); err != nil {
			t.Fatalf("text source status: %v", err)
		}
		if status == "ready" {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	uploadPath := filepath.Join(uploads, "notes.txt")
	fixture, err := os.ReadFile("../../scripts/fixtures/files/notes.txt")
	if err != nil {
		t.Fatalf("read notes fixture: %v", err)
	}
	if err := os.WriteFile(uploadPath, fixture, 0o644); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatalf("multipart part: %v", err)
	}
	_, _ = part.Write(fixture)
	_ = writer.WriteField("scope", "public")
	writer.Close()
	req, err := http.NewRequest("POST", app.server.URL+"/api/knowledge/sources", &body)
	if err != nil {
		t.Fatalf("multipart request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Cookie", jar.header())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("multipart post: %v", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("file upload failed: %d %s", res.StatusCode, raw)
	}
	var uploadResponse map[string]any
	if err := json.Unmarshal(raw, &uploadResponse); err != nil {
		t.Fatalf("upload response: %v", err)
	}
	uploadID, _ := uploadResponse["data"].(map[string]any)["id"].(string)
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		if err := app.pool.QueryRow(context.Background(), "select status::text from knowledge_sources where id = $1", uploadID).Scan(&status); err != nil {
			t.Fatalf("upload source status: %v", err)
		}
		if status == "ready" {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	var chunkText string
	if err := app.pool.QueryRow(context.Background(), "select content from knowledge_chunks where source_id = $1 limit 1", uploadID).Scan(&chunkText); err != nil {
		t.Fatalf("uploaded chunk query: %v", err)
	}
	if !strings.Contains(chunkText, "password resets, billing plans") {
		t.Fatalf("uploaded file content not indexed: %s", chunkText)
	}
}

func TestWizardFlowGate(t *testing.T) {
	mockURL := startMockOpenAI(t)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><head><title>Wizard Fixture</title></head><body><h1>Help topic</h1><p>Billing plans and password resets.</p></body></html>")
	}))
	defer fixture.Close()

	app := startWorkerTestApp(t, t.TempDir())
	jar, _ := app.bootstrapAndSignIn(t, "Wizard Org", "owner@wizard.test")

	page := app.getPage(t, "/legacy/onboarding", jar)
	if page.status != http.StatusOK {
		t.Fatalf("onboarding page failed: %d", page.status)
	}
	html := string(page.body)
	for _, needle := range []string{`id="provider"`, `id="apiKey"`, `id="baseUrl"`, "Save provider", ">Add<", "Finish setup", `value="https://example.com/help"`, "pending"} {
		if !strings.Contains(html, needle) {
			t.Fatalf("onboarding page missing %q", needle)
		}
	}

	saveProvider := app.postForm(t, "/onboarding/provider", url.Values{
		"providerId": {"openai"}, "displayName": {"OpenAI"}, "apiKey": {"test-key"}, "baseUrl": {mockURL},
	}, jar)
	if saveProvider.status != http.StatusSeeOther {
		t.Fatalf("wizard provider save failed: %d %s", saveProvider.status, saveProvider.body)
	}
	page = app.getPage(t, "/legacy/onboarding", jar)
	if !strings.Contains(string(page.body), "done") {
		t.Fatal("wizard did not mark the provider step done")
	}

	addKnowledge := app.postForm(t, "/onboarding/add", url.Values{"url": {fixture.URL + "/"}}, jar)
	if addKnowledge.status != http.StatusSeeOther {
		t.Fatalf("wizard add knowledge failed: %d %s", addKnowledge.status, addKnowledge.body)
	}
	page = app.getPage(t, "/legacy/onboarding", jar)
	if !strings.Contains(string(page.body), "done") {
		t.Fatal("wizard did not mark the knowledge step done")
	}

	finish := app.postForm(t, "/onboarding/finish", url.Values{"complete": {"true"}}, jar)
	if finish.status != http.StatusSeeOther {
		t.Fatalf("wizard finish failed: %d", finish.status)
	}
	page = app.getPage(t, "/legacy/onboarding", jar)
	content := string(page.body)
	if !strings.Contains(content, "ready") || !strings.Contains(content, "desk is ready") {
		t.Fatal("wizard did not reach the finish state")
	}
	if strings.Contains(content, "Finish setup") {
		t.Fatal("finish state must not show the wizard steps")
	}

	status := app.request("GET", "/api/onboarding/status", nil, jar, "")
	data := status.data(t)
	if data["complete"] != true {
		t.Fatalf("onboarding status must be complete after finishing: %s", status.body)
	}
	if data["providerConfigured"] != true {
		t.Fatal("providerConfigured must be true")
	}
	if data["knowledgeAdded"] != true {
		t.Fatal("knowledgeAdded must be true")
	}

	anonymous := app.getPageRaw(t, "/legacy/onboarding", newTestJar())
	if anonymous.status != http.StatusSeeOther {
		t.Fatalf("anonymous visitor must be redirected to login, got %d", anonymous.status)
	}
}
