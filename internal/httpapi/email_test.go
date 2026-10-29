// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func (a *testApp) inboundEmail(secret string, payload map[string]any) testResponse {
	a.t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		a.t.Fatalf("marshal inbound email: %v", err)
	}
	req, err := http.NewRequest("POST", a.server.URL+"/api/email/inbound/postmark", bytes.NewReader(encoded))
	if err != nil {
		a.t.Fatalf("new inbound email request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatalf("inbound email request: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		a.t.Fatalf("read inbound email response: %v", err)
	}
	return testResponse{status: res.StatusCode, body: body}
}

func TestEmailInboundAndOutbox(t *testing.T) {
	app := startWorkerTestApp(t, t.TempDir())
	jar := newTestJar()
	setup := app.request("POST", "/api/app/setup", map[string]any{
		"orgName": "Email Org", "userName": "Email Owner", "email": "owner@email.test", "password": "password123",
	}, jar, "")
	if setup.status != http.StatusCreated {
		t.Fatalf("setup: %d %s", setup.status, setup.body)
	}
	var orgID string
	if err := app.pool.QueryRow(t.Context(), `select m.organization_id from member m join "user" u on u.id = m.user_id where u.email = 'owner@email.test'`).Scan(&orgID); err != nil {
		t.Fatalf("find org: %v", err)
	}
	const secret = "test-inbound-email-secret"
	payload := map[string]any{
		"From":      "Nina <nina@email.test>",
		"FromFull":  map[string]any{"Email": "nina@email.test", "Name": "Nina"},
		"To":        "support@chastedesk.test",
		"ToFull":    []map[string]any{{"Email": "support@chastedesk.test", "Name": "Support"}},
		"Subject":   "Refund request",
		"TextBody":  "Please refund the duplicate charge.",
		"MessageID": "<msg-email-1@email.test>",
		"Headers":   []map[string]any{{"Name": "Message-ID", "Value": "<msg-email-1@email.test>"}},
	}
	first := app.inboundEmail(secret, payload)
	if first.status != http.StatusOK {
		t.Fatalf("inbound email: %d %s", first.status, first.body)
	}
	firstBody := first.data(t)
	if firstBody["newConversation"] != true || firstBody["conversationId"] == "" {
		t.Fatalf("inbound conversation payload: %s", first.body)
	}
	conversationID, _ := firstBody["conversationId"].(string)
	var channel, subject string
	var ticket int64
	if err := app.pool.QueryRow(t.Context(), `select channel::text, coalesce(email_subject, ''), ticket_number from conversations where id = $1`, conversationID).Scan(&channel, &subject, &ticket); err != nil {
		t.Fatalf("read inbound conversation: %v", err)
	}
	if channel != "email" || subject != "Refund request" || ticket < 1000 {
		t.Fatalf("inbound conversation mismatch: channel=%s subject=%s ticket=%d", channel, subject, ticket)
	}
	var messageCount int
	if err := app.pool.QueryRow(t.Context(), `select count(*)::int from messages where conversation_id = $1 and role = 'customer'`, conversationID).Scan(&messageCount); err != nil || messageCount != 1 {
		t.Fatalf("inbound message count: count=%d err=%v", messageCount, err)
	}

	duplicate := app.inboundEmail(secret, payload)
	if duplicate.status != http.StatusOK || duplicate.data(t)["duplicate"] != true {
		t.Fatalf("duplicate inbound email: %d %s", duplicate.status, duplicate.body)
	}
	if err := app.pool.QueryRow(t.Context(), `select count(*)::int from messages where conversation_id = $1`, conversationID).Scan(&messageCount); err != nil || messageCount != 1 {
		t.Fatalf("duplicate message count: count=%d err=%v", messageCount, err)
	}

	if _, err := app.pool.Exec(t.Context(),
		`insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'agent_human', 'We have issued the refund.')`,
		orgID, conversationID); err != nil {
		t.Fatalf("insert operator reply: %v", err)
	}
	var outboxSubject, outboxTo, outboxStatus string
	if err := app.pool.QueryRow(t.Context(),
		`select subject, to_email, status from email_outbox where conversation_id = $1 order by created_at desc limit 1`, conversationID).
		Scan(&outboxSubject, &outboxTo, &outboxStatus); err != nil {
		t.Fatalf("read outbound email: %v", err)
	}
	if outboxStatus != "queued" || outboxTo != "nina@email.test" || !contains(outboxSubject, fmt.Sprintf("[#%d]", ticket)) {
		t.Fatalf("outbound email mismatch: status=%s to=%s subject=%s", outboxStatus, outboxTo, outboxSubject)
	}

	reply := app.inboundEmail(secret, map[string]any{
		"FromFull":  map[string]any{"Email": "nina@email.test", "Name": "Nina"},
		"ToFull":    []map[string]any{{"Email": "support@chastedesk.test"}},
		"Subject":   fmt.Sprintf("Re: Refund request [#%d]", ticket),
		"TextBody":  "Thank you, I can see it now.",
		"MessageID": "<msg-email-2@email.test>",
	})
	if reply.status != http.StatusOK || reply.data(t)["newConversation"] != false {
		t.Fatalf("threaded inbound email: %d %s", reply.status, reply.body)
	}
	if err := app.pool.QueryRow(t.Context(), `select count(*)::int from messages where conversation_id = $1 and role = 'customer'`, conversationID).Scan(&messageCount); err != nil || messageCount != 2 {
		t.Fatalf("threaded message count: count=%d err=%v", messageCount, err)
	}

	if unauthorized := app.inboundEmail("wrong-secret", payload); unauthorized.status != http.StatusUnauthorized {
		t.Fatalf("inbound secret must be required: %d %s", unauthorized.status, unauthorized.body)
	}
	wrongRecipient := app.inboundEmail(secret, map[string]any{
		"FromFull": map[string]any{"Email": "nina@email.test"},
		"ToFull":   []map[string]any{{"Email": "other@chastedesk.test"}},
		"Subject":  "Wrong mailbox", "TextBody": "test", "MessageID": "<msg-wrong@email.test>",
	})
	if wrongRecipient.status != http.StatusUnprocessableEntity {
		t.Fatalf("wrong mailbox must be rejected: %d %s", wrongRecipient.status, wrongRecipient.body)
	}
	selfSender := app.inboundEmail(secret, map[string]any{
		"FromFull": map[string]any{"Email": "support@chastedesk.test"},
		"ToFull":   []map[string]any{{"Email": "support@chastedesk.test"}},
		"Subject":  "Loop", "TextBody": "loop", "MessageID": "<msg-loop@chastedesk.test>",
	})
	if selfSender.status != http.StatusUnprocessableEntity {
		t.Fatalf("self-sender must be rejected: %d %s", selfSender.status, selfSender.body)
	}
	automatic := app.inboundEmail(secret, map[string]any{
		"FromFull": map[string]any{"Email": "nina@email.test"},
		"ToFull":   []map[string]any{{"Email": "support@chastedesk.test"}},
		"Subject":  "Automatic", "TextBody": "automatic", "MessageID": "<msg-auto@email.test>",
		"Headers": []map[string]any{{"Name": "Auto-Submitted", "Value": "auto-replied"}},
	})
	if automatic.status != http.StatusUnprocessableEntity {
		t.Fatalf("auto-submitted mail must be rejected: %d %s", automatic.status, automatic.body)
	}
	authFailed := app.inboundEmail(secret, map[string]any{
		"FromFull": map[string]any{"Email": "nina@email.test"},
		"ToFull":   []map[string]any{{"Email": "support@chastedesk.test"}},
		"Subject":  "Failed auth", "TextBody": "failed", "MessageID": "<msg-auth@email.test>",
		"Headers": []map[string]any{{"Name": "Authentication-Results", "Value": "mx.example; spf=fail dkim=fail dmarc=fail"}},
	})
	if authFailed.status != http.StatusUnprocessableEntity {
		t.Fatalf("failed authentication must be rejected: %d %s", authFailed.status, authFailed.body)
	}
}

func contains(value, needle string) bool {
	return bytes.Contains([]byte(value), []byte(needle))
}
