// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"chastedesk/internal/auth"
	"chastedesk/internal/web/views"
)

func statusLabel(status string) string {
	return strings.ReplaceAll(status, "_", " ")
}

func (s *Server) inboxListView(rows []inboxListRow) []views.InboxConversationView {
	list := make([]views.InboxConversationView, 0, len(rows))
	for _, row := range rows {
		view := views.InboxConversationView{
			ID:          row.ID,
			Status:      row.Status,
			StatusLabel: statusLabel(row.Status),
			Channel:     row.Channel,
			CreatedAt:   row.CreatedAt.Format("2006-01-02 15:04"),
		}
		if row.CustomerName != nil && *row.CustomerName != "" {
			view.CustomerName = *row.CustomerName
		} else if row.CustomerEmail != nil {
			view.CustomerName = *row.CustomerEmail
		} else {
			view.CustomerName = "Visitor"
		}
		if row.CustomerEmail != nil {
			view.CustomerEmail = *row.CustomerEmail
		}
		list = append(list, view)
	}
	return list
}

func (s *Server) buildInboxDetail(ctx context.Context, orgID, conversationID string) (*views.InboxDetailView, error) {
	conversation, err := loadInboxConversation(ctx, s.pool, orgID, conversationID)
	if err != nil {
		return nil, err
	}
	detail := &views.InboxDetailView{ID: conversation.ID, Status: conversation.Status}

	if len(conversation.Handoff) > 0 && string(conversation.Handoff) != "null" {
		var parsed struct {
			Problem             string `json:"problem"`
			RecommendedNextStep string `json:"recommendedNextStep"`
			Evidence            []struct {
				Source  string `json:"source"`
				Version int    `json:"version"`
			} `json:"evidence"`
		}
		if err := json.Unmarshal(conversation.Handoff, &parsed); err == nil {
			handoff := &views.InboxHandoffView{Problem: parsed.Problem, RecommendedNextStep: parsed.RecommendedNextStep}
			for _, item := range parsed.Evidence {
				handoff.Evidence = append(handoff.Evidence, fmt.Sprintf("%s v%d", item.Source, item.Version))
			}
			detail.Handoff = handoff
		}
	}

	messages, err := loadInboxMessages(ctx, s.pool, conversation.ID)
	if err != nil {
		return nil, err
	}
	for _, message := range messages {
		view := views.InboxMessageView{
			ID: message.ID, Role: message.Role, RoleLabel: statusLabel(message.Role), Content: message.Content,
		}
		if len(message.Citations) > 0 && string(message.Citations) != "null" {
			var citations []struct {
				Quote string `json:"quote"`
			}
			if err := json.Unmarshal(message.Citations, &citations); err == nil {
				for i, citation := range citations {
					view.Citations = append(view.Citations, fmt.Sprintf("[%d] %s…", i+1, truncateText(citation.Quote, 60)))
				}
			}
		}
		detail.Messages = append(detail.Messages, view)
	}

	var (
		customerName  *string
		customerEmail *string
		attributes    []byte
	)
	if err := s.pool.QueryRow(ctx, `select name, email, attributes from customers where id = $1`, conversation.CustomerID).
		Scan(&customerName, &customerEmail, &attributes); err == nil {
		customer := &views.InboxCustomerView{}
		if customerName != nil {
			customer.Name = *customerName
		}
		if customerEmail != nil {
			customer.Email = *customerEmail
		}
		var attrs map[string]any
		if err := json.Unmarshal(attributes, &attrs); err != nil {
			attrs = map[string]any{}
		}
		for key, value := range attrs {
			customer.Attributes = append(customer.Attributes, views.InboxAttributeView{Key: key, Value: fmt.Sprint(value)})
		}
		recent, err := loadInboxRecent(ctx, s.pool, orgID, conversation.CustomerID, conversation.ID)
		if err != nil {
			return nil, err
		}
		for _, row := range recent {
			customer.Recent = append(customer.Recent, views.InboxRecentView{
				ID: row.ID, Status: statusLabel(row.Status), CreatedAt: row.CreatedAt.Format("2006-01-02"),
			})
			if row.Status != "resolved" && row.Status != "closed" {
				customer.OpenCount++
			}
		}
		detail.Customer = customer
	}

	actions, err := loadInboxActions(ctx, s.pool, orgID, conversation.ID)
	if err != nil {
		return nil, err
	}
	for _, action := range actions {
		pretty := string(action.Args)
		var decoded any
		if err := json.Unmarshal(action.Args, &decoded); err == nil {
			if encoded, err := json.MarshalIndent(decoded, "", "  "); err == nil {
				pretty = string(encoded)
			}
		}
		detail.Actions = append(detail.Actions, views.InboxActionView{
			ID: action.ID, ToolID: action.ToolID, Args: pretty, Status: action.Status,
			Pending: action.Status == "pending_approval",
		})
	}
	return detail, nil
}

