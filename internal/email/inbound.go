// SPDX-License-Identifier: AGPL-3.0-only
package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/db"
	"chastedesk/internal/id"
	"chastedesk/internal/queue"
)

type InboundMessage struct {
	From       string
	FromName   string
	To         []string
	Subject    string
	TextBody   string
	HTMLBody   string
	MessageID  string
	InReplyTo  string
	References []string
}

type IngestResult struct {
	ConversationID  string
	MessageID       string
	Duplicate       bool
	NewConversation bool
}

var ticketSubjectPattern = regexp.MustCompile(`\[#([0-9]+)\]`)

func Ingest(ctx context.Context, pool *pgxpool.Pool, jobs *queue.Queue, inbound InboundMessage) (IngestResult, error) {
	org, err := db.NewRepo(pool).FirstOrganization(ctx)
	if err != nil {
		return IngestResult{}, fmt.Errorf("email: organization: %w", err)
	}
	from := strings.ToLower(strings.TrimSpace(inbound.From))
	if from == "" {
		return IngestResult{}, errors.New("email: sender is required")
	}
	messageID := NormalizeMessageID(inbound.MessageID)
	if messageID == "" {
		messageID = "<" + id.UUID() + "@chastedesk.local>"
	}
	subject := normalizeSubject(inbound.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	body := strings.TrimSpace(inbound.TextBody)
	if body == "" {
		body = stripHTML(inbound.HTMLBody)
	}
	if body == "" {
		body = "(empty email)"
	}
	if len(body) > 100000 {
		body = body[:100000]
	}
	var duplicateConversation string
	err = pool.QueryRow(ctx,
		`select conversation_id::text from messages where org_id = $1 and email_message_id = $2 limit 1`, org.ID, messageID).Scan(&duplicateConversation)
	if err == nil {
		return IngestResult{ConversationID: duplicateConversation, Duplicate: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return IngestResult{}, err
	}

	conversationID, existingStatus, err := findConversation(ctx, pool, org.ID, from, subject, messageID, inbound)
	if err != nil {
		return IngestResult{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return IngestResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	customerID, err := ensureCustomer(ctx, tx, org.ID, from, inbound.FromName)
	if err != nil {
		return IngestResult{}, err
	}
	newConversation := conversationID == ""
	if newConversation {
		conversationID = id.UUID()
		if _, err := tx.Exec(ctx,
			`insert into conversations (id, org_id, customer_id, channel, status, email_subject, email_thread_key)
			 values ($1, $2, $3, 'email', 'ai_active', $4, $5)`,
			conversationID, org.ID, customerID, subject, messageID); err != nil {
			return IngestResult{}, err
		}
	} else if existingStatus == "resolved" || existingStatus == "closed" {
		if _, err := tx.Exec(ctx, `update conversations set status = 'ai_active', resolved_at = null, closed_at = null, updated_at = now() where id = $1`, conversationID); err != nil {
			return IngestResult{}, err
		}
	}
	meta, err := json.Marshal(map[string]any{
		"emailMessageId": messageID, "emailSubject": subject, "emailFrom": from, "emailTo": inbound.To,
	})
	if err != nil {
		return IngestResult{}, err
	}
	messageRowID := id.UUID()
	var inserted string
	err = tx.QueryRow(ctx,
		`insert into messages (id, org_id, conversation_id, role, content, email_message_id, meta)
		 values ($1, $2, $3, 'customer', $4, $5, $6::jsonb)
		 on conflict (org_id, email_message_id) where email_message_id is not null do nothing
		 returning id::text`, messageRowID, org.ID, conversationID, body, messageID, string(meta)).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return IngestResult{ConversationID: conversationID, Duplicate: true}, nil
	}
	if err != nil {
		return IngestResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return IngestResult{}, err
	}
	if jobs != nil {
		if _, err := jobs.Send(ctx, "ai-respond", map[string]any{
			"orgId": org.ID, "conversationId": conversationID,
		}, queue.SendOptions{SingletonKey: "conv:" + conversationID}); err != nil {
			return IngestResult{}, fmt.Errorf("email: enqueue response: %w", err)
		}
	}
	_ = db.NewRepo(pool).WriteAuditNoOrg(ctx, &org.ID, db.AuditInput{
		ActorType: db.ActorSystem, Type: "email.inbound", TargetType: "conversation",
		TargetID: &conversationID, Payload: map[string]any{"from": from, "messageId": messageID, "newConversation": newConversation},
	})
	return IngestResult{ConversationID: conversationID, MessageID: inserted, NewConversation: newConversation}, nil
}

func findConversation(ctx context.Context, pool *pgxpool.Pool, orgID, from, subject, messageID string, inbound InboundMessage) (string, string, error) {
	if matches := ticketSubjectPattern.FindStringSubmatch(subject); len(matches) == 2 {
		var conversationID, status string
		err := pool.QueryRow(ctx,
			`select c.id::text, c.status::text from conversations c
			   join customers cu on cu.id = c.customer_id
			  where c.org_id = $1 and c.ticket_number = $2 and lower(cu.email) = lower($3)`,
			orgID, matches[1], from).Scan(&conversationID, &status)
		if err == nil {
			return conversationID, status, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", "", err
		}
	}
	ids := []string{NormalizeMessageID(inbound.InReplyTo)}
	for _, reference := range inbound.References {
		ids = append(ids, NormalizeMessageID(reference))
	}
	ids = uniqueNonEmpty(ids)
	if len(ids) > 0 {
		var conversationID, status string
		err := pool.QueryRow(ctx,
			`select c.id::text, c.status::text from messages m
			   join conversations c on c.id = m.conversation_id
			   join customers cu on cu.id = c.customer_id
			  where m.org_id = $1 and m.email_message_id = any($2) and lower(cu.email) = lower($3)
			  order by m.created_at desc limit 1`, orgID, ids, from).Scan(&conversationID, &status)
		if err == nil {
			return conversationID, status, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", "", err
		}
		err = pool.QueryRow(ctx,
			`select c.id::text, c.status::text from email_outbox o
			   join conversations c on c.id = o.conversation_id
			   join customers cu on cu.id = c.customer_id
			  where o.org_id = $1 and o.email_message_id = any($2) and lower(cu.email) = lower($3)
			  order by o.created_at desc limit 1`, orgID, ids, from).Scan(&conversationID, &status)
		if err == nil {
			return conversationID, status, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", "", err
		}
	}
	return "", "", nil
}

func ensureCustomer(ctx context.Context, tx pgx.Tx, orgID, email, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	if _, err := tx.Exec(ctx,
		`insert into customers (org_id, email, name) values ($1, $2, $3)
		 on conflict do nothing`, orgID, email, name); err != nil {
		return "", err
	}
	var customerID string
	if err := tx.QueryRow(ctx, `select id::text from customers where org_id = $1 and lower(email) = lower($2)`, orgID, email).Scan(&customerID); err != nil {
		return "", err
	}
	return customerID, nil
}

func NormalizeMessageID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "<") {
		value = "<" + value
	}
	if !strings.HasSuffix(value, ">") {
		value += ">"
	}
	return value
}

func normalizeSubject(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", ""))
	for {
		lower := strings.ToLower(value)
		switch {
		case strings.HasPrefix(lower, "re:"):
			value = strings.TrimSpace(value[3:])
		case strings.HasPrefix(lower, "fwd:"):
			value = strings.TrimSpace(value[4:])
		case strings.HasPrefix(lower, "fw:"):
			value = strings.TrimSpace(value[3:])
		default:
			if len(value) > 255 {
				value = value[:255]
			}
			return value
		}
	}
}

var htmlTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)

func stripHTML(value string) string {
	value = htmlTagPattern.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
