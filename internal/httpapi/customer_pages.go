// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/web/views"
)

func (s *Server) handleCustomersPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	shell, err := s.shellFor(r, sess, "customers", "Customers", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := s.repo.ListCustomers(db.WithOrg(r.Context(), sess.OrgID), query)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	customers := make([]views.CustomerRowView, 0, len(rows))
	open := 0
	for _, row := range rows {
		view := views.CustomerRowView{ID: row.ID, Conversations: row.Total, Open: row.Open}
		if row.Name != nil {
			view.Name = *row.Name
		}
		if row.Email != nil {
			view.Email = *row.Email
		}
		view.Initial = views.InitialOf(view.Name)
		if row.LastSeen != nil {
			view.LastSeen = views.RelativeTime(*row.LastSeen)
		} else {
			view.LastSeen = "no conversations"
		}
		open += row.Open
		customers = append(customers, view)
	}
	renderPage(w, views.CustomersPage(shell, views.CustomersView{
		Customers: customers, Query: query, Total: len(customers), Open: open,
	}))
}

func (s *Server) handleCustomerDetailPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	shell, err := s.shellFor(r, sess, "customers", "Customer", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	customer, err := s.repo.GetCustomer(ctx, r.PathValue("id"))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	detail := views.CustomerDetailView{ID: customer.ID, CreatedAt: views.RelativeTime(customer.CreatedAt)}
	if customer.Name != nil {
		detail.Name = *customer.Name
	}
	if customer.Email != nil {
		detail.Email = *customer.Email
	}
	detail.Initial = views.InitialOf(detail.Name)
	var attrs map[string]any
	if err := json.Unmarshal(customer.Attributes, &attrs); err != nil {
		attrs = map[string]any{}
	}
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sortStrings(keys)
	for _, key := range keys {
		detail.Attributes = append(detail.Attributes, views.InboxAttributeView{Key: key, Value: fmt.Sprint(attrs[key])})
	}
	conversations, err := s.repo.CustomerConversations(ctx, customer.ID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	open := 0
	for _, conversation := range conversations {
		preview := "No messages yet"
		if conversation.Preview != nil && strings.TrimSpace(*conversation.Preview) != "" {
			preview = truncateText(*conversation.Preview, 140)
		}
		if conversation.Status != "resolved" && conversation.Status != "closed" {
			open++
		}
		detail.Conversations = append(detail.Conversations, views.CustomerConversationView{
			ID: conversation.ID, Status: conversation.Status, Channel: views.ChannelLabel(conversation.Channel),
			CreatedAt: views.RelativeTime(conversation.CreatedAt), Preview: preview,
		})
	}
	detail.OpenCount = open
	detail.TotalCount = len(conversations)
	renderPage(w, views.CustomerDetailPage(shell, detail))
}

func (s *Server) handleApprovalsPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	shell, err := s.shellFor(r, sess, "approvals", "Approvals", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	pending, err := s.repo.ListApprovals(db.WithOrg(r.Context(), sess.OrgID), "pending_approval", 100)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	history, err := s.repo.ListApprovals(db.WithOrg(r.Context(), sess.OrgID), "decided", 20)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	toView := func(row db.ApprovalRow) views.ApprovalRowView {
		name := "Visitor"
		email := ""
		if row.CustomerName != nil {
			name = *row.CustomerName
		}
		if row.CustomerEmail != nil {
			email = *row.CustomerEmail
		}
		pretty := string(row.Args)
		var decoded any
		if err := json.Unmarshal(row.Args, &decoded); err == nil {
			if encoded, err := json.MarshalIndent(decoded, "", "  "); err == nil {
				pretty = string(encoded)
			}
		}
		return views.ApprovalRowView{
			ID: row.ID, ConversationID: row.ConversationID, CustomerName: name, CustomerEmail: email,
			ToolID: row.ToolID, Args: pretty, CreatedAt: views.RelativeTime(row.CreatedAt), Status: row.Status,
		}
	}
	view := views.ApprovalsView{}
	for _, row := range pending {
		view.Pending = append(view.Pending, toView(row))
	}
	for _, row := range history {
		view.History = append(view.History, toView(row))
	}
	renderPage(w, views.ApprovalsPage(shell, view))
}

func notFound(message string) error {
	return apperr.NotFound(message)
}
