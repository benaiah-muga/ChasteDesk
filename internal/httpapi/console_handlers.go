// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"chastedesk/internal/apperr"
	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/ratelimit"
	"chastedesk/internal/web/views"
)

type consoleConversation struct {
	ID                 string               `json:"id"`
	TicketNumber       int64                `json:"ticketNumber"`
	Priority           string               `json:"priority"`
	Tags               []string             `json:"tags"`
	SnoozedUntil       *time.Time           `json:"snoozedUntil,omitempty"`
	FirstResponseDueAt *time.Time           `json:"firstResponseDueAt,omitempty"`
	ResolutionDueAt    *time.Time           `json:"resolutionDueAt,omitempty"`
	SLABreached        bool                 `json:"slaBreached"`
	Status             string               `json:"status"`
	StatusLabel        string               `json:"statusLabel"`
	Channel            string               `json:"channel"`
	ChannelLabel       string               `json:"channelLabel"`
	CreatedAt          time.Time            `json:"createdAt"`
	UpdatedAt          time.Time            `json:"updatedAt"`
	FirstResponseAt    *time.Time           `json:"firstResponseAt,omitempty"`
	Customer           consoleCustomerRef   `json:"customer"`
	Preview            string               `json:"preview"`
	LastRole           string               `json:"lastRole"`
	Assigned           *consoleAssignedUser `json:"assigned"`
	PendingApprovals   int                  `json:"pendingApprovals"`
}

type consoleCustomerRef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Initial string `json:"initial"`
}

type consoleAssignedUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func consoleInitial(value string) string {
	return views.InitialOf(value)
}

func (s *Server) consoleSessionPayload(ctx context.Context, sess *auth.Session) (map[string]any, error) {
	ctxOrg := db.WithOrg(ctx, sess.OrgID)
	counts, err := s.repo.OverviewCounts(ctxOrg)
	if err != nil {
		return nil, err
	}
	orgName, err := s.repo.OrgName(ctxOrg)
	if err != nil {
		return nil, err
	}
	onboarding, err := s.repo.OnboardingState(ctxOrg)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"authenticated": true,
		"setupRequired": false,
		"user": map[string]any{
			"id": sess.UserID, "name": sess.Name, "email": sess.Email,
			"role": string(sess.Role), "initial": consoleInitial(sess.Name),
		},
		"org": map[string]any{"id": sess.OrgID, "name": orgName, "initial": consoleInitial(orgName)},
		"permissions": map[string]any{
			"canAudit":          auth.Can(sess.Role, auth.CapAuditRead),
			"canSettings":       auth.Can(sess.Role, auth.CapSettingsRead),
			"canSettingsWrite":  auth.Can(sess.Role, auth.CapSettingsWrite),
			"canKnowledgeWrite": auth.Can(sess.Role, auth.CapKnowledgeWrite),
			"canInbox":          auth.Can(sess.Role, auth.CapInboxUse),
		},
		"onboarding": map[string]any{
			"complete":           onboarding.Complete,
			"providerConfigured": onboarding.ProviderConfigured,
			"knowledgeAdded":     onboarding.KnowledgeAdded,
			"providerCount":      counts.EnabledProviders,
			"sourceCount":        counts.Sources,
		},
		"counts": countsPayload(counts),
	}, nil
}

func countsPayload(counts db.OverviewCounts) map[string]any {
	return map[string]any{
		"openConversations": counts.OpenConversations,
		"awaitingHuman":     counts.AwaitingHuman,
		"slaBreached":       counts.SLABreached,
		"pendingApprovals":  counts.PendingApprovals,
		"customers":         counts.Customers,
		"sources":           counts.Sources,
		"readySources":      counts.ReadySources,
		"failedSources":     counts.FailedSources,
		"enabledProviders":  counts.EnabledProviders,
		"widgetOrigins":     counts.WidgetOrigins,
	}
}

func (s *Server) handleConsoleSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	exists, err := s.repo.OrgExists(r.Context())
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	sess, err := s.sessionFromRequest(r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if sess == nil || sess.OrgID == "" {
		writeData(w, http.StatusOK, map[string]any{
			"authenticated": false,
			"setupRequired": !exists,
		})
		return
	}
	payload, err := s.consoleSessionPayload(r.Context(), sess)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, payload)
}

