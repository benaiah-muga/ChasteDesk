// SPDX-License-Identifier: AGPL-3.0-only
package email_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"chastedesk/internal/db"
	"chastedesk/internal/email"
	"chastedesk/internal/id"
	"chastedesk/internal/testutil"
)

func startFakeSMTP(t *testing.T) (string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	messages := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)
		write := func(value string) {
			_, _ = writer.WriteString(value)
			_ = writer.Flush()
		}
		write("220 fake-smtp ESMTP\r\n")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			command := strings.TrimSpace(line)
			upper := strings.ToUpper(command)
			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				write("250-fake-smtp\r\n250 SIZE 10485760\r\n")
			case strings.HasPrefix(upper, "MAIL FROM"), strings.HasPrefix(upper, "RCPT TO"):
				write("250 OK\r\n")
			case upper == "DATA":
				write("354 End data with <CR><LF>.<CR><LF>\r\n")
				var body strings.Builder
				for {
					dataLine, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if dataLine == ".\r\n" || dataLine == ".\n" {
						break
					}
					body.WriteString(dataLine)
				}
				messages <- body.String()
				write("250 queued\r\n")
			case upper == "QUIT":
				write("221 bye\r\n")
				return
			default:
				write("250 OK\r\n")
			}
		}
	}()
	return listener.Addr().String(), messages
}

func TestSenderAndDispatcher(t *testing.T) {
	dsn := testutil.StartEphemeralPG(t)
	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	const orgID = "org-email"
	if _, err := pool.Exec(ctx, `insert into organization (id, name, slug) values ($1, 'Email Org', 'email-org')`, orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	customerID := id.UUID()
	if _, err := pool.Exec(ctx, `insert into customers (id, org_id, email, name) values ($1, $2, 'nina@email.test', 'Nina')`, customerID, orgID); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	conversationID := id.UUID()
	if _, err := pool.Exec(ctx, `insert into conversations (id, org_id, customer_id, channel, status, email_subject) values ($1, $2, $3, 'email', 'human_active', 'Refund request')`, conversationID, orgID, customerID); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'agent_human', 'We issued the refund.')`, orgID, conversationID); err != nil {
		t.Fatalf("insert reply: %v", err)
	}
	var subject, to string
	if err := pool.QueryRow(ctx, `select subject, to_email from email_outbox where conversation_id = $1`, conversationID).Scan(&subject, &to); err != nil {
		t.Fatalf("outbox row: %v", err)
	}
	if to != "nina@email.test" || !strings.Contains(subject, "Refund request") {
		t.Fatalf("outbox mismatch: to=%s subject=%s", to, subject)
	}

	address, messages := startFakeSMTP(t)
	host, portString, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split address: %v", err)
	}
	port := 0
	if _, err := fmt.Sscanf(portString, "%d", &port); err != nil {
		t.Fatalf("parse port: %v", err)
	}
	sender := email.NewSender(email.SenderConfig{Host: host, Port: port, From: "support@chastedesk.test", TLS: "none"})
	sent, failed, err := email.DispatchOnce(ctx, db.NewRepo(pool), sender, nil)
	if err != nil || sent != 1 || failed != 0 {
		t.Fatalf("dispatch: sent=%d failed=%d err=%v", sent, failed, err)
	}
	select {
	case message := <-messages:
		if !strings.Contains(message, "Subject: Re: Refund request") || !strings.Contains(message, "We issued the refund.") {
			t.Fatalf("message content: %s", message)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SMTP message not captured")
	}
	var status string
	if err := pool.QueryRow(ctx, `select status from email_outbox where conversation_id = $1`, conversationID).Scan(&status); err != nil || status != "sent" {
		t.Fatalf("outbox status: %s err=%v", status, err)
	}
	invitationID, err := email.EnqueueInvitation(db.WithOrg(ctx, orgID), db.NewRepo(pool), "Email Org", "new@email.test", "https://desk.test/invite/abc")
	if err != nil || invitationID == "" {
		t.Fatalf("enqueue invitation: id=%s err=%v", invitationID, err)
	}
	var kind, invitationSubject string
	if err := pool.QueryRow(ctx, `select kind, subject from email_outbox where id = $1`, invitationID).Scan(&kind, &invitationSubject); err != nil || kind != "invitation" || !strings.Contains(invitationSubject, "Email Org") {
		t.Fatalf("invitation outbox: kind=%s subject=%s err=%v", kind, invitationSubject, err)
	}
	_ = io.Discard
}
