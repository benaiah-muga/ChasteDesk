// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/analytics"
	"chastedesk/internal/id"
	"chastedesk/internal/testutil"
)

func startP11App(t *testing.T) *workerTestApp {
	t.Helper()
	app := startP9App(t)
	app.jobs.Work(app.ctx, "analytics-rollup", 1, analytics.Handler(app.pool))
	return app
}

func TestAuditCoverageGate(t *testing.T) {
	app := startP11App(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Audit Scenario Org", "owner@auditsc.test")

	mock := testutil.StartMockOpenAI(t)
	save := app.request("POST", "/api/settings/providers", map[string]any{
		"providerId": "openai", "displayName": "Mock", "apiKey": "sk-audit-key-0001", "baseUrl": mock.URL,
	}, jar, "")
	if save.status != http.StatusCreated {
		t.Fatalf("provider save: %d %s", save.status, save.body)
	}

	knowledge := app.request("POST", "/api/knowledge/sources", map[string]any{
		"kind": "text", "name": "Audit docs",
		"text":  "Password resets happen on the security settings page. Refunds require human approval.",
		"scope": "public",
	}, jar, "")
	if knowledge.status != http.StatusCreated {
		t.Fatalf("knowledge create: %d %s", knowledge.status, knowledge.body)
	}

	setWidgetOrigins(t, app, jar)
	widgetToken := widgetTokenFor(t, app, jar)
	create := rawRequest(t, "POST", app.server.URL+"/api/widget/conversations?token="+widgetToken,
		map[string]any{"visitorName": "Cust", "visitorEmail": "c@auditsc.test", "firstMessage": "I demand a refund now."},
		map[string]string{"Origin": "https://shop.example.com"})
	if create.status != http.StatusCreated {
		t.Fatalf("widget conversation create: %d %s", create.status, create.body)
	}

	if !waitFor(t, 30*time.Second, func() bool {
		var count int
		_ = app.pool.QueryRow(context.Background(),
			`select count(*)::int from audit_events where org_id = $1 and type = 'run.escalated'`, orgID).Scan(&count)
		return count > 0
	}) {
		t.Fatalf("run.escalated audit row never appeared")
	}

	expected := []string{
		"auth.login", "settings.provider.saved", "knowledge.source.created",
		"conversation.created", "run.started", "run.escalated", "org.created",
	}
	missing := []string{}
	for _, auditType := range expected {
		var count int
		if err := app.pool.QueryRow(context.Background(),
			`select count(*)::int from audit_events where org_id = $1 and type = $2`, orgID, auditType).Scan(&count); err != nil {
			t.Fatalf("audit count %s: %v", auditType, err)
		}
		if count == 0 {
			missing = append(missing, auditType)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing audit types: %s", strings.Join(missing, ", "))
	}

	all := app.request("GET", "/api/audit", nil, jar, "")
	if all.status != http.StatusOK {
		t.Fatalf("audit list: %d %s", all.status, all.body)
	}
	allRows := all.dataArray(t)
	filtered := app.request("GET", "/api/audit?type=run.escalated", nil, jar, "")
	if filtered.status != http.StatusOK {
		t.Fatalf("filtered audit list: %d %s", filtered.status, filtered.body)
	}
	filteredRows := filtered.dataArray(t)
	if len(filteredRows) == 0 {
		t.Fatalf("filter returned no run.escalated rows")
	}
	for _, row := range filteredRows {
		if row["type"] != "run.escalated" {
			t.Fatalf("filter returned non-matching row: %v", row["type"])
		}
	}
	if len(filteredRows) >= len(allRows) {
		t.Fatalf("type filter must narrow results (%d of %d)", len(filteredRows), len(allRows))
	}
}

func testMedian(values []int64) *int64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		value := sorted[mid]
		return &value
	}
	value := int64(math.Round(float64(sorted[mid]+sorted[mid-1]) / 2))
	return &value
}

func querySeconds(t *testing.T, pool *pgxpool.Pool, query, orgID, date string) []int64 {
	t.Helper()
	rows, err := pool.Query(context.Background(), query, orgID, date)
	if err != nil {
		t.Fatalf("query seconds: %v", err)
	}
	defer rows.Close()
	values := []int64{}
	for rows.Next() {
		var seconds int64
		if err := rows.Scan(&seconds); err != nil {
			t.Fatalf("scan seconds: %v", err)
		}
		values = append(values, seconds)
	}
	return values
}

