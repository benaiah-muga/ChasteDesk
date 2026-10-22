// SPDX-License-Identifier: AGPL-3.0-only
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type httpKey struct {
	Method string
	Route  string
	Status int
}

type httpStat struct {
	Count        uint64
	DurationSecs float64
}

type Metrics struct {
	mu       sync.Mutex
	http     map[httpKey]*httpStat
	started  time.Time
	inflight atomic.Int64
	alertsOK atomic.Uint64
	alertsKO atomic.Uint64
}

func NewMetrics() *Metrics {
	return &Metrics{http: map[httpKey]*httpStat{}, started: time.Now()}
}

func (m *Metrics) ObserveHTTP(method, route string, status int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := httpKey{Method: method, Route: route, Status: status}
	stat := m.http[key]
	if stat == nil {
		stat = &httpStat{}
		m.http[key] = stat
	}
	stat.Count++
	stat.DurationSecs += duration.Seconds()
}

func (m *Metrics) IncAlert(sent bool) {
	if sent {
		m.alertsOK.Add(1)
	} else {
		m.alertsKO.Add(1)
	}
}

func (m *Metrics) Render(ctx context.Context, pool *pgxpool.Pool) string {
	m.mu.Lock()
	keys := make([]httpKey, 0, len(m.http))
	for key := range m.http {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Route != keys[j].Route {
			return keys[i].Route < keys[j].Route
		}
		if keys[i].Method != keys[j].Method {
			return keys[i].Method < keys[j].Method
		}
		return keys[i].Status < keys[j].Status
	})
	var builder strings.Builder
	builder.WriteString("# HELP chastedesk_http_requests_total HTTP requests handled by method, route, and status.\n")
	builder.WriteString("# TYPE chastedesk_http_requests_total counter\n")
	for _, key := range keys {
		stat := m.http[key]
		fmt.Fprintf(&builder, "chastedesk_http_requests_total{method=%q,route=%q,status=%q} %d\n", key.Method, key.Route, strconv.Itoa(key.Status), stat.Count)
	}
	builder.WriteString("# HELP chastedesk_http_request_duration_seconds Request duration by method, route, and status.\n")
	builder.WriteString("# TYPE chastedesk_http_request_duration_seconds summary\n")
	for _, key := range keys {
		stat := m.http[key]
		fmt.Fprintf(&builder, "chastedesk_http_request_duration_seconds_sum{method=%q,route=%q,status=%q} %f\n", key.Method, key.Route, strconv.Itoa(key.Status), stat.DurationSecs)
		fmt.Fprintf(&builder, "chastedesk_http_request_duration_seconds_count{method=%q,route=%q,status=%q} %d\n", key.Method, key.Route, strconv.Itoa(key.Status), stat.Count)
	}
	m.mu.Unlock()

	builder.WriteString("# HELP chastedesk_http_in_flight HTTP requests currently being handled.\n")
	builder.WriteString("# TYPE chastedesk_http_in_flight gauge\n")
	fmt.Fprintf(&builder, "chastedesk_http_in_flight %d\n", m.inflight.Load())
	builder.WriteString("# HELP chastedesk_process_start_time_seconds Unix time when this web process started.\n")
	builder.WriteString("# TYPE chastedesk_process_start_time_seconds gauge\n")
	fmt.Fprintf(&builder, "chastedesk_process_start_time_seconds %d\n", m.started.Unix())
	builder.WriteString("# HELP chastedesk_alerts_total Operator alert delivery attempts.\n")
	builder.WriteString("# TYPE chastedesk_alerts_total counter\n")
	fmt.Fprintf(&builder, "chastedesk_alerts_total{result=\"sent\"} %d\n", m.alertsOK.Load())
	fmt.Fprintf(&builder, "chastedesk_alerts_total{result=\"failed\"} %d\n", m.alertsKO.Load())

	if pool == nil {
		return builder.String()
	}
	stat := pool.Stat()
	builder.WriteString("# HELP chastedesk_db_connections Database pool connections by state.\n")
	builder.WriteString("# TYPE chastedesk_db_connections gauge\n")
	for _, item := range []struct {
		state string
		value int32
	}{
		{"acquired", stat.AcquiredConns()},
		{"idle", stat.IdleConns()},
		{"total", stat.TotalConns()},
		{"max", stat.MaxConns()},
	} {
		fmt.Fprintf(&builder, "chastedesk_db_connections{state=%q} %d\n", item.state, item.value)
	}

	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if rows, err := pool.Query(queryCtx, `select name, status, count(*)::bigint from jobs group by name, status order by name, status`); err == nil {
		builder.WriteString("# HELP chastedesk_jobs Background jobs by queue and status.\n")
		builder.WriteString("# TYPE chastedesk_jobs gauge\n")
		for rows.Next() {
			var name, status string
			var count int64
			if err := rows.Scan(&name, &status, &count); err == nil {
				fmt.Fprintf(&builder, "chastedesk_jobs{queue=%q,status=%q} %d\n", name, status, count)
			}
		}
		rows.Close()
	}
	if rows, err := pool.Query(queryCtx, `select status, count(*)::bigint from email_outbox group by status order by status`); err == nil {
		builder.WriteString("# HELP chastedesk_email_outbox Email deliveries by status.\n")
		builder.WriteString("# TYPE chastedesk_email_outbox gauge\n")
		for rows.Next() {
			var status string
			var count int64
			if err := rows.Scan(&status, &count); err == nil {
				fmt.Fprintf(&builder, "chastedesk_email_outbox{status=%q} %d\n", status, count)
			}
		}
		rows.Close()
	}
	if rows, err := pool.Query(queryCtx, `select coalesce(outcome, 'running'), count(*)::bigint from ai_runs group by outcome order by 1`); err == nil {
		builder.WriteString("# HELP chastedesk_ai_runs AI runs by outcome.\n")
		builder.WriteString("# TYPE chastedesk_ai_runs gauge\n")
		for rows.Next() {
			var outcome string
			var count int64
			if err := rows.Scan(&outcome, &count); err == nil {
				fmt.Fprintf(&builder, "chastedesk_ai_runs{outcome=%q} %d\n", outcome, count)
			}
		}
		rows.Close()
	}
	return builder.String()
}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{8,128}$`)

type requestIDKey struct{}

func NewRequestID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(raw)
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.status = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type Middleware struct {
	Metrics  *Metrics
	Notifier *Notifier
	Logger   *slog.Logger
}

func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if !requestIDPattern.MatchString(requestID) {
			requestID = NewRequestID()
		}
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID))
		w.Header().Set("X-Request-ID", requestID)
		if m.Metrics != nil {
			m.Metrics.inflight.Add(1)
			defer m.Metrics.inflight.Add(-1)
		}
		start := time.Now()
		writer := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(writer, r)
		duration := time.Since(start)
		route := strings.TrimPrefix(r.Pattern, r.Method+" ")
		if route == "" {
			route = "unmatched"
		}
		if m.Metrics != nil {
			m.Metrics.ObserveHTTP(r.Method, route, writer.status, duration)
		}
		if m.Logger != nil {
			attrs := []any{"requestId", requestID, "method", r.Method, "route", route, "status", writer.status, "durationMs", duration.Milliseconds()}
			if writer.status >= 500 {
				m.Logger.Error("request failed", attrs...)
			} else {
				m.Logger.Debug("request", attrs...)
			}
		}
		if m.Notifier != nil && writer.status >= 500 {
			m.Notifier.NotifyAsync(r.Context(), "http.5xx", "HTTP request failed", map[string]any{
				"requestId": requestID, "method": r.Method, "route": route, "status": writer.status, "durationMs": duration.Milliseconds(),
			})
		}
	})
}
