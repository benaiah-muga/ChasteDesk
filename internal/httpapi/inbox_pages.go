// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/web/views"
)

func statusLabel(status string) string {
	return strings.ReplaceAll(status, "_", " ")
}

type inboxListRow struct {
	TicketNumber       int64
	Priority           string
	Tags               []string
	SnoozedUntil       *time.Time
	FirstResponseDueAt *time.Time
	ResolutionDueAt    *time.Time
	ID                 string
	CustomerID         string
	Status             string
	Channel            string
	CreatedAt          time.Time
	FirstResponseAt    *time.Time
	Handoff            []byte
	CustomerName       *string
	CustomerEmail      *string
	UpdatedAt          time.Time
	LastMessage        string
	LastMessageRole    string
	AssignedName       string
	AssignedUserID     *string
	PendingActions     int
}

func (s *Server) queryInboxList(ctx context.Context, orgID, status, q string) ([]inboxListRow, error) {
	args := []any{orgID}
	filter := ""
	switch status {
	case "", "all":
	case "open":
		filter += " and c.status not in ('resolved','closed') and (c.snoozed_until is null or c.snoozed_until <= now())"
	case "resolved":
		filter += " and c.status in ('resolved','closed')"
	case "snoozed":
		filter += " and c.snoozed_until > now()"
	case "ai_active", "awaiting_human", "human_active", "closed":
		args = append(args, status)
		filter += fmt.Sprintf(" and c.status = $%d", len(args))
	default:
		return []inboxListRow{}, nil
	}
	if q != "" {
		args = append(args, q)
		filter += fmt.Sprintf(` and (cu.name ilike '%%' || $%d || '%%' or cu.email ilike '%%' || $%d || '%%'
			or c.ticket_number::text = $%d
			or exists (select 1 from unnest(c.tags) tag where tag ilike '%%' || $%d || '%%')
			or exists (select 1 from messages mq where mq.conversation_id = c.id and mq.content ilike '%%' || $%d || '%%'))`,
			len(args), len(args), len(args), len(args), len(args))
	}
	rows, err := s.pool.Query(ctx,
		`select c.ticket_number, c.priority, c.tags, c.snoozed_until, c.first_response_due_at, c.resolution_due_at,
		        c.id, c.customer_id::text, c.status::text, c.channel::text, c.created_at, c.first_response_at, c.handoff, c.updated_at,
		        cu.name, cu.email, coalesce(lm.content, ''), coalesce(lm.role::text, ''), coalesce(au.name, ''), au.id,
		        (select count(*)::int from actions a where a.conversation_id = c.id and a.status = 'pending_approval')
		   from conversations c
		   join customers cu on cu.id = c.customer_id
		   left join "user" au on au.id = c.assigned_user_id
		   left join lateral (
		     select content, role from messages m where m.conversation_id = c.id order by m.created_at desc limit 1
		   ) lm on true
		  where c.org_id = $1`+filter+`
		  order by c.updated_at desc limit 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []inboxListRow{}
	for rows.Next() {
		var row inboxListRow
		if err := rows.Scan(&row.TicketNumber, &row.Priority, &row.Tags, &row.SnoozedUntil, &row.FirstResponseDueAt, &row.ResolutionDueAt, &row.ID, &row.CustomerID, &row.Status, &row.Channel, &row.CreatedAt, &row.FirstResponseAt, &row.Handoff, &row.UpdatedAt, &row.CustomerName, &row.CustomerEmail, &row.LastMessage, &row.LastMessageRole, &row.AssignedName, &row.AssignedUserID, &row.PendingActions); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Server) inboxListView(rows []inboxListRow, activeID string) []views.InboxConversationView {
	list := make([]views.InboxConversationView, 0, len(rows))
	for _, row := range rows {
		view := views.InboxConversationView{
			ID:           row.ID,
			Status:       row.Status,
			StatusLabel:  statusLabel(row.Status),
			Channel:      row.Channel,
			ChannelLabel: views.ChannelLabel(row.Channel),
			CreatedAt:    row.CreatedAt.Format("2006-01-02 15:04"),
			Time:         views.RelativeTime(row.UpdatedAt),
			Preview:      truncateText(strings.TrimSpace(row.LastMessage), 150),
			Active:       activeID != "" && row.ID == activeID,
			Assigned:     row.AssignedName,
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

func (s *Server) inboxFilters(ctx context.Context, orgID, active string) ([]views.InboxFilterView, error) {
	counts, err := s.repo.ConversationStatusCounts(db.WithOrg(ctx, orgID))
	if err != nil {
		return nil, err
	}
	byStatus := map[string]int{}
	total, open, resolved := 0, 0, 0
	for _, item := range counts {
		byStatus[item.Status] = item.Count
		total += item.Count
		if item.Status == "resolved" || item.Status == "closed" {
			resolved += item.Count
		} else {
			open += item.Count
		}
	}
	return []views.InboxFilterView{
		{Key: "", Label: "All", Count: total, Active: active == "" || active == "all"},
		{Key: "open", Label: "Open", Count: open, Active: active == "open"},
		{Key: "ai_active", Label: "AI active", Count: byStatus["ai_active"], Active: active == "ai_active"},
		{Key: "awaiting_human", Label: "Awaiting human", Count: byStatus["awaiting_human"], Active: active == "awaiting_human"},
		{Key: "human_active", Label: "With human", Count: byStatus["human_active"], Active: active == "human_active"},
		{Key: "resolved", Label: "Resolved", Count: resolved, Active: active == "resolved"},
	}, nil
}

func (s *Server) buildInboxDetail(ctx context.Context, orgID, conversationID string) (*views.InboxDetailView, error) {
	conversation, err := loadInboxConversation(ctx, s.pool, orgID, conversationID)
	if err != nil {
		return nil, err
	}
	detail := &views.InboxDetailView{
		ID:           conversation.ID,
		Status:       conversation.Status,
		Channel:      conversation.Channel,
		ChannelLabel: views.ChannelLabel(conversation.Channel),
		AssignedName: conversation.AssignedName,
		Assigned:     conversation.AssignedName != "",
		CreatedAt:    views.RelativeTime(conversation.CreatedAt),
	}

	if len(conversation.Handoff) > 0 && string(conversation.Handoff) != "null" {
		var parsed struct {
			Problem             string            `json:"problem"`
			RecommendedNextStep string            `json:"recommendedNextStep"`
			Evidence            []json.RawMessage `json:"evidence"`
			Investigated        []string          `json:"investigated"`
			ActionsTaken        []string          `json:"actionsTaken"`
		}
		if err := json.Unmarshal(conversation.Handoff, &parsed); err == nil {
			handoff := &views.InboxHandoffView{Problem: parsed.Problem, RecommendedNextStep: parsed.RecommendedNextStep, Investigated: parsed.Investigated, Actions: parsed.ActionsTaken}
			for _, raw := range parsed.Evidence {
				var item struct {
					Source  string `json:"source"`
					Version int    `json:"version"`
					Quote   string `json:"quote"`
				}
				if err := json.Unmarshal(raw, &item); err == nil && item.Source != "" {
					handoff.Evidence = append(handoff.Evidence, fmt.Sprintf("%s v%d", item.Source, item.Version))
					continue
				}
				var plain string
				if err := json.Unmarshal(raw, &plain); err == nil && plain != "" {
					handoff.Evidence = append(handoff.Evidence, plain)
				}
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
			ID:        message.ID,
			Role:      message.Role,
			RoleLabel: statusLabel(message.Role),
			Content:   message.Content,
			CreatedAt: views.RelativeTime(message.CreatedAt),
		}
		switch message.Role {
		case "customer":
			view.Author = "Customer"
		case "agent_ai":
			view.Author = "Desk AI"
		case "agent_human":
			view.Author = "Teammate"
		case "internal_note":
			view.Author = "Internal note"
		default:
			view.Author = "System"
		}
		if len(message.Citations) > 0 && string(message.Citations) != "null" {
			var citations []struct {
				SourceName string          `json:"sourceName"`
				SourceID   string          `json:"sourceId"`
				Version    int             `json:"version"`
				Quote      string          `json:"quote"`
				Locator    json.RawMessage `json:"locator"`
			}
			if err := json.Unmarshal(message.Citations, &citations); err == nil {
				for index, citation := range citations {
					view.Citations = append(view.Citations, views.InboxCitationView{
						Index:      index + 1,
						SourceName: views.StringOr(citation.SourceName, views.StringOr(citation.SourceID, "Knowledge source")),
						Version:    citation.Version,
						Quote:      truncateText(citation.Quote, 120),
					})
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
	if err := s.pool.QueryRow(ctx, `select name, email, attributes from customers where id = $1 and org_id = $2`, conversation.CustomerID, orgID).
		Scan(&customerName, &customerEmail, &attributes); err == nil {
		customer := &views.InboxCustomerView{}
		if customerName != nil {
			customer.Name = *customerName
		}
		if customerEmail != nil {
			customer.Email = *customerEmail
		}
		customer.Initial = views.InitialOf(customer.Name)
		var attrs map[string]any
		if err := json.Unmarshal(attributes, &attrs); err != nil {
			attrs = map[string]any{}
		}
		keys := make([]string, 0, len(attrs))
		for key := range attrs {
			keys = append(keys, key)
		}
		sortStrings(keys)
		for _, key := range keys {
			customer.Attributes = append(customer.Attributes, views.InboxAttributeView{Key: key, Value: fmt.Sprint(attrs[key])})
		}
		recent, err := loadInboxRecent(ctx, s.pool, orgID, conversation.CustomerID, conversation.ID)
		if err != nil {
			return nil, err
		}
		for _, row := range recent {
			customer.Recent = append(customer.Recent, views.InboxRecentView{
				ID: row.ID, Status: statusLabel(row.Status), CreatedAt: row.CreatedAt.Format("Jan 2"),
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
		pending := action.Status == "pending_approval"
		if pending {
			detail.PendingApprovals++
		}
		detail.Actions = append(detail.Actions, views.InboxActionView{
			ID: action.ID, ToolID: action.ToolID, Args: pretty, Status: action.Status,
			Pending: pending, CreatedAt: views.RelativeTime(action.CreatedAt),
		})
	}
	return detail, nil
}

func (s *Server) handleInboxPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	shell, err := s.shellFor(r, sess, "inbox", "Inbox", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	status := r.URL.Query().Get("status")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := s.queryInboxList(r.Context(), sess.OrgID, status, query)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	filters, err := s.inboxFilters(r.Context(), sess.OrgID, status)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	renderPage(w, views.InboxPage(shell, views.InboxView{
		List:         s.inboxListView(rows, ""),
		Filters:      filters,
		Status:       status,
		Query:        query,
		TotalOpen:    shell.OpenConversations,
		PendingCount: shell.PendingApprovals,
	}))
}

func (s *Server) handleInboxDetailPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	shell, err := s.shellFor(r, sess, "inbox", "Inbox", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	detail, err := s.buildInboxDetail(r.Context(), sess.OrgID, r.PathValue("id"))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	rows, err := s.queryInboxList(r.Context(), sess.OrgID, "", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	filters, err := s.inboxFilters(r.Context(), sess.OrgID, "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	renderPage(w, views.InboxPage(shell, views.InboxView{
		List:         s.inboxListView(rows, detail.ID),
		Filters:      filters,
		Detail:       detail,
		TotalOpen:    shell.OpenConversations,
		PendingCount: shell.PendingApprovals,
	}))
}

func (s *Server) handleInboxListPartial(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := s.queryInboxList(r.Context(), sess.OrgID, status, query)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	filters, err := s.inboxFilters(r.Context(), sess.OrgID, status)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.InboxConversationList(views.InboxView{List: s.inboxListView(rows, ""), Filters: filters, Status: status, Query: query}).Render(r.Context(), w)
}

func (s *Server) handleInboxLivePartial(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	detail, err := s.buildInboxDetail(r.Context(), sess.OrgID, r.PathValue("id"))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.InboxDetailLive(*detail).Render(r.Context(), w)
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

func (s *Server) handleInboxAssignForm(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	conversationID := r.PathValue("id")
	var assigned *string
	if r.FormValue("assign") != "clear" {
		value := sess.UserID
		assigned = &value
	}
	tag, err := s.pool.Exec(r.Context(), `update conversations set assigned_user_id = $2, updated_at = now() where id = $1 and org_id = $3`,
		conversationID, assigned, sess.OrgID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		s.respondError(w, r, apperr.NotFound("conversation not found"))
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "conversation.updated",
		TargetType: "conversation", TargetID: &conversationID, Payload: map[string]any{"assigned": assigned != nil},
	})
	http.Redirect(w, r, "/dashboard/inbox/"+conversationID, http.StatusSeeOther)
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
	returnTo := r.FormValue("returnTo")
	if returnTo == "/dashboard/approvals" {
		http.Redirect(w, r, returnTo, http.StatusSeeOther)
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
	draft, _, grounded, err := s.copilotDraft(r.Context(), sess.OrgID, r.PathValue("id"))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.InboxCopilotFragment(r.PathValue("id"), draft, grounded).Render(r.Context(), w)
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
	if _, err := s.inboxReply(r.Context(), sess, r.PathValue("id"), content, false); err != nil {
		s.respondError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/inbox/"+r.PathValue("id"), http.StatusSeeOther)
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