func TestAnalyticsMetricsGate(t *testing.T) {
	app := startP9App(t)
	_, orgID := app.bootstrapAndSignIn(t, "Analytics Org", "owner@analytics.test")
	ctx := context.Background()

	convs := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		customerID := id.UUID()
		conversationID := id.UUID()
		if _, err := app.pool.Exec(ctx,
			`insert into customers (id, org_id, email, name, attributes) values ($1, $2, $3, $4, '{}'::jsonb)`,
			customerID, orgID, "c"+string(rune('0'+i))+"@a.test", "C"+string(rune('0'+i))); err != nil {
			t.Fatalf("insert customer: %v", err)
		}
		if _, err := app.pool.Exec(ctx,
			`insert into conversations (id, org_id, customer_id, channel, status, created_at) values ($1, $2, $3, 'widget', 'ai_active', now())`,
			conversationID, orgID, customerID); err != nil {
			t.Fatalf("insert conversation: %v", err)
		}
		convs = append(convs, conversationID)
	}
	message := func(conversationID, role, content string) {
		if _, err := app.pool.Exec(ctx,
			`insert into messages (org_id, conversation_id, role, content) values ($1, $2, $3, $4)`,
			orgID, conversationID, role, content); err != nil {
			t.Fatalf("insert message: %v", err)
		}
	}

	// c0: answered + resolved
	message(convs[0], "customer", "How do I reset my password?")
	message(convs[0], "agent_ai", "Use the security page to reset it.")
	if _, err := app.pool.Exec(ctx,
		`update conversations set status = 'resolved', resolved_at = now(), first_response_at = now() where id = $1`, convs[0]); err != nil {
		t.Fatalf("resolve c0: %v", err)
	}

	// c1: answered, escalated with handoff, human resolved
	message(convs[1], "customer", "I want a refund.")
	message(convs[1], "agent_ai", "Escalating to a teammate.")
	if _, err := app.pool.Exec(ctx,
		`update conversations set status = 'resolved', resolved_at = now(), handoff = $2::jsonb, first_response_at = now() where id = $1`,
		convs[1], `{"problem":"refund"}`); err != nil {
		t.Fatalf("resolve c1: %v", err)
	}

	// c2: answered + resolved
	message(convs[2], "customer", "How do I configure webhooks?")
	message(convs[2], "agent_ai", "Use settings then webhooks.")
	if _, err := app.pool.Exec(ctx,
		`update conversations set status = 'resolved', resolved_at = now(), first_response_at = now() where id = $1`, convs[2]); err != nil {
		t.Fatalf("resolve c2: %v", err)
	}

	// c3: escalated awaiting human
	message(convs[3], "customer", "Cancel my account.")
	if _, err := app.pool.Exec(ctx,
		`update conversations set status = 'awaiting_human', handoff = $2::jsonb where id = $1`,
		convs[3], `{"problem":"cancellation"}`); err != nil {
		t.Fatalf("escalate c3: %v", err)
	}

	// c4: ai_active, no response
	message(convs[4], "customer", "Something else entirely.")

	// c5: human handled from the start, resolved
	message(convs[5], "customer", "Complex billing question.")
	message(convs[5], "agent_human", "Let me help with billing.")
	if _, err := app.pool.Exec(ctx,
		`update conversations set status = 'resolved', resolved_at = now(), first_response_at = now() where id = $1`, convs[5]); err != nil {
		t.Fatalf("resolve c5: %v", err)
	}

	var today string
	if err := app.pool.QueryRow(ctx, `select to_char(now(), 'YYYY-MM-DD')`).Scan(&today); err != nil {
		t.Fatalf("db date: %v", err)
	}

	var expTotal, expEscalations, expAiResolved, expResolved int
	if err := app.pool.QueryRow(ctx,
		`select
		   (select count(*)::int from conversations where org_id = $1) as total,
		   (select count(*)::int from conversations where org_id = $1 and handoff is not null) as escalations,
		   (select count(*)::int from conversations c where org_id = $1 and c.status = 'resolved'
		      and not exists (select 1 from messages m where m.conversation_id = c.id and m.role = 'agent_human')) as ai_resolved_count,
		   (select count(*)::int from conversations c where org_id = $1 and c.status = 'resolved') as resolved_count`,
		orgID).Scan(&expTotal, &expEscalations, &expAiResolved, &expResolved); err != nil {
		t.Fatalf("expected counts: %v", err)
	}
	expFirstResponse := testMedian(querySeconds(t, app.pool,
		`select distinct on (c.id) extract(epoch from (m.created_at - c.created_at))::int as seconds
		 from conversations c
		 join messages m on m.conversation_id = c.id and m.role in ('agent_ai', 'agent_human')
		 where c.org_id = $1 and to_char(c.created_at, 'YYYY-MM-DD') = $2
		 order by c.id, m.created_at asc`, orgID, today))
	expResolution := testMedian(querySeconds(t, app.pool,
		`select extract(epoch from (c.resolved_at - c.created_at))::int as seconds
		 from conversations c
		 where c.org_id = $1 and c.resolved_at is not null and to_char(c.created_at, 'YYYY-MM-DD') = $2`, orgID, today))

	metrics, err := analytics.Rollup(ctx, app.pool, orgID, today)
	if err != nil {
		t.Fatalf("rollup: %v", err)
	}
	if metrics.TotalConversations != expTotal {
		t.Fatalf("total mismatch %d vs %d", metrics.TotalConversations, expTotal)
	}
	expAiRate := 0.0
	if expResolved > 0 {
		expAiRate = float64(expAiResolved) / float64(expResolved)
	}
	if math.Abs(metrics.AIResolutionRate-expAiRate) > 0.001 {
		t.Fatalf("ai rate mismatch %v vs %v", metrics.AIResolutionRate, expAiRate)
	}
	if math.Abs(metrics.EscalationRate-float64(expEscalations)/float64(expTotal)) > 0.001 {
		t.Fatalf("escalation rate mismatch %v", metrics.EscalationRate)
	}
	if !equalPointer(metrics.P50FirstResponseSeconds, expFirstResponse) {
		t.Fatalf("p50 first response mismatch %v vs %v", metrics.P50FirstResponseSeconds, expFirstResponse)
	}
	if !equalPointer(metrics.P50ResolutionSeconds, expResolution) {
		t.Fatalf("p50 resolution mismatch %v vs %v", metrics.P50ResolutionSeconds, expResolution)
	}

	var storedRaw []byte
	if err := app.pool.QueryRow(ctx,
		`select metrics from analytics_daily where org_id = $1 and date = $2`, orgID, today).Scan(&storedRaw); err != nil {
		t.Fatalf("stored metrics missing: %v", err)
	}
	var stored analytics.Metrics
	if err := json.Unmarshal(storedRaw, &stored); err != nil {
		t.Fatalf("stored metrics parse: %v", err)
	}
	if stored.TotalConversations != metrics.TotalConversations {
		t.Fatalf("stored total mismatch %d vs %d", stored.TotalConversations, metrics.TotalConversations)
	}

	caught := false
	func() {
		defer func() {
			if recover() != nil {
				caught = true
			}
		}()
		if metrics.TotalConversations != -1 {
			panic("probe")
		}
	}()
	if !caught {
		t.Fatalf("metrics assertions cannot fail")
	}
}

