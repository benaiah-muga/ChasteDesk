"use client";

import { useCallback, useEffect, useState } from "react";

interface AuditRow {
  id: number;
  type: string;
  actorType: string;
  targetType: string;
  targetId: string | null;
  payload: Record<string, unknown>;
  createdAt: string;
}

export default function AuditPage() {
  const [rows, setRows] = useState<AuditRow[]>([]);
  const [type, setType] = useState("");
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    const params = new URLSearchParams();
    if (type) params.set("type", type);
    const res = await fetch(`/api/audit?${params}`);
    if (res.ok) {
      const body = await res.json();
      setRows(body.data);
    } else {
      setError("Failed to load audit log");
    }
  }, [type]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <main style={{ maxWidth: 960, margin: "0 auto" }}>
      <h1>Audit log</h1>
      <div className="row" style={{ marginBottom: 12 }}>
        <input
          className="input"
          style={{ maxWidth: 260 }}
          placeholder="Filter by event type…"
          value={type}
          onChange={(e) => setType(e.target.value)}
          aria-label="Filter by type"
        />
        <button className="btn btn-secondary" onClick={() => void load()}>
          Apply
        </button>
        <button className="btn btn-secondary" onClick={() => setType("")}>
          Clear
        </button>
      </div>
      {error ? <p className="error-text">{error}</p> : null}
      <table style={{ width: "100%", borderCollapse: "collapse" }}>
        <thead>
          <tr>
            <th style={{ textAlign: "left", padding: 8 }}>When</th>
            <th style={{ textAlign: "left", padding: 8 }}>Type</th>
            <th style={{ textAlign: "left", padding: 8 }}>Actor</th>
            <th style={{ textAlign: "left", padding: 8 }}>Target</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.id}>
              <td style={{ padding: 8, fontSize: 12 }}>{new Date(r.createdAt).toLocaleString()}</td>
              <td style={{ padding: 8 }}>
                <span className="badge">{r.type}</span>
              </td>
              <td style={{ padding: 8, fontSize: 12 }}>{r.actorType}</td>
              <td style={{ padding: 8, fontSize: 12 }}>
                {r.targetType}
                {r.targetId ? ` (${r.targetId.slice(0, 8)}…)` : ""}
              </td>
            </tr>
          ))}
          {rows.length === 0 ? (
            <tr>
              <td colSpan={4} className="muted" style={{ padding: 8 }}>
                No audit events match.
              </td>
            </tr>
          ) : null}
        </tbody>
      </table>
    </main>
  );
}
