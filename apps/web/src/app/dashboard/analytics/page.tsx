"use client";

import { useEffect, useState } from "react";

interface Metrics {
  total_conversations: number;
  ai_resolution_rate: number;
  escalation_rate: number;
  p50_first_response_seconds: number | null;
  p50_resolution_seconds: number | null;
}

export default function AnalyticsPage() {
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    fetch("/api/analytics/summary")
      .then((r) => r.json())
      .then((b) => {
        setMetrics(b.data.metrics ?? null);
        setLoaded(true);
      });
  }, []);

  const cards: Array<[string, string]> = metrics
    ? [
        ["Total conversations", String(metrics.total_conversations)],
        ["AI resolution rate", `${Math.round(metrics.ai_resolution_rate * 100)}%`],
        ["Escalation rate", `${Math.round(metrics.escalation_rate * 100)}%`],
        ["Median first response", metrics.p50_first_response_seconds !== null ? `${metrics.p50_first_response_seconds}s` : "—"],
        ["Median resolution", metrics.p50_resolution_seconds !== null ? `${metrics.p50_resolution_seconds}s` : "—"],
      ]
    : [];

  return (
    <main style={{ maxWidth: 960, margin: "0 auto" }}>
      <h1>Analytics</h1>
      {!loaded ? (
        <p className="muted">Loading…</p>
      ) : !metrics ? (
        <p className="muted">No analytics yet — data appears as conversations resolve.</p>
      ) : (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))", gap: 12 }}>
          {cards.map(([label, value]) => (
            <div key={label} className="card" aria-label={label}>
              <div className="muted" style={{ fontSize: 12 }}>
                {label}
              </div>
              <div style={{ fontSize: 26, fontWeight: 600 }}>{value}</div>
            </div>
          ))}
        </div>
      )}
    </main>
  );
}
