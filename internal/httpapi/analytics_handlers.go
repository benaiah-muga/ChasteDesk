// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"chastedesk/internal/analytics"
	"chastedesk/internal/auth"
	"chastedesk/internal/queue"
)

func (s *Server) handleAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	days := 14
	if raw := r.URL.Query().Get("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			days = parsed
		}
	}
	if days < 1 {
		days = 1
	}
	if days > 90 {
		days = 90
	}
	since := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := s.pool.Query(r.Context(),
		`select date, metrics from analytics_daily
		 where org_id = $1 and date >= $2 order by date asc`, sess.OrgID, since)
	if err != nil {
		s.respondError(w, r, err)
		return
	}
	defer rows.Close()
	trend := []map[string]any{}
	var latest json.RawMessage
	for rows.Next() {
		var date string
		var metrics json.RawMessage
		if err := rows.Scan(&date, &metrics); err != nil {
			s.respondError(w, r, err)
			return
		}
		trend = append(trend, map[string]any{"date": date, "metrics": metrics})
		latest = metrics
	}
	if err := rows.Err(); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"metrics": latest, "trend": trend})
}

func (s *Server) handleAnalyticsRollup(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.requireCapability(w, r, auth.CapInboxUse)
	if !ok {
		return
	}
	if r.Body != nil {
		var body struct {
			OrgID string `json:"orgId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if _, err := s.queue.Send(r.Context(), "analytics-rollup", map[string]any{"orgId": sess.OrgID},
		queue.SendOptions{SingletonKey: "rollup:" + sess.OrgID}); err != nil {
		s.respondError(w, r, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"queued": true})
}

func rateLabel(rate float64) string {
	return strconv.Itoa(int(math.Round(rate*100))) + "%"
}

func secondsLabel(seconds *int64) string {
	if seconds == nil {
		return "-"
	}
	return strconv.FormatInt(*seconds, 10) + "s"
}

func (s *Server) analyticsPageData(r *http.Request, orgID string, days int) (*analytics.Metrics, []map[string]any, error) {
	if days < 1 {
		days = 14
	}
	var raw json.RawMessage
	err := s.pool.QueryRow(r.Context(),
		`select metrics from analytics_daily where org_id = $1 order by date desc limit 1`, orgID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var metrics analytics.Metrics
	if err := json.Unmarshal(raw, &metrics); err != nil {
		return nil, nil, err
	}
	since := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := s.pool.Query(r.Context(),
		`select date, metrics from analytics_daily
		 where org_id = $1 and date >= $2 order by date asc`, orgID, since)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	trend := []map[string]any{}
	for rows.Next() {
		var date string
		var dayMetrics json.RawMessage
		if err := rows.Scan(&date, &dayMetrics); err != nil {
			return nil, nil, err
		}
		var day analytics.Metrics
		if err := json.Unmarshal(dayMetrics, &day); err != nil {
			return nil, nil, err
		}
		trend = append(trend, map[string]any{"date": date, "total": day.TotalConversations})
	}
	return &metrics, trend, rows.Err()
}
