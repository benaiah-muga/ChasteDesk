// SPDX-License-Identifier: AGPL-3.0-only
package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"chastedesk/internal/queue"
)

type Metrics struct {
	TotalConversations      int     `json:"total_conversations"`
	AIResolutionRate        float64 `json:"ai_resolution_rate"`
	EscalationRate          float64 `json:"escalation_rate"`
	P50FirstResponseSeconds *int64  `json:"p50_first_response_seconds"`
	P50ResolutionSeconds    *int64  `json:"p50_resolution_seconds"`
}

func Rollup(ctx context.Context, pool *pgxpool.Pool, orgID, date string) (Metrics, error) {
	var metrics Metrics

	if err := pool.QueryRow(ctx,
		`select count(*)::int from conversations where org_id = $1 and to_char(created_at, 'YYYY-MM-DD') = $2`,
		orgID, date).Scan(&metrics.TotalConversations); err != nil {
		return metrics, err
	}

	var totalResolved, aiResolved int
	if err := pool.QueryRow(ctx,
		`select count(*)::int as total_resolved,
		        coalesce(sum(case when human_count = 0 then 1 else 0 end), 0)::int as ai_resolved
		 from (
		   select c.id,
		     (select count(*)::int from messages m where m.conversation_id = c.id and m.role in ('agent_human')) as human_count
		   from conversations c
		   where c.org_id = $1 and c.status = 'resolved' and to_char(c.resolved_at, 'YYYY-MM-DD') = $2
		 ) t`, orgID, date).Scan(&totalResolved, &aiResolved); err != nil {
		return metrics, err
	}
	if totalResolved > 0 {
		metrics.AIResolutionRate = float64(aiResolved) / float64(totalResolved)
	}

	var escalations int
	if err := pool.QueryRow(ctx,
		`select count(*)::int from conversations
		 where org_id = $1 and status in ('awaiting_human', 'human_active', 'resolved', 'closed')
		   and handoff is not null and to_char(updated_at, 'YYYY-MM-DD') = $2`, orgID, date).Scan(&escalations); err != nil {
		return metrics, err
	}
	if metrics.TotalConversations > 0 {
		metrics.EscalationRate = float64(escalations) / float64(metrics.TotalConversations)
	}

	firstResponse, err := p50(ctx, pool,
		`select distinct on (c.id) extract(epoch from (m.created_at - c.created_at))::int as seconds
		 from conversations c
		 join messages m on m.conversation_id = c.id and m.role in ('agent_ai', 'agent_human')
		 where c.org_id = $1 and to_char(c.created_at, 'YYYY-MM-DD') = $2
		 order by c.id, m.created_at asc`, orgID, date)
	if err != nil {
		return metrics, err
	}
	metrics.P50FirstResponseSeconds = firstResponse

	resolution, err := p50(ctx, pool,
		`select extract(epoch from (c.resolved_at - c.created_at))::int as seconds
		 from conversations c
		 where c.org_id = $1 and c.resolved_at is not null and to_char(c.created_at, 'YYYY-MM-DD') = $2`, orgID, date)
	if err != nil {
		return metrics, err
	}
	metrics.P50ResolutionSeconds = resolution

	encoded, err := json.Marshal(metrics)
	if err != nil {
		return metrics, err
	}
	if _, err := pool.Exec(ctx,
		`insert into analytics_daily (org_id, date, metrics, computed_at)
		 values ($1, $2, $3::jsonb, now())
		 on conflict (org_id, date) do update set metrics = $3::jsonb, computed_at = now()`,
		orgID, date, string(encoded)); err != nil {
		return metrics, err
	}
	return metrics, nil
}

func p50(ctx context.Context, pool *pgxpool.Pool, query, orgID, date string) (*int64, error) {
	rows, err := pool.Query(ctx, query, orgID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []int64{}
	for rows.Next() {
		var seconds int64
		if err := rows.Scan(&seconds); err != nil {
			return nil, err
		}
		values = append(values, seconds)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return Median(values), nil
}

func Median(values []int64) *int64 {
	if len(values) == 0 {
		return nil
	}
	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		value := sorted[mid]
		return &value
	}
	value := int64(math.Round(float64(sorted[mid]+sorted[mid-1]) / 2))
	return &value
}

func RollupDay(ctx context.Context, pool *pgxpool.Pool, orgID string) error {
	var today, yesterday string
	if err := pool.QueryRow(ctx,
		`select to_char(now() at time zone 'utc', 'YYYY-MM-DD'),
		        to_char((now() at time zone 'utc') - interval '1 day', 'YYYY-MM-DD')`).Scan(&today, &yesterday); err != nil {
		return err
	}
	if _, err := Rollup(ctx, pool, orgID, today); err != nil {
		return err
	}
	_, err := Rollup(ctx, pool, orgID, yesterday)
	return err
}

func Handler(pool *pgxpool.Pool) queue.Handler {
	return func(ctx context.Context, job queue.Job) error {
		var data struct {
			OrgID string `json:"orgId"`
		}
		if err := json.Unmarshal(job.Data, &data); err != nil {
			return err
		}
		if data.OrgID == "" {
			return fmt.Errorf("analytics rollup: missing orgId")
		}
		return RollupDay(ctx, pool, data.OrgID)
	}
}