func equalPointer(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func TestAnalyticsUIGate(t *testing.T) {
	app := startP9App(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Metrics Org", "owner@metrics.test")
	ctx := context.Background()

	page := pageBody(t, app.request("GET", "/dashboard/analytics", nil, jar, ""))
	if !strings.Contains(page, "No analytics yet") {
		t.Fatalf("analytics page must render an empty state first")
	}

	var today string
	if err := app.pool.QueryRow(ctx, `select to_char(now(), 'YYYY-MM-DD')`).Scan(&today); err != nil {
		t.Fatalf("db date: %v", err)
	}
	metricsJSON := `{"total_conversations":7,"ai_resolution_rate":0.6,"escalation_rate":0.2,"p50_first_response_seconds":30,"p50_resolution_seconds":900}`
	if _, err := app.pool.Exec(ctx,
		`insert into analytics_daily (org_id, date, metrics) values ($1, $2, $3::jsonb)`, orgID, today, metricsJSON); err != nil {
		t.Fatalf("seed analytics row: %v", err)
	}

	page = pageBody(t, app.request("GET", "/dashboard/analytics", nil, jar, ""))
	for _, expected := range []string{
		"Total conversations", "AI resolution rate", "Escalation rate",
		"Median first response", "Median resolution", ">7<", "60%", "20%", "30s", "900s",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("analytics page missing %q", expected)
		}
	}

	summary := app.request("GET", "/api/analytics/summary", nil, jar, "")
	if summary.status != http.StatusOK {
		t.Fatalf("summary endpoint: %d %s", summary.status, summary.body)
	}
	summaryData := summary.data(t)
	metrics, _ := summaryData["metrics"].(map[string]any)
	if metrics == nil {
		t.Fatalf("summary metrics must not be null")
	}
	if total, _ := metrics["total_conversations"].(float64); total != 7 {
		t.Fatalf("summary total mismatch: %v", metrics["total_conversations"])
	}
	trend, _ := summaryData["trend"].([]any)
	if len(trend) != 1 {
		t.Fatalf("trend must contain the seeded day, got %d entries", len(trend))
	}
}

func TestAnalyticsRollupJobGate(t *testing.T) {
	app := startP11App(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Rollup Org", "owner@rollup.test")
	ctx := context.Background()

	customerID := id.UUID()
	if _, err := app.pool.Exec(ctx,
		`insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'r@rollup.test', 'R', '{}'::jsonb)`,
		customerID, orgID); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	escalated := id.UUID()
	if _, err := app.pool.Exec(ctx,
		`insert into conversations (id, org_id, customer_id, channel, status, handoff, updated_at)
		 values ($1, $2, $3, 'widget', 'awaiting_human', '{"problem":"refund"}'::jsonb, now())`,
		escalated, orgID, customerID); err != nil {
		t.Fatalf("insert escalated conversation: %v", err)
	}
	resolved := id.UUID()
	if _, err := app.pool.Exec(ctx,
		`insert into conversations (id, org_id, customer_id, channel, status, resolved_at, created_at)
		 values ($1, $2, $3, 'widget', 'resolved', now(), now())`,
		resolved, orgID, customerID); err != nil {
		t.Fatalf("insert resolved conversation: %v", err)
	}
	if _, err := app.pool.Exec(ctx,
		`insert into messages (org_id, conversation_id, role, content) values
		 ($1, $2, 'customer', 'Reset help'), ($1, $2, 'agent_ai', 'Use the security page.')`,
		orgID, resolved); err != nil {
		t.Fatalf("insert resolved messages: %v", err)
	}

	queued := app.request("POST", "/api/analytics/rollup", map[string]any{}, jar, "")
	if queued.status != http.StatusOK {
		t.Fatalf("rollup enqueue: %d %s", queued.status, queued.body)
	}
	if value, _ := queued.data(t)["queued"].(bool); !value {
		t.Fatalf("rollup must report queued")
	}

	var today string
	if err := app.pool.QueryRow(ctx, `select to_char(now(), 'YYYY-MM-DD')`).Scan(&today); err != nil {
		t.Fatalf("db date: %v", err)
	}
	if !waitFor(t, 20*time.Second, func() bool {
		var raw []byte
		if err := app.pool.QueryRow(ctx,
			`select metrics from analytics_daily where org_id = $1 and date = $2`, orgID, today).Scan(&raw); err != nil {
			return false
		}
		var stored analytics.Metrics
		if err := json.Unmarshal(raw, &stored); err != nil {
			return false
		}
		return stored.TotalConversations == 2
	}) {
		t.Fatalf("rollup job never stored today's metrics")
	}

	page := pageBody(t, app.request("GET", "/dashboard/analytics", nil, jar, ""))
	if !strings.Contains(page, "Total conversations") {
		t.Fatalf("analytics page must show cards after rollup")
	}
	summary := app.request("GET", "/api/analytics/summary", nil, jar, "")
	metrics, _ := summary.data(t)["metrics"].(map[string]any)
	if total, _ := metrics["total_conversations"].(float64); total != 2 {
		t.Fatalf("summary must reflect rollup, got %v", metrics["total_conversations"])
	}
}
