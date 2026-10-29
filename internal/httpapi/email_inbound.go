// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"chastedesk/internal/email"
	"chastedesk/internal/ratelimit"
)

type postmarkInboundPayload struct {
	From     string `json:"From"`
	FromFull struct {
		Email string `json:"Email"`
		Name  string `json:"Name"`
	} `json:"FromFull"`
	To     string `json:"To"`
	ToFull []struct {
		Email string `json:"Email"`
		Name  string `json:"Name"`
	} `json:"ToFull"`
	Subject   string `json:"Subject"`
	TextBody  string `json:"TextBody"`
	HTMLBody  string `json:"HTMLBody"`
	MessageID string `json:"MessageID"`
	Headers   []struct {
		Name  string `json:"Name"`
		Value string `json:"Value"`
	} `json:"Headers"`
}

var emailReferenceSplit = regexp.MustCompile(`\s+`)

func (s *Server) handleEmailInbound(w http.ResponseWriter, r *http.Request) {
	if s.cfg.EmailInboundSecret == "" {
		http.NotFound(w, r)
		return
	}
	if !s.validInboundSecret(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "unauthorized", "message": "invalid inbound email secret"}})
		return
	}
	if _, err := ratelimit.Enforce(r.Context(), s.pool, "email-inbound:"+clientIP(r), 300, 60); err != nil {
		s.respondError(w, r, err)
		return
	}
	var payload postmarkInboundPayload
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	if err := dec.Decode(&payload); err != nil {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "The inbound email payload is not valid JSON.")
		return
	}
	from := strings.TrimSpace(payload.FromFull.Email)
	fromName := strings.TrimSpace(payload.FromFull.Name)
	if from == "" {
		if parsed, err := mail.ParseAddress(payload.From); err == nil {
			from = parsed.Address
			fromName = parsed.Name
		}
	}
	if from == "" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "The inbound email is missing a sender.")
		return
	}
	if !validEmail(from) {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "The inbound email sender is not valid.")
		return
	}
	if strings.EqualFold(from, s.cfg.EmailInboundAddress) {
		writeErrorBody(w, http.StatusUnprocessableEntity, "loop_detected", "The support mailbox cannot send mail to itself.")
		return
	}
	autoSubmitted := inboundHeader(payload.Headers, "Auto-Submitted")
	if autoSubmitted != "" && !strings.EqualFold(autoSubmitted, "no") {
		writeErrorBody(w, http.StatusUnprocessableEntity, "auto_submitted", "Automatically submitted messages are not accepted.")
		return
	}
	authResults := strings.ToLower(inboundHeader(payload.Headers, "Authentication-Results") + " " + inboundHeader(payload.Headers, "Received-SPF"))
	if strings.Contains(authResults, "dmarc=fail") || strings.Contains(authResults, "spf=fail") || strings.Contains(authResults, "dkim=fail") {
		writeErrorBody(w, http.StatusUnprocessableEntity, "authentication_failed", "The inbound email did not pass sender authentication.")
		return
	}
	to := make([]string, 0, len(payload.ToFull)+1)
	for _, recipient := range payload.ToFull {
		if recipient.Email != "" {
			to = append(to, strings.ToLower(recipient.Email))
		}
	}
	if len(to) == 0 && payload.To != "" {
		if parsed, err := mail.ParseAddress(payload.To); err == nil {
			to = append(to, strings.ToLower(parsed.Address))
		}
	}
	if s.cfg.EmailInboundAddress != "" && !containsAddress(to, s.cfg.EmailInboundAddress) {
		writeErrorBody(w, http.StatusUnprocessableEntity, "recipient_not_supported", "This mailbox is not configured for the email channel.")
		return
	}
	inReplyTo, references := inboundReferences(payload.Headers)
	messageID := payload.MessageID
	if messageID == "" {
		for _, header := range payload.Headers {
			if strings.EqualFold(header.Name, "Message-ID") {
				messageID = header.Value
				break
			}
		}
	}
	result, err := email.Ingest(r.Context(), s.pool, s.queue, email.InboundMessage{
		From: from, FromName: fromName, To: to, Subject: payload.Subject,
		TextBody: payload.TextBody, HTMLBody: payload.HTMLBody,
		MessageID: messageID, InReplyTo: inReplyTo, References: references,
	})
	if err != nil {
		s.logger.Warn("inbound email failed", "error", err)
		writeErrorBody(w, http.StatusInternalServerError, "email_failed", "The inbound email could not be stored.")
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"conversationId": result.ConversationID, "messageId": result.MessageID,
		"duplicate": result.Duplicate, "newConversation": result.NewConversation,
	})
}

func (s *Server) validInboundSecret(r *http.Request) bool {
	provided := r.Header.Get("X-ChasteDesk-Email-Secret")
	if provided == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if parts := strings.Fields(auth); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			provided = parts[1]
		}
	}
	if provided == "" {
		provided = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.EmailInboundSecret)) == 1
}

func containsAddress(addresses []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, address := range addresses {
		if strings.EqualFold(strings.TrimSpace(address), target) {
			return true
		}
	}
	return false
}

func inboundReferences(headers []struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}) (string, []string) {
	var inReplyTo string
	references := []string{}
	for _, header := range headers {
		switch strings.ToLower(header.Name) {
		case "in-reply-to":
			if fields := strings.Fields(header.Value); len(fields) > 0 {
				inReplyTo = fields[0]
			}
		case "references":
			references = append(references, emailReferenceSplit.Split(strings.TrimSpace(header.Value), -1)...)
		}
	}
	return inReplyTo, references
}

func inboundHeader(headers []struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}, name string) string {
	for _, header := range headers {
		if strings.EqualFold(header.Name, name) {
			return strings.TrimSpace(header.Value)
		}
	}
	return ""
}
