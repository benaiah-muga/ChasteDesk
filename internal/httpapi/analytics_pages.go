// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"net/http"
	"strconv"

	"chastedesk/internal/auth"
	"chastedesk/internal/db"
	"chastedesk/internal/web/views"
)

func (s *Server) handleAnalyticsPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	metrics, trendRows, err := s.analyticsPageData(r, sess.OrgID)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	trend := make([]views.AnalyticsTrendPoint, 0, len(trendRows))
	for _, point := range trendRows {
		trend = append(trend, views.AnalyticsTrendPoint{
			Date:  point["date"].(string),
			Total: point["total"].(int),
		})
	}
	if metrics == nil {
		renderPage(w, views.AnalyticsPage(nil, trend))
		return
	}
	renderPage(w, views.AnalyticsPage(&views.AnalyticsCardView{
		Total:        strconv.Itoa(metrics.TotalConversations),
		AIResolution: rateLabel(metrics.AIResolutionRate),
		Escalation:   rateLabel(metrics.EscalationRate),
		MedianFirst:  secondsLabel(metrics.P50FirstResponseSeconds),
		MedianRes:    secondsLabel(metrics.P50ResolutionSeconds),
	}, trend))
}

func (s *Server) handleAuditPage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapAuditRead)
	if !ok {
		return
	}
	typeFilter := r.URL.Query().Get("type")
	q := r.URL.Query().Get("q")
	rows, err := s.repo.ListAuditEvents(db.WithOrg(r.Context(), sess.OrgID), typeFilter, q, 100)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	events := make([]views.AuditEventView, 0, len(rows))
	for _, row := range rows {
		event := views.AuditEventView{
			ID:        row.ID,
			Type:      row.Type,
			ActorType: row.ActorType,
			Target:    row.TargetType,
			CreatedAt: row.CreatedAt.Format("2006-01-02 15:04"),
		}
		if row.ActorID != nil {
			event.ActorID = *row.ActorID
		}
		if row.TargetID != nil {
			event.Target = row.TargetType + " " + *row.TargetID
		}
		if len(row.Payload) > 0 {
			event.Payload = string(row.Payload)
		}
		events = append(events, event)
	}
	renderPage(w, views.AuditPage(events, typeFilter, q))
}
