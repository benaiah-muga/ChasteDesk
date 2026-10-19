// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/queue"
	"chastedesk/internal/web/views"
)

func analyticsDays(r *http.Request) int {
	days := 14
	if raw := r.URL.Query().Get("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			switch parsed {
			case 7, 14, 30:
				days = parsed
			}
		}
	}
	return days
}

func (s *Server) handleAnalyticsPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	shell, err := s.shellFor(r, sess, "analytics", "Analytics", "")
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	days := analyticsDays(r)
	metrics, trendRows, err := s.analyticsPageData(r, sess.OrgID, days)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	trend := make([]views.AnalyticsTrendPoint, 0, len(trendRows))
	for _, point := range trendRows {
		date, _ := point["date"].(string)
		total, _ := point["total"].(int)
		trend = append(trend, views.AnalyticsTrendPoint{Date: date, Total: total})
	}
	view := views.AnalyticsView{
		Days: days, Trend: trend, Chart: views.BuildTrendChart(trend),
		CanRollup: auth.Can(sess.Role, auth.CapInboxUse),
	}
	if metrics != nil {
		view.Cards = &views.AnalyticsCardView{
			Total:        strconv.Itoa(metrics.TotalConversations),
			AIResolution: rateLabel(metrics.AIResolutionRate),
			Escalation:   rateLabel(metrics.EscalationRate),
			MedianFirst:  secondsLabel(metrics.P50FirstResponseSeconds),
			MedianRes:    secondsLabel(metrics.P50ResolutionSeconds),
		}
	}
	if len(trend) > 0 {
		view.LatestDate = views.ShortDate(trend[len(trend)-1].Date)
	} else {
		view.LatestDate = "-"
	}
	counts, err := s.repo.ConversationStatusCounts(db.WithOrg(r.Context(), sess.OrgID))
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	byStatus := map[string]int{}
	total := 0
	for _, item := range counts {
		byStatus[item.Status] = item.Count
		total += item.Count
	}
	makeDist := func(status, label, tone string) views.StatusDistView {
		count := byStatus[status]
		percent := 0
		if total > 0 && count > 0 {
			percent = count * 100 / total
			if percent < 3 {
				percent = 3
			}
		}
		return views.StatusDistView{Label: label, Count: count, Percent: percent, Tone: tone}
	}
	view.StatusDist = []views.StatusDistView{
		makeDist("ai_active", "AI active", "accent"),
		makeDist("awaiting_human", "Awaiting human", "warn"),
		makeDist("human_active", "With human", "info"),
		makeDist("resolved", "Resolved", "success"),
		makeDist("closed", "Closed", "info"),
	}
	for _, item := range view.StatusDist {
		if item.Label == "Awaiting human" || item.Label == "With human" || item.Label == "AI active" {
			view.TotalOpen += item.Count
		}
	}
	renderPage(w, views.AnalyticsPage(shell, view))
}

func (s *Server) handleAnalyticsRollupPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	days := 14
	if parsed, err := strconv.Atoi(r.FormValue("days")); err == nil {
		switch parsed {
		case 7, 14, 30:
			days = parsed
		}
	}
	if _, err := s.queue.Send(r.Context(), "analytics-rollup", map[string]any{"orgId": sess.OrgID},
		queue.SendOptions{SingletonKey: "rollup:" + sess.OrgID}); err != nil {
		s.respondError(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard/analytics?days=%d&msg=Rollup+queued.+Refresh+in+a+moment.", days), http.StatusSeeOther)
}

func (s *Server) handleAuditPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapAuditRead)
	if !ok {
		return
	}
	shell, err := s.shellFor(r, sess, "audit", "Audit log", "")
	if err != nil {
		s.respondError(w, r, err)
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
	view := views.AuditView{
		Types: types, TypeFilter: typeFilter, Actor: actor, Query: query,
		HasMore: hasMore, Showing: len(rows),
	}
	for _, row := range rows {
		event := views.AuditEventView{
			ID: row.ID, Type: row.Type, ActorType: row.ActorType, Target: row.TargetType,
			CreatedAt: views.RelativeTime(row.CreatedAt), Icon: auditIcon(row.Type),
		}
		if row.ActorID != nil {
			event.ActorID = *row.ActorID
		}
		if row.TargetID != nil {
			event.Target = row.TargetType + " " + *row.TargetID
		}
		if len(row.Payload) > 0 {
			event.Payload = prettyJSON(row.Payload)
		}
		view.Events = append(view.Events, event)
		if row.ID < view.NextBefore || view.NextBefore == 0 {
			view.NextBefore = row.ID
		}
	}
	renderPage(w, views.AuditPage(shell, view))
}

func auditIcon(eventType string) string {
	switch {
	case strings.HasPrefix(eventType, "auth"):
		return "user"
	case strings.HasPrefix(eventType, "settings"), strings.HasPrefix(eventType, "agent"):
		return "settings"
	case strings.HasPrefix(eventType, "knowledge"):
		return "book"
	case strings.HasPrefix(eventType, "run"):
		return "bot"
	case strings.HasPrefix(eventType, "action"):
		return "approvals"
	case strings.HasPrefix(eventType, "conversation"), strings.HasPrefix(eventType, "message"):
		return "message"
	}
	return "scroll"
}

func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.pageSession(w, r, auth.CapAuditRead)
	if !ok {
		return
	}
	rows, err := s.repo.ListAuditEventsFiltered(db.WithOrg(r.Context(), sess.OrgID),
		r.URL.Query().Get("type"), r.URL.Query().Get("actor"), strings.TrimSpace(r.URL.Query().Get("q")), 0, 5000)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="chastedesk-audit.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"id", "created_at", "type", "actor_type", "actor_id", "target_type", "target_id", "payload"})
	for _, row := range rows {
		actorID, targetID := "", ""
		if row.ActorID != nil {
			actorID = *row.ActorID
		}
		if row.TargetID != nil {
			targetID = *row.TargetID
		}
		payload := ""
		if len(row.Payload) > 0 {
			payload = string(row.Payload)
		}
		_ = writer.Write([]string{
			strconv.FormatInt(row.ID, 10), row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			row.Type, row.ActorType, actorID, row.TargetType, targetID, payload,
		})
	}
	writer.Flush()
	_ = s.repo.WriteAudit(db.WithOrg(r.Context(), sess.OrgID), db.AuditInput{
		ActorType: db.ActorUser, ActorID: &sess.UserID, Type: "audit.exported",
		TargetType: "audit_log", Payload: map[string]any{"rows": len(rows)},
	})
}