func (s *Server) handleInboxPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	rows, err := s.queryInboxList(r.Context(), sess.OrgID, r.URL.Query().Get("status"), r.URL.Query().Get("q"))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	renderPage(w, views.InboxPage(s.inboxListView(rows), nil))
}

func (s *Server) handleInboxDetailPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	rows, err := s.queryInboxList(r.Context(), sess.OrgID, "", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	detail, err := s.buildInboxDetail(r.Context(), sess.OrgID, r.PathValue("id"))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	renderPage(w, views.InboxPage(s.inboxListView(rows), detail))
}

func (s *Server) handleInboxReplyForm(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	content := r.FormValue("content")
	if strings.TrimSpace(content) == "" || len(content) > 8000 {
		http.Error(w, "content must be between 1 and 8000 characters", http.StatusBadRequest)
		return
	}
	internal := r.FormValue("internal") == "on" || r.FormValue("internal") == "true" || r.FormValue("internal") == "1"
	if _, err := s.inboxReply(r.Context(), sess, r.PathValue("id"), content, internal); err != nil {
		s.respondError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/inbox/"+r.PathValue("id"), http.StatusSeeOther)
}

func (s *Server) handleInboxResolveForm(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	action := r.FormValue("action")
	if action != "resolve" && action != "reopen" {
		http.Error(w, "action must be resolve or reopen", http.StatusBadRequest)
		return
	}
	if err := s.inboxResolve(r.Context(), sess, r.PathValue("id"), action); err != nil {
		s.respondError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/inbox/"+r.PathValue("id"), http.StatusSeeOther)
}

func (s *Server) handleInboxActionForm(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	actionID := r.FormValue("actionId")
	decision := r.FormValue("decision")
	conversationID := r.FormValue("conversationId")
	if !uuidShape.MatchString(actionID) || (decision != "approve" && decision != "reject") {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if _, err := s.decideAction(r.Context(), sess.OrgID, sess.UserID, actionID, decision); err != nil {
		s.respondError(w, r, err)
		return
	}
	if conversationID != "" {
		http.Redirect(w, r, "/dashboard/inbox/"+conversationID, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/dashboard/inbox", http.StatusSeeOther)
}

func (s *Server) handleInboxCopilotForm(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	draft, _, _, err := s.copilotDraft(r.Context(), sess.OrgID, r.PathValue("id"))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	renderPage(w, views.InboxCopilotFragment(r.PathValue("id"), draft))
}

func (s *Server) handleInboxCopilotSendForm(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	content := r.FormValue("content")
	if strings.TrimSpace(content) == "" || len(content) > 8000 {
		http.Error(w, "content must be between 1 and 8000 characters", http.StatusBadRequest)
		return
	}
	if err := s.copilotSend(r.Context(), sess.OrgID, r.PathValue("id"), content); err != nil {
		s.respondError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/inbox/"+r.PathValue("id"), http.StatusSeeOther)
}
