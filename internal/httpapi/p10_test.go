// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"chastedesk/internal/id"
	"chastedesk/internal/widget"
)

func postFormRequest(t *testing.T, target string, values map[string]string, jar *testJar) testResponse {
	t.Helper()
	form := url.Values{}
	for key, value := range values {
		form.Set(key, value)
	}
	req, err := http.NewRequest("POST", target, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new form request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if jar != nil {
		req.Header.Set("Cookie", jar.header())
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("form request: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read form response: %v", err)
	}
	return testResponse{status: res.StatusCode, body: body}
}

func collectStreamFrames(t *testing.T, streamURL string, wait time.Duration) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Origin", "https://shop.example.com")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	reader := bufio.NewReader(res.Body)
	frames := []string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return frames
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "data: ") {
			frames = append(frames, strings.TrimPrefix(line, "data: "))
		}
	}
}

func seedInboxCustomer(t *testing.T, app *workerTestApp, orgID, email, name, attributes string) string {
	t.Helper()
	customerID := id.UUID()
	if _, err := app.pool.Exec(context.Background(),
		`insert into customers (id, org_id, email, name, attributes) values ($1, $2, $3, $4, $5::jsonb)`,
		customerID, orgID, email, name, attributes); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	return customerID
}

func seedInboxConversation(t *testing.T, app *workerTestApp, orgID, customerID, status string, handoff string) string {
	t.Helper()
	conversationID := id.UUID()
	payload := "null"
	if handoff != "" {
		payload = handoff
	}
	if _, err := app.pool.Exec(context.Background(),
		`insert into conversations (id, org_id, customer_id, channel, status, handoff) values ($1, $2, $3, 'widget', $4, $5::jsonb)`,
		conversationID, orgID, customerID, status, payload); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	return conversationID
}