type consoleSetupBody struct {
	OrgName  string `json:"orgName"`
	UserName string `json:"userName"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleConsoleSetup(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[consoleSetupBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body.OrgName = strings.TrimSpace(body.OrgName)
	body.UserName = strings.TrimSpace(body.UserName)
	body.Email = strings.TrimSpace(body.Email)
	switch {
	case body.OrgName == "" || len(body.OrgName) > 100:
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Workspace name must be between 1 and 100 characters.")
		return
	case body.UserName == "" || len(body.UserName) > 100:
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Your name must be between 1 and 100 characters.")
		return
	case !validEmail(body.Email):
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Enter a valid email address.")
		return
	case len(body.Password) < 8 || len(body.Password) > 200:
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Password must be between 8 and 200 characters.")
		return
	}
	if _, err := ratelimit.Enforce(r.Context(), s.pool, "setup:"+clientIP(r), 20, 3600); err != nil {
		s.respondError(w, r, err)
		return
	}
	exists, err := s.repo.OrgExists(r.Context())
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if exists {
		writeErrorBody(w, http.StatusConflict, "conflict", "This installation already has a workspace. Sign in instead.")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	orgID, userID, err := s.repo.Bootstrap(r.Context(), db.BootstrapInput{
		OrgName: body.OrgName, UserName: body.UserName, Email: body.Email, PasswordHash: hash,
	})
	if err != nil {
		if errors.Is(err, db.ErrOrgExists) || db.IsUniqueViolation(err) {
			writeErrorBody(w, http.StatusConflict, "conflict", "This installation already has a workspace. Sign in instead.")
			return
		}
		s.respondError(w, r, err)
		return
	}
	if err := s.startSession(w, r, userID); err != nil {
		s.respondError(w, r, err)
		return
	}
	s.auditAuthEvent(r, userID, "auth.login", "session")
	sess := &auth.Session{UserID: userID, Name: body.UserName, Email: body.Email, OrgID: orgID, Role: auth.RoleOwner}
	payload, err := s.consoleSessionPayload(r.Context(), sess)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, payload)
}

func consoleConversationFromRow(row inboxListRow) consoleConversation {
	customerName := "Visitor"
	if row.CustomerName != nil && strings.TrimSpace(*row.CustomerName) != "" {
		customerName = *row.CustomerName
	} else if row.CustomerEmail != nil && strings.TrimSpace(*row.CustomerEmail) != "" {
		customerName = *row.CustomerEmail
	}
	email := ""
	if row.CustomerEmail != nil {
		email = *row.CustomerEmail
	}
	item := consoleConversation{
		ID: row.ID, TicketNumber: row.TicketNumber, Priority: row.Priority, Tags: row.Tags, SnoozedUntil: row.SnoozedUntil,
		FirstResponseDueAt: row.FirstResponseDueAt, ResolutionDueAt: row.ResolutionDueAt, SLABreached: slaBreached(row.FirstResponseAt, row.FirstResponseDueAt, row.ResolutionDueAt, row.Status),
		Status: row.Status, StatusLabel: statusLabel(row.Status),
		Channel: row.Channel, ChannelLabel: views.ChannelLabel(row.Channel),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, FirstResponseAt: row.FirstResponseAt,
		Customer: consoleCustomerRef{ID: row.CustomerID, Name: customerName, Email: email, Initial: consoleInitial(customerName)},
		Preview:  truncateText(strings.TrimSpace(row.LastMessage), 160),
		LastRole: row.LastMessageRole, PendingApprovals: row.PendingActions,
	}
	if row.AssignedName != "" {
		assignedID := ""
		if row.AssignedUserID != nil {
			assignedID = *row.AssignedUserID
		}
		item.Assigned = &consoleAssignedUser{ID: assignedID, Name: row.AssignedName}
	}
	if item.Preview == "" {
		item.Preview = "No messages yet"
	}
	return item
}

func statusCountMap(items []db.StatusCount) map[string]int {
	out := map[string]int{}
	for _, item := range items {
		out[item.Status] = item.Count
	}
	return out
}

func (s *Server) handleConsoleOverview(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	ctx := db.WithOrg(r.Context(), sess.OrgID)
	counts, err := s.repo.OverviewCounts(ctx)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	metrics, trendRows, err := s.analyticsPageData(r, sess.OrgID, 14)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	rows, err := s.queryInboxList(r.Context(), sess.OrgID, "", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	recent := make([]consoleConversation, 0, 6)
	for index, row := range rows {
		if index >= 6 {
			break
		}
		recent = append(recent, consoleConversationFromRow(row))
	}
	statusCounts, err := s.repo.ConversationStatusCounts(ctx)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	byStatus := statusCountMap(statusCounts)
	statusMix := []map[string]any{
		{"status": "ai_active", "label": "AI active", "count": byStatus["ai_active"]},
		{"status": "awaiting_human", "label": "Awaiting human", "count": byStatus["awaiting_human"]},
		{"status": "human_active", "label": "With human", "count": byStatus["human_active"]},
		{"status": "resolved", "label": "Resolved", "count": byStatus["resolved"] + byStatus["closed"]},
	}
	trend := make([]map[string]any, 0, len(trendRows))
	for _, point := range trendRows {
		trend = append(trend, map[string]any{"date": point["date"], "total": point["total"]})
	}
	metricsPayload := map[string]any{
		"aiResolutionRate": nil, "p50FirstResponseSeconds": nil, "p50ResolutionSeconds": nil,
		"totalConversations": nil, "latestDate": nil,
	}
	if metrics != nil {
		metricsPayload["aiResolutionRate"] = metrics.AIResolutionRate
		metricsPayload["p50FirstResponseSeconds"] = metrics.P50FirstResponseSeconds
		metricsPayload["p50ResolutionSeconds"] = metrics.P50ResolutionSeconds
		metricsPayload["totalConversations"] = metrics.TotalConversations
	}
	if len(trend) > 0 {
		metricsPayload["latestDate"] = trend[len(trend)-1]["date"]
	}

	attention := []map[string]any{}
	addAttention := func(kind, title, body, href, severity string, count int) {
		attention = append(attention, map[string]any{
			"kind": kind, "title": title, "body": body, "href": href, "severity": severity, "count": count,
		})
	}
	if counts.SLABreached > 0 {
		addAttention("sla", fmt.Sprintf("%d %s breaching SLA", counts.SLABreached, pluralWord(counts.SLABreached, "ticket", "tickets")), "A first-response or resolution target is overdue.", "/dashboard/inbox?sla=breached", "danger", counts.SLABreached)
	}
	if counts.PendingApprovals > 0 {
		addAttention("approval", fmt.Sprintf("%d %s waiting for a decision", counts.PendingApprovals, pluralWord(counts.PendingApprovals, "approval", "approvals")), "The agent stopped before executing a risky action.", "/dashboard/approvals", "warn", counts.PendingApprovals)
	}
	if counts.AwaitingHuman > 0 {
		addAttention("human", awaitingLabel(counts.AwaitingHuman), "The AI escalated with a complete handoff.", "/dashboard/inbox?status=awaiting_human", "danger", counts.AwaitingHuman)
	}
	if counts.FailedSources > 0 {
		addAttention("source", fmt.Sprintf("%d %s failed to index", counts.FailedSources, pluralWord(counts.FailedSources, "source", "sources")), "Inspect the error and retry the source.", "/dashboard/knowledge?status=failed", "danger", counts.FailedSources)
	}
	if counts.EnabledProviders == 0 {
		addAttention("provider", "Connect an AI provider", "Every conversation is routing to a human until a provider is connected.", "/dashboard/settings/providers", "warn", 1)
	}
	if counts.ReadySources == 0 {
		addAttention("knowledge", "Add your first knowledge source", "The AI will escalate instead of guessing until it has approved material.", "/dashboard/knowledge?add=1", "info", 1)
	}

	readiness := []map[string]any{
		{"key": "provider", "title": "AI provider", "body": providerReadiness(counts.EnabledProviders), "done": counts.EnabledProviders > 0, "href": "/dashboard/settings/providers", "action": actionWord(counts.EnabledProviders > 0, "Review", "Connect")},
		{"key": "knowledge", "title": "Knowledge", "body": knowledgeReadiness(counts.ReadySources, counts.Sources), "done": counts.ReadySources > 0, "href": "/dashboard/knowledge", "action": actionWord(counts.ReadySources > 0, "Manage", "Add")},
		{"key": "policy", "title": "Agent policy", "body": "Answer, escalate, and budget rules", "done": counts.EnabledProviders > 0, "href": "/dashboard/settings/agent", "action": "Review"},
		{"key": "widget", "title": "Widget origins", "body": originsReadiness(counts.WidgetOrigins), "done": counts.WidgetOrigins > 0, "href": "/dashboard/settings/widget", "action": actionWord(counts.WidgetOrigins > 0, "Manage", "Allow")},
	}
	statusText := "AI is answering from ready knowledge."
	statusOK := true
	switch {
	case counts.EnabledProviders == 0:
		statusText = "AI is paused. Every conversation routes to a human until a provider is connected."
		statusOK = false
	case counts.ReadySources == 0:
		statusText = "The AI has no ready knowledge. It will escalate rather than guess."
		statusOK = false
	default:
		statusText = fmt.Sprintf("AI is answering from %d ready %s across %d open %s.", counts.ReadySources, pluralWord(counts.ReadySources, "source", "sources"), counts.OpenConversations, pluralWord(counts.OpenConversations, "conversation", "conversations"))
	}
	writeData(w, http.StatusOK, map[string]any{
		"greeting":  greeting(sess.Name),
		"today":     time.Now().Format("2006-01-02"),
		"status":    map[string]any{"ok": statusOK, "text": statusText},
		"counts":    countsPayload(counts),
		"metrics":   metricsPayload,
		"recent":    recent,
		"attention": attention,
		"readiness": readiness,
		"statusMix": statusMix,
		"trend":     trend,
	})
}

func consoleMemberPayload(row db.MemberRow, selfID string) map[string]any {
	return map[string]any{
		"id": row.UserID, "name": row.Name, "email": row.Email, "role": row.Role,
		"initial": consoleInitial(row.Name), "self": row.UserID == selfID,
	}
}

func (s *Server) handleConsoleInbox(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	assignee := r.URL.Query().Get("assignee")
	priority := r.URL.Query().Get("priority")
	tag := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("tag")))
	slaFilter := r.URL.Query().Get("sla")
	sortOrder := r.URL.Query().Get("sort")
	rows, err := s.queryInboxList(r.Context(), sess.OrgID, status, query)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	filtered := rows[:0]
	for _, row := range rows {
		switch assignee {
		case "", "all":
		case "me":
			if row.AssignedUserID == nil || *row.AssignedUserID != sess.UserID {
				continue
			}
		case "unassigned":
			if row.AssignedUserID != nil {
				continue
			}
		default:
			if row.AssignedUserID == nil || *row.AssignedUserID != assignee {
				continue
			}
		}
		if priority != "" && priority != "all" && row.Priority != priority {
			continue
		}
		if slaFilter == "breached" && !slaBreached(row.FirstResponseAt, row.FirstResponseDueAt, row.ResolutionDueAt, row.Status) {
			continue
		}
		if tag != "" {
			found := false
			for _, value := range row.Tags {
				if strings.ToLower(value) == tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		filtered = append(filtered, row)
	}
	priorityRank := map[string]int{"urgent": 0, "high": 1, "normal": 2, "low": 3}
	switch sortOrder {
	case "oldest":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].UpdatedAt.Before(filtered[j].UpdatedAt) })
	case "priority":
		sort.SliceStable(filtered, func(i, j int) bool {
			left, right := priorityRank[filtered[i].Priority], priorityRank[filtered[j].Priority]
			if left == right {
				return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
			}
			return left < right
		})
	default:
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt) })
	}
	items := make([]consoleConversation, 0, len(filtered))
	for _, row := range filtered {
		items = append(items, consoleConversationFromRow(row))
	}
	statusCounts, err := s.repo.ConversationStatusCounts(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	counts := statusCountMap(statusCounts)
	total, open, resolved := 0, 0, 0
	for key, count := range counts {
		total += count
		if key == "resolved" || key == "closed" {
			resolved += count
		} else {
			open += count
		}
	}
	counts["total"] = total
	counts["open"] = open
	counts["resolved_total"] = resolved
	var snoozed int
	if err := s.pool.QueryRow(r.Context(),
		`select count(*)::int from conversations where org_id = $1 and snoozed_until > now() and status not in ('resolved','closed')`, sess.OrgID).Scan(&snoozed); err != nil {
		s.respondError(w, r, err)
		return
	}
	counts["snoozed"] = snoozed
	counts["open"] = open - snoozed
	var slaBreachedCount int
	if err := s.pool.QueryRow(r.Context(),
		`select count(*)::int from conversations
		  where org_id = $1 and status not in ('resolved','closed')
		    and ((first_response_at is null and first_response_due_at < now()) or (resolution_due_at < now()))`, sess.OrgID).Scan(&slaBreachedCount); err != nil {
		s.respondError(w, r, err)
		return
	}
	counts["slaBreached"] = slaBreachedCount
	tagRows, err := s.pool.Query(r.Context(),
		`select tag, count(*)::int from (
		   select unnest(tags) as tag from conversations where org_id = $1
		 ) tagged group by tag order by lower(tag)`, sess.OrgID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	availableTags := []map[string]any{}
	for tagRows.Next() {
		var name string
		var count int
		if err := tagRows.Scan(&name, &count); err != nil {
			tagRows.Close()
			s.respondError(w, r, err)
			return
		}
		availableTags = append(availableTags, map[string]any{"name": name, "count": count})
	}
	if err := tagRows.Err(); err != nil {
		tagRows.Close()
		s.respondError(w, r, err)
		return
	}
	tagRows.Close()
	memberRows, err := s.repo.ListMembers(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	members := make([]map[string]any, 0, len(memberRows))
	for _, member := range memberRows {
		members = append(members, consoleMemberPayload(member, sess.UserID))
	}
	writeData(w, http.StatusOK, map[string]any{
		"conversations": items,
		"counts":        counts,
		"members":       members,
		"total":         len(items),
		"availableTags": availableTags,
	})
}

type consoleBulkInboxBody struct {
	IDs            []string `json:"ids"`
	Action         string   `json:"action"`
	AssignedUserID *string  `json:"assignedUserId"`
	Priority       *string  `json:"priority"`
	SnoozedUntil   *string  `json:"snoozedUntil"`
	ClearSnooze    bool     `json:"clearSnooze"`
}

func (s *Server) handleConsoleInboxBulk(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[consoleBulkInboxBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if len(body.IDs) == 0 || len(body.IDs) > 100 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Choose between 1 and 100 conversations.")
		return
	}
	ids := []string{}
	for _, id := range body.IDs {
		if !uuidShape.MatchString(id) {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "Conversation ids must be UUIDs.")
			return
		}
		ids = append(ids, id)
	}
	updated, failed := []string{}, []map[string]string{}
	switch body.Action {
	case "resolve", "reopen", "close":
		for _, id := range ids {
			if err := s.inboxResolve(r.Context(), sess, id, body.Action); err != nil {
				failed = append(failed, map[string]string{"id": id, "message": err.Error()})
				continue
			}
			updated = append(updated, id)
		}
	case "priority":
		if body.Priority == nil || !ticketPriorities[strings.ToLower(strings.TrimSpace(*body.Priority))] {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "Priority must be low, normal, high, or urgent.")
			return
		}
		priority := strings.ToLower(strings.TrimSpace(*body.Priority))
		for _, id := range ids {
			if _, err := s.repo.UpdateTicketFields(db.WithOrg(r.Context(), sess.OrgID), id, db.TicketUpdate{Priority: &priority}); err != nil {
				failed = append(failed, map[string]string{"id": id, "message": err.Error()})
				continue
			}
			updated = append(updated, id)
			s.publishConversationEvent(r.Context(), sess.OrgID, id, "ticket.updated")
			_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
				ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "conversation.ticket_updated",
				TargetType: "conversation", TargetID: &id, Payload: map[string]any{"priority": priority},
			})
		}
	case "snooze":
		var until *time.Time
		if !body.ClearSnooze && body.SnoozedUntil != nil {
			parsed, err := parseTicketSnooze(*body.SnoozedUntil)
			if err != nil {
				s.respondError(w, r, err)
				return
			}
			until = parsed
		}
		if until == nil && !body.ClearSnooze {
			writeErrorBody(w, http.StatusBadRequest, "bad_request", "Choose a snooze time or clear it.")
			return
		}
		for _, id := range ids {
			if _, err := s.repo.UpdateTicketFields(db.WithOrg(r.Context(), sess.OrgID), id, db.TicketUpdate{SnoozedUntil: until, ClearSnooze: body.ClearSnooze || until == nil}); err != nil {
				failed = append(failed, map[string]string{"id": id, "message": err.Error()})
				continue
			}
			updated = append(updated, id)
			s.publishConversationEvent(r.Context(), sess.OrgID, id, "ticket.updated")
			_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
				ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "conversation.ticket_updated",
				TargetType: "conversation", TargetID: &id, Payload: map[string]any{"snoozedUntil": until},
			})
		}
	case "assign":
		if body.AssignedUserID != nil {
			var member bool
			if err := s.pool.QueryRow(r.Context(),
				`select exists(select 1 from member where organization_id = $1 and user_id = $2)`,
				sess.OrgID, *body.AssignedUserID).Scan(&member); err != nil {
				s.respondError(w, r, err)
				return
			}
			if !member {
				writeErrorBody(w, http.StatusBadRequest, "bad_request", "That teammate is not a member of this workspace.")
				return
			}
		}
		tag, err := s.pool.Exec(r.Context(),
			`update conversations set assigned_user_id = $3, updated_at = now()
			 where org_id = $1 and id = any($2::uuid[])`, sess.OrgID, ids, body.AssignedUserID)
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		for _, id := range ids {
			updated = append(updated, id)
			_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
				ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "conversation.updated",
				TargetType: "conversation", TargetID: &id, Payload: map[string]any{"assigned": body.AssignedUserID != nil},
			})
		}
		_ = tag
	default:
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Action must be resolve, close, reopen, assign, priority, or snooze.")
		return
	}
	writeData(w, http.StatusOK, map[string]any{"updated": updated, "failed": failed})
}

type consoleCustomerRow struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Email       string            `json:"email"`
	Initial     string            `json:"initial"`
	Attributes  map[string]string `json:"attributes"`
	CreatedAt   time.Time         `json:"createdAt"`
	LastSeen    *time.Time        `json:"lastSeen"`
	Total       int               `json:"total"`
	Open        int               `json:"open"`
	LastStatus  *string           `json:"lastStatus"`
	LastChannel *string           `json:"lastChannel"`
}

func consoleAttributes(raw []byte) map[string]string {
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(decoded))
	keys := make([]string, 0, len(decoded))
	for key := range decoded {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = fmt.Sprint(decoded[key])
	}
	return out
}

func (s *Server) handleConsoleCustomers(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	status := r.URL.Query().Get("status")
	sortBy := r.URL.Query().Get("sort")
	if status == "" {
		status = "all"
	}
	order := "last_seen desc nulls last, cu.created_at desc"
	if sortBy == "name" {
		order = "coalesce(cu.name, cu.email) asc"
	} else if sortBy == "newest" {
		order = "cu.created_at desc"
	}
	rows, err := s.pool.Query(r.Context(),
		`select cu.id::text, coalesce(cu.name, ''), coalesce(cu.email, ''), cu.created_at, cu.attributes,
		        max(c.updated_at) as last_seen,
		        count(c.id)::int as total,
		        count(c.id) filter (where c.status not in ('resolved','closed'))::int as open,
		        (array_agg(c.status::text order by c.updated_at desc) filter (where c.id is not null))[1] as last_status,
		        (array_agg(c.channel::text order by c.updated_at desc) filter (where c.id is not null))[1] as last_channel
		   from customers cu
		   left join conversations c on c.customer_id = cu.id and c.org_id = cu.org_id
		  where cu.org_id = $1
		    and ($2 = '' or cu.name ilike '%' || $2 || '%' or cu.email ilike '%' || $2 || '%')
		  group by cu.id, cu.name, cu.email, cu.created_at, cu.attributes
		 having ($3 = 'all' or count(c.id) filter (where c.status not in ('resolved','closed')) > 0)
		  order by `+order+` limit 200`, sess.OrgID, query, status)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	defer rows.Close()
	customers := []consoleCustomerRow{}
	plans := map[string]bool{}
	totalOpen := 0
	for rows.Next() {
		var (
			row        consoleCustomerRow
			attributes []byte
		)
		if err := rows.Scan(&row.ID, &row.Name, &row.Email, &row.CreatedAt, &attributes, &row.LastSeen, &row.Total, &row.Open, &row.LastStatus, &row.LastChannel); err != nil {
			s.respondError(w, r, err)
			return
		}
		row.Initial = consoleInitial(row.Name)
		if row.Name == "" {
			row.Name = row.Email
		}
		row.Attributes = consoleAttributes(attributes)
		if plan := row.Attributes["plan"]; plan != "" {
			plans[plan] = true
		}
		totalOpen += row.Open
		customers = append(customers, row)
	}
	if err := rows.Err(); err != nil {
		s.respondError(w, r, err)
		return
	}
	planNames := make([]string, 0, len(plans))
	for plan := range plans {
		planNames = append(planNames, plan)
	}
	sort.Strings(planNames)
	writeData(w, http.StatusOK, map[string]any{
		"customers": customers, "query": query, "totalOpen": totalOpen, "availablePlans": planNames,
	})
}

func (s *Server) handleConsoleCustomer(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		s.respondError(w, r, apperr.NotFound("customer not found"))
		return
	}
	customer, err := s.repo.GetCustomer(db.WithOrg(r.Context(), sess.OrgID), id)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	rows, err := s.pool.Query(r.Context(),
		`select c.id::text, c.status::text, c.channel::text, c.created_at, c.updated_at,
		        coalesce((select m.content from messages m where m.conversation_id = c.id order by m.created_at desc limit 1), '')
		   from conversations c where c.org_id = $1 and c.customer_id = $2
		  order by c.updated_at desc limit 100`, sess.OrgID, id)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	defer rows.Close()
	conversations := []map[string]any{}
	open := 0
	var lastSeen *time.Time
	for rows.Next() {
		var (
			conversationID string
			status         string
			channel        string
			createdAt      time.Time
			updatedAt      time.Time
			preview        string
		)
		if err := rows.Scan(&conversationID, &status, &channel, &createdAt, &updatedAt, &preview); err != nil {
			s.respondError(w, r, err)
			return
		}
		if status != "resolved" && status != "closed" {
			open++
		}
		if lastSeen == nil || updatedAt.After(*lastSeen) {
			value := updatedAt
			lastSeen = &value
		}
		conversations = append(conversations, map[string]any{
			"id": conversationID, "status": status, "channel": channel, "createdAt": createdAt,
			"updatedAt": updatedAt, "preview": truncateText(strings.TrimSpace(preview), 180),
		})
	}
	if err := rows.Err(); err != nil {
		s.respondError(w, r, err)
		return
	}
	name := ""
	if customer.Name != nil {
		name = *customer.Name
	}
	email := ""
	if customer.Email != nil {
		email = *customer.Email
	}
	row := consoleCustomerRow{
		ID: customer.ID, Name: name, Email: email, Initial: consoleInitial(name), Attributes: consoleAttributes(customer.Attributes),
		CreatedAt: customer.CreatedAt, LastSeen: lastSeen, Total: len(conversations), Open: open,
	}
	if row.Name == "" {
		row.Name = email
	}
	writeData(w, http.StatusOK, map[string]any{
		"customer": row, "conversations": conversations, "openCount": open, "totalCount": len(conversations),
	})
}

func (s *Server) handleConsoleApprovals(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	toPayload := func(row db.ApprovalRow) map[string]any {
		name, email := "Visitor", ""
		if row.CustomerName != nil {
			name = *row.CustomerName
		}
		if row.CustomerEmail != nil {
			email = *row.CustomerEmail
		}
		var args any
		if err := json.Unmarshal(row.Args, &args); err != nil {
			args = string(row.Args)
		}
		item := map[string]any{
			"id": row.ID, "conversationId": row.ConversationID, "toolId": row.ToolID,
			"args": args, "status": row.Status, "createdAt": row.CreatedAt,
			"customer": map[string]any{"name": name, "email": email, "initial": consoleInitial(name)},
		}
		if row.DecidedAt != nil {
			item["decidedAt"] = row.DecidedAt
		}
		return item
	}
	pending, err := s.repo.ListApprovals(db.WithOrg(r.Context(), sess.OrgID), "pending_approval", 100)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	history, err := s.repo.ListApprovals(db.WithOrg(r.Context(), sess.OrgID), "decided", 28)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	pendingItems := make([]map[string]any, 0, len(pending))
	for _, row := range pending {
		pendingItems = append(pendingItems, toPayload(row))
	}
	historyItems := make([]map[string]any, 0, len(history))
	for _, row := range history {
		historyItems = append(historyItems, toPayload(row))
	}
	writeData(w, http.StatusOK, map[string]any{"pending": pendingItems, "history": historyItems})
}

type consoleApprovalDecisionBody struct {
	IDs      []string `json:"ids"`
	Decision string   `json:"decision"`
}

func (s *Server) handleConsoleApprovalsBulk(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	body, err := decodeBody[consoleApprovalDecisionBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if len(body.IDs) == 0 || len(body.IDs) > 100 {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Choose between 1 and 100 approvals.")
		return
	}
	if body.Decision != "approve" && body.Decision != "reject" {
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Decision must be approve or reject.")
		return
	}
	done := []string{}
	failed := []map[string]string{}
	for _, id := range body.IDs {
		if !uuidShape.MatchString(id) {
			failed = append(failed, map[string]string{"id": id, "message": "Invalid approval id."})
			continue
		}
		if _, err := s.decideAction(r.Context(), sess.OrgID, sess.UserID, id, body.Decision); err != nil {
			failed = append(failed, map[string]string{"id": id, "message": err.Error()})
			continue
		}
		done = append(done, id)
	}
	writeData(w, http.StatusOK, map[string]any{"done": done, "failed": failed})
}

func (s *Server) handleConsoleKnowledge(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapKnowledgeRead)
	if !ok {
		return
	}
	rows, err := s.repo.ListKnowledgeSources(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	ready, processing, failed, chunks := 0, 0, 0, 0
	vectorSearch := false
	for _, row := range rows {
		chunks += row.Chunks
		switch row.Status {
		case "ready":
			ready++
		case "pending", "fetching", "indexing":
			processing++
		case "failed":
			failed++
		}
		if row.Vector {
			vectorSearch = true
		}
	}
	writeData(w, http.StatusOK, map[string]any{
		"sources": rows,
		"counts": map[string]any{
			"total": len(rows), "ready": ready, "processing": processing, "failed": failed,
			"chunks": chunks, "vectorSearch": vectorSearch,
		},
		"canWrite": auth.Can(sess.Role, auth.CapKnowledgeWrite),
	})
}

type consoleScopeBody struct {
	Scope string `json:"scope"`
}

func (s *Server) handleConsoleKnowledgeUpdate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		s.respondError(w, r, apperr.NotFound("source not found"))
		return
	}
	body, err := decodeBody[consoleScopeBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	scope := normalizeScope(body.Scope)
	found, err := s.repo.UpdateKnowledgeSourceScope(db.WithOrg(r.Context(), sess.OrgID), id, scope)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("source not found"))
		return
	}
	s.auditSourceUpdated(r, sess, id, map[string]any{"scope": scope})
	writeData(w, http.StatusOK, map[string]any{"updated": true})
}

func (s *Server) handleConsoleKnowledgeReindex(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		s.respondError(w, r, apperr.NotFound("source not found"))
		return
	}
	source, err := s.repo.GetKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), id)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.enqueueReindex(r.Context(), sess.OrgID, id, source); err != nil {
		s.respondError(w, r, err)
		return
	}
	if err := s.repo.SetKnowledgeSourcePending(db.WithOrg(r.Context(), sess.OrgID), id); err != nil {
		s.respondError(w, r, err)
		return
	}
	s.auditSourceUpdated(r, sess, id, map[string]any{"reindex": true})
	writeData(w, http.StatusOK, map[string]any{"reindexing": true})
}

func (s *Server) handleConsoleKnowledgeDelete(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapKnowledgeWrite)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidShape.MatchString(id) {
		s.respondError(w, r, apperr.NotFound("source not found"))
		return
	}
	found, err := s.repo.DeleteKnowledgeSource(db.WithOrg(r.Context(), sess.OrgID), id)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	if !found {
		s.respondError(w, r, apperr.NotFound("source not found"))
		return
	}
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "knowledge.source.deleted",
		TargetType: "knowledge_source", TargetID: &id, Payload: map[string]any{},
	})
	writeData(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) handleConsoleAnalytics(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	days := analyticsDays(r)
	metrics, trendRows, err := s.analyticsPageData(r, sess.OrgID, days)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	trend := make([]map[string]any, 0, len(trendRows))
	for _, point := range trendRows {
		trend = append(trend, map[string]any{"date": point["date"], "total": point["total"]})
	}
	statusCounts, err := s.repo.ConversationStatusCounts(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	byStatus := statusCountMap(statusCounts)
	total := 0
	openNow := 0
	for status, count := range byStatus {
		total += count
		if status != "resolved" && status != "closed" {
			openNow += count
		}
	}
	labels := map[string]string{"ai_active": "AI active", "awaiting_human": "Awaiting human", "human_active": "With human", "resolved": "Resolved", "closed": "Closed"}
	mix := []map[string]any{}
	for _, status := range []string{"ai_active", "awaiting_human", "human_active", "resolved", "closed"} {
		count := byStatus[status]
		percent := 0
		if total > 0 {
			percent = count * 100 / total
		}
		mix = append(mix, map[string]any{"status": status, "label": labels[status], "count": count, "percent": percent})
	}
	latestDate := any(nil)
	if len(trend) > 0 {
		latestDate = trend[len(trend)-1]["date"]
	}
	metricsPayload := any(nil)
	if metrics != nil {
		metricsPayload = metrics
	}
	writeData(w, http.StatusOK, map[string]any{
		"days": days, "metrics": metricsPayload, "trend": trend, "statusMix": mix,
		"openNow": openNow, "latestDate": latestDate,
	})
}

func (s *Server) handleConsoleAudit(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapAuditRead)
	if !ok {
		return
	}
	typeFilter := r.URL.Query().Get("type")
	actor := r.URL.Query().Get("actor")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	before := int64(0)
	if raw := r.URL.Query().Get("before"); raw != "" {
		before, _ = strconv.ParseInt(raw, 10, 64)
	}
	rows, err := s.repo.ListAuditEventsFiltered(db.WithOrg(r.Context(), sess.OrgID), typeFilter, actor, query, before, 51)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	hasMore := len(rows) > 50
	if hasMore {
		rows = rows[:50]
	}
	types, err := s.repo.AuditTypes(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	memberRows, err := s.repo.ListMembers(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	names := map[string]string{}
	members := make([]map[string]any, 0, len(memberRows))
	for _, member := range memberRows {
		names[member.UserID] = member.Name
		members = append(members, consoleMemberPayload(member, sess.UserID))
	}
	events := make([]map[string]any, 0, len(rows))
	nextBefore := before
	for _, row := range rows {
		actorName := "Desk AI"
		if row.ActorType == "system" {
			actorName = "System"
		}
		if row.ActorID != nil {
			if name, ok := names[*row.ActorID]; ok {
				actorName = name
			} else {
				actorName = *row.ActorID
			}
		}
		var payload any
		if len(row.Payload) > 0 {
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				payload = string(row.Payload)
			}
		}
		events = append(events, map[string]any{
			"id": row.ID, "type": row.Type, "actorType": row.ActorType, "actorId": row.ActorID,
			"actorName": actorName, "targetType": row.TargetType, "targetId": row.TargetID,
			"payload": payload, "createdAt": row.CreatedAt,
		})
		if nextBefore == 0 || row.ID < nextBefore {
			nextBefore = row.ID
		}
	}
	writeData(w, http.StatusOK, map[string]any{
		"events": events, "types": types, "hasMore": hasMore, "nextBefore": nextBefore, "members": members,
	})
}

func (s *Server) handleConsoleSearch(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		writeData(w, http.StatusOK, map[string]any{"conversations": []any{}, "customers": []any{}, "sources": []any{}})
		return
	}
	rows, err := s.queryInboxList(r.Context(), sess.OrgID, "", query)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	conversations := []consoleConversation{}
	for index, row := range rows {
		if index >= 6 {
			break
		}
		conversations = append(conversations, consoleConversationFromRow(row))
	}
	customerRows, err := s.repo.ListCustomers(db.WithOrg(r.Context(), sess.OrgID), query)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	customers := []map[string]any{}
	for index, row := range customerRows {
		if index >= 5 {
			break
		}
		name, email := "", ""
		if row.Name != nil {
			name = *row.Name
		}
		if row.Email != nil {
			email = *row.Email
		}
		if name == "" {
			name = email
		}
		customers = append(customers, map[string]any{
			"id": row.ID, "name": name, "email": email, "open": row.Open, "total": row.Total,
			"initial": consoleInitial(name), "lastSeen": row.LastSeen,
		})
	}
	sources := []map[string]any{}
	if auth.Can(sess.Role, auth.CapKnowledgeRead) {
		sourceRows, err := s.repo.ListKnowledgeSources(db.WithOrg(r.Context(), sess.OrgID))
		if err != nil {
			s.respondError(w, r, err)
			return
		}
		lower := strings.ToLower(query)
		for _, row := range sourceRows {
			if len(sources) >= 5 {
				break
			}
			if strings.Contains(strings.ToLower(row.Name), lower) {
				sources = append(sources, map[string]any{
					"id": row.ID, "name": row.Name, "status": row.Status, "type": row.Type, "scope": row.Scope,
				})
			}
		}
	}
	writeData(w, http.StatusOK, map[string]any{
		"conversations": conversations, "customers": customers, "sources": sources,
	})
}

func (s *Server) handleConsoleInvitation(w http.ResponseWriter, r *http.Request) {
	invitation, orgName, err := s.repo.PendingInvitation(r.Context(), r.PathValue("id"))
	if err != nil {
		s.respondError(w, r, apperr.NotFound("This invitation has expired, was revoked, or was already used."))
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"email": invitation.Email, "role": invitation.Role, "orgName": orgName, "expiresAt": invitation.ExpiresAt,
	})
}

type consoleInviteAcceptBody struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (s *Server) handleConsoleInvitationAccept(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBody[consoleInviteAcceptBody](r)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	switch {
	case body.Name == "" || len(body.Name) > 100:
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Your name must be between 1 and 100 characters.")
		return
	case len(body.Password) < 8 || len(body.Password) > 200:
		writeErrorBody(w, http.StatusBadRequest, "bad_request", "Password must be between 8 and 200 characters.")
		return
	}
	if _, err := ratelimit.Enforce(r.Context(), s.pool, "invite:"+clientIP(r), 20, 3600); err != nil {
		s.respondError(w, r, err)
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	userID, orgID, err := s.repo.AcceptInvitation(r.Context(), r.PathValue("id"), body.Name, hash)
	if err != nil {
		if errors.Is(err, db.ErrEmailExists) {
			writeErrorBody(w, http.StatusConflict, "conflict", "That email already has an account. Sign in instead.")
			return
		}
		if errors.Is(err, db.ErrNotFound) {
			s.respondError(w, r, apperr.NotFound("This invitation has expired, was revoked, or was already used."))
			return
		}
		s.respondError(w, r, err)
		return
	}
	if err := s.startSession(w, r, userID); err != nil {
		s.respondError(w, r, err)
		return
	}
	s.auditAuthEvent(r, userID, "auth.login", "session")
	_ = s.repo.WriteAuditNoOrg(r.Context(), &orgID, db.AuditInput{
		ActorType: db.ActorUser, ActorID: &userID, Type: "user.accepted_invite",
		TargetType: "user", TargetID: &userID, Payload: map[string]any{},
	})
	sess := &auth.Session{UserID: userID, Name: body.Name, OrgID: orgID}
	if membership, role, membershipErr := s.repo.MembershipForUser(r.Context(), userID); membershipErr == nil {
		sess.OrgID = membership
		sess.Role = auth.Role(role)
	}
	_ = s.pool.QueryRow(r.Context(), `select email from "user" where id = $1`, userID).Scan(&sess.Email)
	payload, err := s.consoleSessionPayload(r.Context(), sess)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusCreated, payload)
}

func slaBreached(firstResponseAt *time.Time, firstResponseDueAt, resolutionDueAt *time.Time, status string) bool {
	if status == "resolved" || status == "closed" {
		return false
	}
	now := time.Now()
	if firstResponseAt == nil && firstResponseDueAt != nil && firstResponseDueAt.Before(now) {
		return true
	}
	return resolutionDueAt != nil && resolutionDueAt.Before(now)
}
