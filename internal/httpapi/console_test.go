// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"chastedesk/internal/auth"
	"chastedesk/internal/id"
)

func consoleMap(t *testing.T, value any, name string) map[string]any {
	t.Helper()
	row, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %v", name, value)
	}
	return row
}

func TestConsole(t *testing.T) {
	app := startWorkerTestApp(t, t.TempDir())

	before := app.request("GET", "/api/app/session", nil, nil, "")
	if before.status != http.StatusOK {
		t.Fatalf("public session: %d %s", before.status, before.body)
	}
	if before.data(t)["setupRequired"] != true {
		t.Fatalf("fresh install must advertise setup: %s", before.body)
	}

	metricsResponse := app.request("GET", "/metrics", nil, nil, "")
	if metricsResponse.status != http.StatusOK || !strings.Contains(string(metricsResponse.body), "chastedesk_http_requests_total") {
		t.Fatalf("metrics endpoint: %d %s", metricsResponse.status, metricsResponse.body)
	}

	jar := newTestJar()
	setup := app.request("POST", "/api/app/setup", map[string]any{
		"orgName": "Console Org", "userName": "Amina Okoye", "email": "owner@console.test", "password": "password123",
	}, jar, "")
	if setup.status != http.StatusCreated {
		t.Fatalf("console setup: %d %s", setup.status, setup.body)
	}
	orgID, _ := setup.data(t)["org"].(map[string]any)["id"].(string)
	if orgID == "" {
		t.Fatalf("setup did not return an org: %s", setup.body)
	}

	session := app.request("GET", "/api/app/session", nil, jar, "")
	if session.status != http.StatusOK {
		t.Fatalf("session: %d %s", session.status, session.body)
	}
	sessionData := session.data(t)
	if sessionData["authenticated"] != true {
		t.Fatalf("session must be authenticated: %s", session.body)
	}
	permissions := consoleMap(t, sessionData["permissions"], "permissions")
	if permissions["canSettings"] != true || permissions["canAudit"] != true {
		t.Fatalf("owner permissions incomplete: %s", session.body)
	}

	provider := app.request("POST", "/api/app/settings/providers", map[string]any{
		"providerId": "openai", "displayName": "OpenAI", "apiKey": "sk-console-test",
	}, jar, "")
	if provider.status != http.StatusCreated {
		t.Fatalf("provider save: %d %s", provider.status, provider.body)
	}

	ctx := context.Background()
	customerID := id.UUID()
	if _, err := app.pool.Exec(ctx,
		`insert into customers (id, org_id, email, name, attributes) values ($1, $2, 'ada@console.test', 'Ada Lovelace', '{"plan":"Scale","region":"EU"}'::jsonb)`,
		customerID, orgID); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	conversationID := id.UUID()
	if _, err := app.pool.Exec(ctx,
		`insert into conversations (id, org_id, customer_id, channel, status, handoff, updated_at) values ($1, $2, $3, 'widget', 'awaiting_human', '{"problem":"Billing email","recommendedNextStep":"Walk through Settings","investigated":["billing"],"evidence":[],"actionsTaken":[]}'::jsonb, now())`,
		conversationID, orgID, customerID); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	if _, err := app.pool.Exec(ctx,
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', 'How do I change billing email?')`,
		orgID, conversationID); err != nil {
		t.Fatalf("seed message: %v", err)
	}
	var agentID string
	if err := app.pool.QueryRow(ctx, `select id from agents where org_id = $1 limit 1`, orgID).Scan(&agentID); err != nil {
		t.Fatalf("agent: %v", err)
	}
	runID := id.UUID()
	if _, err := app.pool.Exec(ctx,
		`insert into ai_runs (id, org_id, conversation_id, agent_id, status) values ($1, $2, $3, $4, 'awaiting_approval')`,
		runID, orgID, conversationID, agentID); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	actionID := id.UUID()
	if _, err := app.pool.Exec(ctx,
		`insert into actions (id, org_id, run_id, conversation_id, agent_id, tool_id, args, status) values ($1, $2, $3, $4, $5, 'conversations.add_note', '{"note":"Check the plan before replying."}'::jsonb, 'pending_approval')`,
		actionID, orgID, runID, conversationID, agentID); err != nil {
		t.Fatalf("seed action: %v", err)
	}
	if _, err := app.pool.Exec(ctx,
		`insert into knowledge_sources (org_id, type, name, scope, status, source_config) values ($1, 'url', 'Billing settings', 'public', 'ready', '{"url":"https://docs.example.test/billing"}'::jsonb)`,
		orgID); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if _, err := app.pool.Exec(ctx,
		`insert into analytics_daily (org_id, date, metrics) values ($1, to_char(now(), 'YYYY-MM-DD'), '{"total_conversations":7,"ai_resolution_rate":0.6,"escalation_rate":0.2,"p50_first_response_seconds":30,"p50_resolution_seconds":900}'::jsonb)`,
		orgID); err != nil {
		t.Fatalf("seed analytics: %v", err)
	}

	overview := app.request("GET", "/api/app/overview", nil, jar, "")
	if overview.status != http.StatusOK {
		t.Fatalf("overview: %d %s", overview.status, overview.body)
	}
	overviewData := overview.data(t)
	if overviewData["greeting"] == "" {
		t.Fatalf("overview greeting missing: %s", overview.body)
	}
	if len(overviewData["attention"].([]any)) == 0 {
		t.Fatalf("overview must surface attention: %s", overview.body)
	}
	if len(overviewData["recent"].([]any)) == 0 {
		t.Fatalf("overview must include recent conversations: %s", overview.body)
	}

	inbox := app.request("GET", "/api/app/inbox?status=awaiting_human", nil, jar, "")
	if inbox.status != http.StatusOK {
		t.Fatalf("inbox: %d %s", inbox.status, inbox.body)
	}
	inboxData := inbox.data(t)
	items, _ := inboxData["conversations"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one filtered conversation: %s", inbox.body)
	}
	first := consoleMap(t, items[0], "conversation")
	if first["customer"].(map[string]any)["name"] != "Ada Lovelace" {
		t.Fatalf("conversation customer missing: %s", inbox.body)
	}
	if first["pendingApprovals"].(float64) != 1 {
		t.Fatalf("pending approval count missing: %s", inbox.body)
	}

	detail := app.request("GET", "/api/inbox/conversations/"+conversationID, nil, jar, "")
	if detail.status != http.StatusOK {
		t.Fatalf("detail: %d %s", detail.status, detail.body)
	}
	detailConversation := consoleMap(t, detail.data(t)["conversation"], "conversation detail")
	if detailConversation["status"] != "awaiting_human" {
		t.Fatalf("detail status mismatch: %s", detail.body)
	}
	if detailConversation["ticketNumber"].(float64) < 1001 {
		t.Fatalf("ticket number was not assigned: %s", detail.body)
	}
	if detailConversation["firstResponseDueAt"] == nil || detailConversation["resolutionDueAt"] == nil {
		t.Fatalf("SLA due dates were not assigned: %s", detail.body)
	}
	numberBefore := detailConversation["ticketNumber"].(float64)

	snoozeUntil := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	fields := app.request("PATCH", "/api/app/inbox/"+conversationID+"/ticket", map[string]any{
		"priority": "urgent", "tags": []string{"Billing", "VIP", "vip"}, "snoozedUntil": snoozeUntil,
	}, jar, "")
	if fields.status != http.StatusOK {
		t.Fatalf("ticket update: %d %s", fields.status, fields.body)
	}
	ticketData := fields.data(t)
	if ticketData["priority"] != "urgent" {
		t.Fatalf("ticket priority mismatch: %s", fields.body)
	}
	if len(ticketData["tags"].([]any)) != 2 {
		t.Fatalf("ticket tags were not deduplicated: %s", fields.body)
	}
	if ticketData["snoozedUntil"] == nil {
		t.Fatalf("ticket snooze was not saved: %s", fields.body)
	}
	var (
		priority     string
		ticketNumber int64
		tags         []string
	)
	if err := app.pool.QueryRow(ctx, `select priority, ticket_number, tags from conversations where id = $1`, conversationID).Scan(&priority, &ticketNumber, &tags); err != nil {
		t.Fatalf("read ticket fields: %v", err)
	}
	if priority != "urgent" || ticketNumber != int64(numberBefore) || len(tags) != 2 {
		t.Fatalf("ticket fields did not persist: priority=%s number=%d tags=%v", priority, ticketNumber, tags)
	}
	filteredByTicket := app.request("GET", "/api/app/inbox?priority=urgent&tag=billing", nil, jar, "")
	if filteredByTicket.status != http.StatusOK {
		t.Fatalf("ticket filters: %d %s", filteredByTicket.status, filteredByTicket.body)
	}
	if len(filteredByTicket.data(t)["conversations"].([]any)) != 1 {
		t.Fatalf("ticket filters lost the conversation: %s", filteredByTicket.body)
	}
	cleared := app.request("PATCH", "/api/app/inbox/"+conversationID+"/ticket", map[string]any{"clearSnooze": true}, jar, "")
	if cleared.status != http.StatusOK {
		t.Fatalf("clear snooze: %d %s", cleared.status, cleared.body)
	}
	if cleared.data(t)["snoozedUntil"] != nil {
		t.Fatalf("snooze did not clear: %s", cleared.body)
	}

	close := app.request("POST", "/api/inbox/conversations/"+conversationID+"/resolve", map[string]any{"action": "close"}, jar, "")
	if close.status != http.StatusOK {
		t.Fatalf("close ticket: %d %s", close.status, close.body)
	}
	var closedStatus string
	if err := app.pool.QueryRow(ctx, `select status::text from conversations where id = $1`, conversationID).Scan(&closedStatus); err != nil {
		t.Fatalf("read closed status: %v", err)
	}
	if closedStatus != "closed" {
		t.Fatalf("ticket did not close: %s", closedStatus)
	}
	reopened := app.request("POST", "/api/inbox/conversations/"+conversationID+"/resolve", map[string]any{"action": "reopen"}, jar, "")
	if reopened.status != http.StatusOK {
		t.Fatalf("reopen ticket: %d %s", reopened.status, reopened.body)
	}

	bulkSnooze := app.request("POST", "/api/app/inbox/bulk", map[string]any{
		"ids": []string{conversationID}, "action": "snooze", "snoozedUntil": time.Now().Add(4 * time.Hour).UTC().Format(time.RFC3339),
	}, jar, "")
	if bulkSnooze.status != http.StatusOK {
		t.Fatalf("bulk snooze: %d %s", bulkSnooze.status, bulkSnooze.body)
	}
	snoozedList := app.request("GET", "/api/app/inbox?status=snoozed", nil, jar, "")
	if len(snoozedList.data(t)["conversations"].([]any)) != 1 {
		t.Fatalf("snoozed filter lost the ticket: %s", snoozedList.body)
	}
	openList := app.request("GET", "/api/app/inbox?status=open", nil, jar, "")
	if len(openList.data(t)["conversations"].([]any)) != 0 {
		t.Fatalf("snoozed ticket still appeared in open: %s", openList.body)
	}
	bulkPriority := app.request("POST", "/api/app/inbox/bulk", map[string]any{
		"ids": []string{conversationID}, "action": "priority", "priority": "high",
	}, jar, "")
	if bulkPriority.status != http.StatusOK {
		t.Fatalf("bulk priority: %d %s", bulkPriority.status, bulkPriority.body)
	}
	bulkClose := app.request("POST", "/api/app/inbox/bulk", map[string]any{
		"ids": []string{conversationID}, "action": "close",
	}, jar, "")
	if bulkClose.status != http.StatusOK {
		t.Fatalf("bulk close: %d %s", bulkClose.status, bulkClose.body)
	}
	if err := app.pool.QueryRow(ctx, `select status::text, priority from conversations where id = $1`, conversationID).Scan(&closedStatus, &priority); err != nil {
		t.Fatalf("read bulk ticket state: %v", err)
	}
	if closedStatus != "closed" || priority != "high" {
		t.Fatalf("bulk ticket state mismatch: status=%s priority=%s", closedStatus, priority)
	}
	closedReply := app.request("POST", "/api/inbox/conversations/"+conversationID+"/reply", map[string]any{
		"content": "Reopening from the closed state.", "internal": false,
	}, jar, "")
	if closedReply.status != http.StatusCreated {
		t.Fatalf("reply to closed ticket: %d %s", closedReply.status, closedReply.body)
	}
	var closedAt *time.Time
	if err := app.pool.QueryRow(ctx, `select status::text, closed_at from conversations where id = $1`, conversationID).Scan(&closedStatus, &closedAt); err != nil {
		t.Fatalf("read reply-reopened state: %v", err)
	}
	if closedStatus != "human_active" || closedAt != nil {
		t.Fatalf("reply did not clear the closed state: status=%s closedAt=%v", closedStatus, closedAt)
	}
	bulkReopen := app.request("POST", "/api/app/inbox/bulk", map[string]any{
		"ids": []string{conversationID}, "action": "reopen",
	}, jar, "")
	if bulkReopen.status != http.StatusOK {
		t.Fatalf("bulk reopen: %d %s", bulkReopen.status, bulkReopen.body)
	}

	if _, err := app.pool.Exec(ctx, `update conversations set created_at = now() - interval '2 hours' where id = $1`, conversationID); err != nil {
		t.Fatalf("age conversation: %v", err)
	}
	slaSave := app.request("POST", "/api/app/settings/sla", map[string]any{"policies": []map[string]any{
		{"priority": "urgent", "firstResponseMinutes": 10, "resolutionMinutes": 60, "enabled": true},
		{"priority": "high", "firstResponseMinutes": 20, "resolutionMinutes": 120, "enabled": true},
		{"priority": "normal", "firstResponseMinutes": 30, "resolutionMinutes": 60, "enabled": true},
		{"priority": "low", "firstResponseMinutes": 60, "resolutionMinutes": 240, "enabled": true},
	}}, jar, "")
	if slaSave.status != http.StatusOK || len(slaSave.data(t)["policies"].([]any)) != 4 {
		t.Fatalf("SLA save: %d %s", slaSave.status, slaSave.body)
	}
	breachedList := app.request("GET", "/api/app/inbox?sla=breached", nil, jar, "")
	if breachedList.status != http.StatusOK || len(breachedList.data(t)["conversations"].([]any)) != 1 {
		t.Fatalf("SLA breach filter: %d %s", breachedList.status, breachedList.body)
	}
	var breachedFlag bool
	var firstDue, resolutionDue *time.Time
	if err := app.pool.QueryRow(ctx, `select first_response_due_at, resolution_due_at from conversations where id = $1`, conversationID).Scan(&firstDue, &resolutionDue); err != nil {
		t.Fatalf("read SLA due dates: %v", err)
	}
	if firstDue == nil || resolutionDue == nil {
		t.Fatalf("SLA save did not recalculate due dates")
	}
	detailAfterSLA := app.request("GET", "/api/inbox/conversations/"+conversationID, nil, jar, "")
	breachedFlag, _ = detailAfterSLA.data(t)["conversation"].(map[string]any)["slaBreached"].(bool)
	if !breachedFlag {
		t.Fatalf("detail did not report the breached ticket: %s", detailAfterSLA.body)
	}

	assign := app.request("POST", "/api/inbox/conversations/"+conversationID+"/assign", map[string]any{"assignedUserId": nil}, jar, "")
	if assign.status != http.StatusOK {
		t.Fatalf("unassign: %d %s", assign.status, assign.body)
	}
	var assigned *string
	if err := app.pool.QueryRow(ctx, `select assigned_user_id from conversations where id = $1`, conversationID).Scan(&assigned); err != nil {
		t.Fatalf("read assignee: %v", err)
	}
	if assigned != nil {
		t.Fatalf("null assignedUserId must clear assignment, got %v", *assigned)
	}

	bulk := app.request("POST", "/api/app/inbox/bulk", map[string]any{"ids": []string{conversationID}, "action": "resolve"}, jar, "")
	if bulk.status != http.StatusOK {
		t.Fatalf("bulk resolve: %d %s", bulk.status, bulk.body)
	}
	if len(bulk.data(t)["updated"].([]any)) != 1 {
		t.Fatalf("bulk resolve did not report an update: %s", bulk.body)
	}
	reopen := app.request("POST", "/api/inbox/conversations/"+conversationID+"/resolve", map[string]any{"action": "reopen"}, jar, "")
	if reopen.status != http.StatusOK {
		t.Fatalf("reopen: %d %s", reopen.status, reopen.body)
	}

	customers := app.request("GET", "/api/app/customers?q=ada", nil, jar, "")
	if customers.status != http.StatusOK {
		t.Fatalf("customers: %d %s", customers.status, customers.body)
	}
	if len(customers.data(t)["customers"].([]any)) != 1 {
		t.Fatalf("customer search mismatch: %s", customers.body)
	}
	customer := app.request("GET", "/api/app/customers/"+customerID, nil, jar, "")
	if customer.status != http.StatusOK {
		t.Fatalf("customer detail: %d %s", customer.status, customer.body)
	}

	approvals := app.request("GET", "/api/app/approvals", nil, jar, "")
	if approvals.status != http.StatusOK {
		t.Fatalf("approvals: %d %s", approvals.status, approvals.body)
	}
	if len(approvals.data(t)["pending"].([]any)) != 1 {
		t.Fatalf("pending approvals mismatch: %s", approvals.body)
	}
	decision := app.request("POST", "/api/app/approvals/bulk", map[string]any{"ids": []string{actionID}, "decision": "approve"}, jar, "")
	if decision.status != http.StatusOK {
		t.Fatalf("approval decision: %d %s", decision.status, decision.body)
	}
	if len(decision.data(t)["done"].([]any)) != 1 {
		t.Fatalf("approval decision was not recorded: %s", decision.body)
	}

	knowledge := app.request("GET", "/api/app/knowledge", nil, jar, "")
	if knowledge.status != http.StatusOK {
		t.Fatalf("knowledge: %d %s", knowledge.status, knowledge.body)
	}
	if knowledge.data(t)["counts"].(map[string]any)["ready"].(float64) != 1 {
		t.Fatalf("knowledge counts mismatch: %s", knowledge.body)
	}

	createdSource := app.request("POST", "/api/app/knowledge", map[string]any{
		"kind": "text", "name": "Console policy", "text": "The console policy is to cite the source.", "scope": "internal",
	}, jar, "")
	if createdSource.status != http.StatusCreated {
		t.Fatalf("knowledge create: %d %s", createdSource.status, createdSource.body)
	}
	sourceID, _ := createdSource.data(t)["id"].(string)
	updated := app.request("PATCH", "/api/app/knowledge/"+sourceID, map[string]any{"scope": "public"}, jar, "")
	if updated.status != http.StatusOK {
		t.Fatalf("knowledge update: %d %s", updated.status, updated.body)
	}
	deleted := app.request("DELETE", "/api/app/knowledge/"+sourceID, nil, jar, "")
	if deleted.status != http.StatusOK {
		t.Fatalf("knowledge delete: %d %s", deleted.status, deleted.body)
	}

	analytics := app.request("GET", "/api/app/analytics?days=7", nil, jar, "")
	if analytics.status != http.StatusOK {
		t.Fatalf("analytics: %d %s", analytics.status, analytics.body)
	}
	if analytics.data(t)["metrics"] == nil {
		t.Fatalf("analytics metrics missing: %s", analytics.body)
	}
	if len(analytics.data(t)["trend"].([]any)) == 0 {
		t.Fatalf("analytics trend missing: %s", analytics.body)
	}

	audit := app.request("GET", "/api/app/audit", nil, jar, "")
	if audit.status != http.StatusOK {
		t.Fatalf("audit: %d %s", audit.status, audit.body)
	}
	if len(audit.data(t)["events"].([]any)) == 0 {
		t.Fatalf("audit events missing: %s", audit.body)
	}

	search := app.request("GET", "/api/app/search?q=ada", nil, jar, "")
	if search.status != http.StatusOK {
		t.Fatalf("search: %d %s", search.status, search.body)
	}
	if len(search.data(t)["customers"].([]any)) == 0 {
		t.Fatalf("search customers missing: %s", search.body)
	}

	settings := app.request("GET", "/api/app/settings", nil, jar, "")
	if settings.status != http.StatusOK {
		t.Fatalf("settings: %d %s", settings.status, settings.body)
	}
	settingsData := settings.data(t)
	if len(settingsData["providers"].([]any)) != 1 {
		t.Fatalf("settings providers mismatch: %s", settings.body)
	}
	if settingsData["canAdmin"] != true {
		t.Fatalf("owner must be able to administer members: %s", settings.body)
	}
	if len(settingsData["slaPolicies"].([]any)) != 4 {
		t.Fatalf("four SLA policies must be seeded: %s", settings.body)
	}
	ownerID, _ := consoleMap(t, settingsData["user"], "settings user")["id"].(string)

	memberID := id.UUID()
	if _, err := app.pool.Exec(ctx, `insert into "user" (id, name, email) values ($1, 'Sam Member', 'sam@console.test')`, memberID); err != nil {
		t.Fatalf("insert member user: %v", err)
	}
	if _, err := app.pool.Exec(ctx, `insert into member (id, organization_id, user_id, role) values ($1, $2, $3, 'agent')`, id.UUID(), orgID, memberID); err != nil {
		t.Fatalf("insert member: %v", err)
	}
	if _, err := app.pool.Exec(ctx, `insert into session (id, expires_at, token, user_id) values ($1, now() + interval '1 day', 'member-session', $2)`, id.UUID(), memberID); err != nil {
		t.Fatalf("insert member session: %v", err)
	}
	settingsWithMember := app.request("GET", "/api/app/settings", nil, jar, "")
	memberSeen := false
	for _, raw := range settingsWithMember.data(t)["members"].([]any) {
		member := consoleMap(t, raw, "member")
		if member["id"] == memberID && member["role"] == "agent" {
			memberSeen = true
		}
	}
	if !memberSeen {
		t.Fatalf("member id missing from settings payload: %s", settingsWithMember.body)
	}
	changedRole := app.request("PATCH", "/api/app/settings/members/"+memberID, map[string]any{"role": "admin"}, jar, "")
	if changedRole.status != http.StatusOK {
		t.Fatalf("change member role: %d %s", changedRole.status, changedRole.body)
	}
	if changedRole.data(t)["sessionsRevoked"].(float64) < 1 {
		t.Fatalf("role change did not revoke member sessions: %s", changedRole.body)
	}
	if _, err := app.pool.Exec(ctx, `insert into session (id, expires_at, token, user_id) values ($1, now() + interval '1 day', 'member-session-2', $2)`, id.UUID(), memberID); err != nil {
		t.Fatalf("insert member session again: %v", err)
	}
	removedMember := app.request("DELETE", "/api/app/settings/members/"+memberID, nil, jar, "")
	if removedMember.status != http.StatusOK {
		t.Fatalf("remove member: %d %s", removedMember.status, removedMember.body)
	}
	var memberRows, sessionRows int
	if err := app.pool.QueryRow(ctx, `select count(*)::int from member where organization_id = $1 and user_id = $2`, orgID, memberID).Scan(&memberRows); err != nil {
		t.Fatalf("count members: %v", err)
	}
	if err := app.pool.QueryRow(ctx, `select count(*)::int from session where user_id = $1`, memberID).Scan(&sessionRows); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if memberRows != 0 || sessionRows != 0 {
		t.Fatalf("member removal left access behind: members=%d sessions=%d", memberRows, sessionRows)
	}
	if selfRemove := app.request("DELETE", "/api/app/settings/members/"+ownerID, nil, jar, ""); selfRemove.status != http.StatusBadRequest {
		t.Fatalf("self removal must be rejected: %d %s", selfRemove.status, selfRemove.body)
	}

	agentSave := app.request("POST", "/api/app/settings/agent", map[string]any{
		"purpose": "Answer console questions.", "publicScope": true, "internalScope": false,
		"tools": map[string]string{
			"knowledge.search": "autonomous", "customers.lookup": "approval_required",
			"conversations.add_note": "approval_required", "escalate": "autonomous",
		},
		"keywords": "refund", "threshold": 0.4,
		"providerId": "openai", "modelId": "gpt-4.1-mini",
		"maxSteps": 5, "maxToolCalls": 0, "maxInputTokens": 5000, "maxOutputTokens": 1000,
	}, jar, "")
	if agentSave.status != http.StatusOK {
		t.Fatalf("agent save: %d %s", agentSave.status, agentSave.body)
	}
	var rawPolicy []byte
	if err := app.pool.QueryRow(ctx, `select policy from agents where org_id = $1 limit 1`, orgID).Scan(&rawPolicy); err != nil {
		t.Fatalf("read policy: %v", err)
	}
	var policy map[string]any
	if err := json.Unmarshal(rawPolicy, &policy); err != nil {
		t.Fatalf("decode policy: %v", err)
	}
	if policy["purpose"] != "Answer console questions." {
		t.Fatalf("policy purpose not saved: %v", policy["purpose"])
	}

	widgetSave := app.request("POST", "/api/app/settings/widget", map[string]any{
		"origins": []string{"https://shop.example.test"}, "enabled": true,
		"displayName": "Console", "greeting": "Hello", "primaryColor": "#d96c3f", "position": "right",
	}, jar, "")
	if widgetSave.status != http.StatusOK {
		t.Fatalf("widget save: %d %s", widgetSave.status, widgetSave.body)
	}

	key := app.request("POST", "/api/app/settings/api-keys", map[string]any{"name": "Console key"}, jar, "")
	if key.status != http.StatusCreated {
		t.Fatalf("api key create: %d %s", key.status, key.body)
	}
	secret, _ := key.data(t)["secret"].(string)
	if len(secret) < 20 {
		t.Fatalf("api key secret not returned: %s", key.body)
	}
	keyID, _ := key.data(t)["id"].(string)
	revoked := app.request("DELETE", "/api/app/settings/api-keys/"+keyID, nil, jar, "")
	if revoked.status != http.StatusOK {
		t.Fatalf("api key revoke: %d %s", revoked.status, revoked.body)
	}

	quickReply := app.request("POST", "/api/app/canned-replies", map[string]any{
		"title": "Billing email", "body": "You can update the billing email in Settings, then Billing.",
	}, jar, "")
	if quickReply.status != http.StatusCreated {
		t.Fatalf("canned reply create: %d %s", quickReply.status, quickReply.body)
	}
	replyID, _ := quickReply.data(t)["id"].(string)
	duplicateReply := app.request("POST", "/api/app/canned-replies", map[string]any{
		"title": "billing EMAIL", "body": "Duplicate title.",
	}, jar, "")
	if duplicateReply.status != http.StatusConflict {
		t.Fatalf("duplicate canned reply: %d %s", duplicateReply.status, duplicateReply.body)
	}
	listReplies := app.request("GET", "/api/app/canned-replies", nil, jar, "")
	if len(listReplies.data(t)["replies"].([]any)) != 1 {
		t.Fatalf("canned reply list mismatch: %s", listReplies.body)
	}
	updatedReply := app.request("PATCH", "/api/app/canned-replies/"+replyID, map[string]any{
		"title": "Billing email", "body": "Billing email changes apply to the next invoice.",
	}, jar, "")
	if updatedReply.status != http.StatusOK {
		t.Fatalf("canned reply update: %d %s", updatedReply.status, updatedReply.body)
	}
	deletedReply := app.request("DELETE", "/api/app/canned-replies/"+replyID, nil, jar, "")
	if deletedReply.status != http.StatusOK {
		t.Fatalf("canned reply delete: %d %s", deletedReply.status, deletedReply.body)
	}

	account := app.request("POST", "/api/app/account", map[string]any{"name": "Amina Renamed"}, jar, "")
	if account.status != http.StatusOK {
		t.Fatalf("account save: %d %s", account.status, account.body)
	}
	accountGet := app.request("GET", "/api/app/account", nil, jar, "")
	if accountGet.data(t)["name"] != "Amina Renamed" {
		t.Fatalf("account name not saved: %s", accountGet.body)
	}

	if accountGet.data(t)["twoFactorEnabled"] != false {
		t.Fatalf("two-factor must start disabled: %s", accountGet.body)
	}
	setup2FA := app.request("POST", "/api/app/account/two-factor/setup", nil, jar, "")
	if setup2FA.status != http.StatusOK {
		t.Fatalf("2FA setup: %d %s", setup2FA.status, setup2FA.body)
	}
	secret2FA, _ := setup2FA.data(t)["secret"].(string)
	uri2FA, _ := setup2FA.data(t)["otpauthUri"].(string)
	if secret2FA == "" || !strings.HasPrefix(uri2FA, "otpauth://totp/") {
		t.Fatalf("2FA setup payload incomplete: %s", setup2FA.body)
	}
	code2FA, err := auth.TOTPCode(secret2FA, time.Now())
	if err != nil {
		t.Fatalf("generate TOTP: %v", err)
	}
	enable2FA := app.request("POST", "/api/app/account/two-factor/enable", map[string]any{"code": code2FA}, jar, "")
	if enable2FA.status != http.StatusOK {
		t.Fatalf("2FA enable: %d %s", enable2FA.status, enable2FA.body)
	}
	recoveryCodes, _ := enable2FA.data(t)["recoveryCodes"].([]any)
	if len(recoveryCodes) != 10 {
		t.Fatalf("recovery codes missing: %s", enable2FA.body)
	}
	accountWith2FA := app.request("GET", "/api/app/account", nil, jar, "")
	if accountWith2FA.data(t)["twoFactorEnabled"] != true {
		t.Fatalf("2FA not enabled in account: %s", accountWith2FA.body)
	}

	login2FA := app.request("POST", "/api/auth/sign-in/email", map[string]any{"email": "owner@console.test", "password": "password123"}, newTestJar(), "")
	if login2FA.status != http.StatusOK || login2FA.data(t)["twoFactorRequired"] != true {
		t.Fatalf("2FA sign-in challenge missing: %d %s", login2FA.status, login2FA.body)
	}
	challenge2FA, _ := login2FA.data(t)["challengeToken"].(string)
	if challenge2FA == "" {
		t.Fatalf("2FA challenge token missing: %s", login2FA.body)
	}
	codeNow, _ := auth.TOTPCode(secret2FA, time.Now())
	loginJar := newTestJar()
	verify2FA := app.request("POST", "/api/auth/two-factor/verify", map[string]any{"challengeToken": challenge2FA, "code": codeNow}, loginJar, "")
	if verify2FA.status != http.StatusOK {
		t.Fatalf("2FA verify: %d %s", verify2FA.status, verify2FA.body)
	}
	verifiedSession := app.request("GET", "/api/app/session", nil, loginJar, "")
	if verifiedSession.data(t)["authenticated"] != true {
		t.Fatalf("2FA session not authenticated: %s", verifiedSession.body)
	}

	recoveryChallengeJar := newTestJar()
	recoveryLogin := app.request("POST", "/api/auth/sign-in/email", map[string]any{"email": "owner@console.test", "password": "password123"}, recoveryChallengeJar, "")
	recoveryChallenge, _ := recoveryLogin.data(t)["challengeToken"].(string)
	recoveryVerify := app.request("POST", "/api/auth/two-factor/verify", map[string]any{"challengeToken": recoveryChallenge, "code": recoveryCodes[0]}, recoveryChallengeJar, "")
	if recoveryVerify.status != http.StatusOK {
		t.Fatalf("2FA recovery code: %d %s", recoveryVerify.status, recoveryVerify.body)
	}

	codeForDisable, _ := auth.TOTPCode(secret2FA, time.Now())
	disable2FA := app.request("POST", "/api/app/account/two-factor/disable", map[string]any{"currentPassword": "password123", "code": codeForDisable}, jar, "")
	if disable2FA.status != http.StatusOK {
		t.Fatalf("2FA disable: %d %s", disable2FA.status, disable2FA.body)
	}
	accountWithout2FA := app.request("GET", "/api/app/account", nil, jar, "")
	if accountWithout2FA.data(t)["twoFactorEnabled"] != false {
		t.Fatalf("2FA remained enabled: %s", accountWithout2FA.body)
	}

	invite := app.request("POST", "/api/app/settings/invitations", map[string]any{"email": "robin@console.test", "role": "agent"}, jar, "")
	if invite.status != http.StatusCreated {
		t.Fatalf("invite create: %d %s", invite.status, invite.body)
	}
	inviteID, _ := invite.data(t)["id"].(string)
	preview := app.request("GET", "/api/app/invitations/"+inviteID, nil, nil, "")
	if preview.status != http.StatusOK {
		t.Fatalf("invite preview: %d %s", preview.status, preview.body)
	}
	if preview.data(t)["email"] != "robin@console.test" {
		t.Fatalf("invite preview mismatch: %s", preview.body)
	}
	revokedInvite := app.request("DELETE", "/api/app/settings/invitations/"+inviteID, nil, jar, "")
	if revokedInvite.status != http.StatusOK {
		t.Fatalf("invite revoke: %d %s", revokedInvite.status, revokedInvite.body)
	}

	var pending int
	if err := app.pool.QueryRow(ctx, `select count(*)::int from actions where org_id = $1 and status = 'pending_approval'`, orgID).Scan(&pending); err != nil {
		t.Fatalf("pending count: %v", err)
	}
	if pending != 0 {
		t.Fatalf("approved action still pending: %d", pending)
	}
}
