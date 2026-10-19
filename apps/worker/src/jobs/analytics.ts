// SPDX-License-Identifier: AGPL-3.0-only
import type pg from "pg";
import { JOB_NAMES } from "@chastedesk/shared";

export interface AnalyticsMetrics {
  total_conversations: number;
  ai_resolution_rate: number;
  escalation_rate: number;
  p50_first_response_seconds: number | null;
  p50_resolution_seconds: number | null;
}

export async function rollupAnalytics(pool: pg.Pool, orgId: string, date: string): Promise<AnalyticsMetrics> {
  const created = await pool.query(
    `select count(*)::int as c from conversations
     where org_id = $1 and to_char(created_at, 'YYYY-MM-DD') = $2`,
    [orgId, date],
  );
  const total = created.rows[0].c;

  const resolved = await pool.query(
    `select
       count(*)::int as total_resolved,
       sum(case when human_count = 0 then 1 else 0 end)::int as ai_resolved
     from (
       select c.id,
         (select count(*)::int from messages m where m.conversation_id = c.id and m.role in ('agent_human')) as human_count
       from conversations c
       where c.org_id = $1 and c.status = 'resolved' and to_char(c.resolved_at, 'YYYY-MM-DD') = $2
     ) t`,
    [orgId, date],
  );
  const first = resolved.rows[0];
  const totalResolved = Number(first?.total_resolved ?? 0);
  const aiResolved = Number(first?.ai_resolved ?? 0);

  const escalated = await pool.query(
    `select count(*)::int as c from conversations
     where org_id = $1 and status in ('awaiting_human', 'human_active', 'resolved', 'closed')
       and handoff is not null and to_char(updated_at, 'YYYY-MM-DD') = $2`,
    [orgId, date],
  );
  const escalations = Number(escalated.rows[0]?.c ?? 0);

  const p50 = async (sqlText: string) => {
    const r = await pool.query(sqlText, [orgId, date]);
    const values = r.rows.map((row) => Number(row.seconds)).filter((n) => Number.isFinite(n)).sort((a, b) => a - b);
    void r;
    if (values.length === 0) return null;
    const mid = Math.floor(values.length / 2);
    const a = values[mid];
    const b = values[mid - 1];
    if (a === undefined) return null;
    return values.length % 2 === 1 ? a : b === undefined ? a : Math.round((a + b) / 2);
  };

  const p50FirstResponse = await p50(
    `select distinct on (c.id) extract(epoch from (m.created_at - c.created_at))::int as seconds
     from conversations c
     join messages m on m.conversation_id = c.id and m.role in ('agent_ai', 'agent_human')
     where c.org_id = $1 and to_char(c.created_at, 'YYYY-MM-DD') = $2
     order by c.id, m.created_at asc`,
  );
  const p50Resolution = await p50(
    `select extract(epoch from (c.resolved_at - c.created_at))::int as seconds
     from conversations c
     where c.org_id = $1 and c.resolved_at is not null and to_char(c.created_at, 'YYYY-MM-DD') = $2`,
  );

  const metrics: AnalyticsMetrics = {
    total_conversations: total,
    ai_resolution_rate: totalResolved > 0 ? aiResolved / totalResolved : 0,
    escalation_rate: total > 0 ? escalations / total : 0,
    p50_first_response_seconds: p50FirstResponse ?? null,
    p50_resolution_seconds: p50Resolution ?? null,
  };

  await pool.query(
    `insert into analytics_daily (org_id, date, metrics, computed_at)
     values ($1, $2, $3::jsonb, now())
     on conflict (org_id, date) do update set metrics = $3::jsonb, computed_at = now()`,
    [orgId, date, JSON.stringify(metrics)],
  );
  return metrics;
}

export const ANALYTICS_JOB = JOB_NAMES.analyticsRollup;