func seedInboxMessage(t *testing.T, app *workerTestApp, orgID, conversationID, role, content string) {
	t.Helper()
	if _, err := app.pool.Exec(context.Background(),
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, $3, $4)`,
		orgID, conversationID, role, content); err != nil {
		t.Fatalf("insert message: %v", err)
	}
}

func pageBody(t *testing.T, res testResponse) string {
	t.Helper()
	if res.status != http.StatusOK {
		t.Fatalf("page status %d: %s", res.status, res.body)
	}
	return string(res.body)
}

func TestInboxContextGate(t *testing.T) {
	app := startP9App(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Inbox Org", "owner@inbox.test")
	customerID := seedInboxCustomer(t, app, orgID, "nina@inbox.test", "Nina Nina", `{"tier":"gold"}`)
	handoff := `{"problem":"Customer cannot reset password.","customerContext":{"name":"Nina Nina","email":"nina@inbox.test","attributes":{"tier":"gold"},"recentConversations":[]},"investigated":["searched knowledge base"],"evidence":[{"source":"Help","version":1,"quote":"use the security page"}],"actionsTaken":[],"recommendedNextStep":"Walk the customer through the reset flow."}`
	seeded := map[string]string{}
	for _, status := range []string{"ai_active", "awaiting_human", "resolved"} {
		payload := ""
		if status == "awaiting_human" {
			payload = handoff
		}
		conversationID := seedInboxConversation(t, app, orgID, customerID, status, payload)
		seeded[status] = conversationID
		seedInboxMessage(t, app, orgID, conversationID, "customer", "Message for "+status+" conversation about password trouble.")
	}

	list := app.request("GET", "/api/inbox/conversations", nil, jar, "")
	if list.status != http.StatusOK {
		t.Fatalf("inbox list api: %d %s", list.status, list.body)
	}
	if items := list.dataArray(t); len(items) != 3 {
		t.Fatalf("API list must return 3 conversations, got %d", len(items))
	}

	listPage := pageBody(t, app.request("GET", "/dashboard/inbox", nil, jar, ""))
	if !strings.Contains(listPage, "awaiting human") {
		t.Fatalf("list must show awaiting human status")
	}
	if !strings.Contains(listPage, "Nina Nina") {
		t.Fatalf("list must show customer name")
	}

	detail := pageBody(t, app.request("GET", "/dashboard/inbox/"+seeded["awaiting_human"], nil, jar, ""))
	for _, expected := range []string{
		"Why the AI escalated", "Walk the customer through the reset flow.",
		"Nina Nina", "nina@inbox.test", "gold", "password trouble", "1 other open conversation",
	} {
		if !strings.Contains(detail, expected) {
			t.Fatalf("detail page missing %q", expected)
		}
	}
}

func TestInboxReplyGate(t *testing.T) {
	app := startP9App(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Reply Org", "owner@reply.test")
	setWidgetOrigins(t, app, jar)
	widgetToken := widgetTokenFor(t, app, jar)
	customerID := seedInboxCustomer(t, app, orgID, "n@reply.test", "Nora", `{}`)
	handoff := `{"problem":"Refund requested","customerContext":{"name":"Nora","email":"n@reply.test","attributes":{},"recentConversations":[]},"investigated":["kb search"],"evidence":[{"source":"Refund policy","version":2,"quote":"refunds need approval"}],"actionsTaken":[],"recommendedNextStep":"Approve refund manually."}`
	conversationID := seedInboxConversation(t, app, orgID, customerID, "awaiting_human", handoff)
	seedInboxMessage(t, app, orgID, conversationID, "customer", "I want a refund please.")

	convToken, err := widget.SignConversationToken(testAppSecret, orgID, conversationID, time.Now())
	if err != nil {
		t.Fatalf("sign conversation token: %v", err)
	}
	streamURL := app.server.URL + "/api/widget/conversations/" + conversationID + "/stream?token=" + widgetToken + "&convToken=" + convToken
	frames := make(chan []string, 1)
	go func() { frames <- collectStreamFrames(t, streamURL, 8*time.Second) }()
	time.Sleep(500 * time.Millisecond)

	reply := app.request("POST", "/api/inbox/conversations/"+conversationID+"/reply", map[string]any{
		"content": "Here is the reply from your teammate.", "internal": false,
	}, jar, "")
	if reply.status != http.StatusCreated {
		t.Fatalf("reply failed: %d %s", reply.status, reply.body)
	}
	role, _ := reply.data(t)["role"].(string)
	if role != "agent_human" {
		t.Fatalf("reply role must be agent_human, got %s", role)
	}

	sawHuman := false
	for _, frame := range <-frames {
		if strings.Contains(frame, "reply from your teammate") {
			sawHuman = true
		}
	}
	if !sawHuman {
		t.Fatalf("human reply not visible on widget stream")
	}

	note := app.request("POST", "/api/inbox/conversations/"+conversationID+"/reply", map[string]any{
		"content": "Internal: customer is eligible per policy.", "internal": true,
	}, jar, "")
	if note.status != http.StatusCreated {
		t.Fatalf("internal note failed: %d %s", note.status, note.body)
	}
	for _, frame := range collectStreamFrames(t, streamURL, 2500*time.Millisecond) {
		if strings.Contains(frame, "Internal:") {
			t.Fatalf("internal note leaked to widget stream: %s", frame)
		}
	}

	ctx := context.Background()
	var roles []string
	rows, err := app.pool.Query(ctx, `select role::text from messages where conversation_id = $1 order by created_at`, conversationID)
	if err != nil {
		t.Fatalf("load roles: %v", err)
	}
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			t.Fatalf("scan role: %v", err)
		}
		roles = append(roles, role)
	}
	rows.Close()
	hasHuman, hasNote := false, false
	for _, role := range roles {
		if role == "agent_human" {
			hasHuman = true
		}
		if role == "internal_note" {
			hasNote = true
		}
	}
	if !hasHuman || !hasNote {
		t.Fatalf("expected agent_human and internal_note messages, got %v", roles)
	}

	resolve := postFormRequest(t, app.server.URL+"/dashboard/inbox/"+conversationID+"/resolve",
		map[string]string{"action": "resolve"}, jar)
	if resolve.status != http.StatusOK {
		t.Fatalf("resolve form failed: %d %s", resolve.status, resolve.body)
	}
	var status string
	var resolvedAt *time.Time
	if err := app.pool.QueryRow(ctx, `select status::text, resolved_at from conversations where id = $1`, conversationID).
		Scan(&status, &resolvedAt); err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if status != "resolved" || resolvedAt == nil {
		t.Fatalf("conversation not resolved (status=%s resolved_at=%v)", status, resolvedAt)
	}

	detail := pageBody(t, app.request("GET", "/dashboard/inbox/"+conversationID, nil, jar, ""))
	if !strings.Contains(detail, "Approve refund manually.") {
		t.Fatalf("handoff recommended step not rendered")
	}
}

func TestInboxApprovalsGate(t *testing.T) {
	app := startP9App(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Approval UI Org", "owner@apui.test")
	customerID := seedInboxCustomer(t, app, orgID, "c@apui.test", "Cust", `{}`)
	conversationID := seedInboxConversation(t, app, orgID, customerID, "awaiting_human", "")
	agentID := id.UUID()
	if _, err := app.pool.Exec(context.Background(),
		`insert into agents (id, org_id, name, policy, is_active) values ($1, $2, 'A', '{}'::jsonb, true)`,
		agentID, orgID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	runID := id.UUID()
	if _, err := app.pool.Exec(context.Background(),
		`insert into ai_runs (id, org_id, conversation_id, agent_id, status) values ($1, $2, $3, $4, 'awaiting_approval')`,
		runID, orgID, conversationID, agentID); err != nil {
		t.Fatalf("insert run: %v", err)
	}
	toApprove := seedPendingActionForConversation(t, app, orgID, conversationID, runID, agentID, "approval-ui-note")
	toReject := seedPendingActionForConversation(t, app, orgID, conversationID, runID, agentID, "approval-ui-reject")

	detail := pageBody(t, app.request("GET", "/dashboard/inbox/"+conversationID, nil, jar, ""))
	if !strings.Contains(detail, "Approve") || !strings.Contains(detail, "conversations.add_note") {
		t.Fatalf("approval card must render before decision")
	}

	postFormRequest(t, app.server.URL+"/dashboard/inbox/action", map[string]string{
		"actionId": toApprove, "decision": "approve", "conversationId": conversationID,
	}, jar)
	if !waitFor(t, 15*time.Second, func() bool {
		var status string
		_ = app.pool.QueryRow(context.Background(), `select status::text from actions where id = $1`, toApprove).Scan(&status)
		return status == "executed" || status == "approved"
	}) {
		t.Fatalf("approved action never progressed")
	}

	postFormRequest(t, app.server.URL+"/dashboard/inbox/action", map[string]string{
		"actionId": toReject, "decision": "reject", "conversationId": conversationID,
	}, jar)
	var rejectedStatus string
	if err := app.pool.QueryRow(context.Background(),
		`select status::text from actions where id = $1`, toReject).Scan(&rejectedStatus); err != nil {
		t.Fatalf("load rejected action: %v", err)
	}
	if rejectedStatus != "rejected_before_exec" {
		t.Fatalf("reject path status wrong: %s", rejectedStatus)
	}
}

func seedPendingActionForConversation(t *testing.T, app *workerTestApp, orgID, conversationID, runID, agentID, note string) string {
	t.Helper()
	actionID := id.UUID()
	args, _ := json.Marshal(map[string]any{"note": note})
	if _, err := app.pool.Exec(context.Background(),
		`insert into actions (id, org_id, run_id, conversation_id, agent_id, tool_id, args, status)
		 values ($1, $2, $3, $4, $5, 'conversations.add_note', $6::jsonb, 'pending_approval')`,
		actionID, orgID, runID, conversationID, agentID, string(args)); err != nil {
		t.Fatalf("insert pending action: %v", err)
	}
	return actionID
}

func TestCopilotGate(t *testing.T) {
	app := startP9App(t)
	jar, orgID := app.bootstrapAndSignIn(t, "Copilot Org", "owner@cop.test")
	customerID := seedInboxCustomer(t, app, orgID, "c@cop.test", "Cust", `{}`)
	conversationID := seedInboxConversation(t, app, orgID, customerID, "ai_active", "")
	seedInboxMessage(t, app, orgID, conversationID, "customer", "How do I reset my password?")
	sourceID := id.UUID()
	chunkID := id.UUID()
	ctx := context.Background()
	if _, err := app.pool.Exec(ctx,
		`insert into knowledge_sources (id, org_id, type, name, scope, status, version) values ($1, $2, 'text', 'Reset docs', 'public', 'ready', 1)`,
		sourceID, orgID); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := app.pool.Exec(ctx,
		`insert into knowledge_chunks (id, org_id, source_id, version, chunk_index, content, locator, token_count)
		 values ($1, $2, $3, 1, 0, 'To reset your password open the security settings page and choose a new credential.', '{}'::jsonb, 20)`,
		chunkID, orgID, sourceID); err != nil {
		t.Fatalf("insert chunk: %v", err)
	}

	detail := pageBody(t, app.request("GET", "/dashboard/inbox/"+conversationID, nil, jar, ""))
	if !strings.Contains(detail, "Draft with copilot") {
		t.Fatalf("detail page missing copilot button")
	}

	draft := app.request("POST", "/api/copilot/draft", map[string]any{"conversationId": conversationID}, jar, "")
	if draft.status != http.StatusOK {
		t.Fatalf("copilot draft failed: %d %s", draft.status, draft.body)
	}
	draftData := draft.data(t)
	draftText, _ := draftData["draft"].(string)
	if !strings.Contains(draftText, "security settings") {
		t.Fatalf("draft must contain grounded knowledge: %s", draftText)
	}
	if grounded, _ := draftData["grounded"].(bool); !grounded {
		t.Fatalf("draft must be grounded")
	}
	if citations, _ := draftData["citations"].([]any); len(citations) == 0 {
		t.Fatalf("draft must cite evidence")
	}

	var before int
	if err := app.pool.QueryRow(ctx,
		`select count(*)::int from messages where conversation_id = $1 and role = 'agent_human'`, conversationID).Scan(&before); err != nil {
		t.Fatalf("count agent_human: %v", err)
	}
	if before != 0 {
		t.Fatalf("copilot draft must not auto-send")
	}

	fragment := postFormRequest(t, app.server.URL+"/dashboard/inbox/"+conversationID+"/copilot", map[string]string{}, jar)
	if fragment.status != http.StatusOK || !strings.Contains(string(fragment.body), "Copilot draft (not sent)") {
		t.Fatalf("copilot fragment missing: %d", fragment.status)
	}

	postFormRequest(t, app.server.URL+"/dashboard/inbox/"+conversationID+"/copilot/send",
		map[string]string{"content": draftText}, jar)
	var after int
	if err := app.pool.QueryRow(ctx,
		`select count(*)::int from messages where conversation_id = $1 and role = 'agent_human'`, conversationID).Scan(&after); err != nil {
		t.Fatalf("count agent_human after send: %v", err)
	}
	if after != 1 {
		t.Fatalf("sent draft must become an agent_human message, got %d", after)
	}
}
