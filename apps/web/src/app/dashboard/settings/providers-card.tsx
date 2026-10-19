"use client";

import { useCallback, useEffect, useState } from "react";

interface ProviderRow {
  id: string;
  providerId: string;
  displayName: string;
  enabled: boolean;
  keyLast4: string | null;
  baseUrl: string | null;
}

const PROVIDER_OPTIONS = [
  { id: "openai", label: "OpenAI" },
  { id: "anthropic", label: "Anthropic" },
  { id: "google", label: "Google AI" },
  { id: "ollama", label: "Ollama (local)" },
  { id: "openai-compatible", label: "OpenAI-compatible endpoint" },
];

export default function ProvidersCard() {
  const [providers, setProviders] = useState<ProviderRow[]>([]);
  const [providerId, setProviderId] = useState("openai");
  const [displayName, setDisplayName] = useState("OpenAI");
  const [apiKey, setApiKey] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    const res = await fetch("/api/settings/providers");
    if (res.ok) {
      const body = await res.json();
      setProviders(body.data);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setMessage(null);
    const res = await fetch("/api/settings/providers", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ providerId, displayName, apiKey: apiKey || undefined, baseUrl: baseUrl || undefined }),
    });
    setBusy(false);
    if (res.ok) {
      setMessage("Provider saved. The key is stored encrypted.");
      setApiKey("");
      await load();
    } else {
      const body = await res.json().catch(() => null);
      setError(body?.error?.message ?? "Save failed");
    }
  }

  async function test(id: string) {
    setBusy(true);
    setError(null);
    setMessage(null);
    const res = await fetch(`/api/settings/providers/${id}/connect`, { method: "POST" });
    const body = await res.json();
    setBusy(false);
    if (res.ok) {
      setMessage(body.data.ok ? `Connected. ${body.data.models.length} models available.` : `Connection failed: ${body.data.error}`);
    } else {
      setError("Test failed");
    }
  }

  async function remove(id: string) {
    setBusy(true);
    await fetch(`/api/settings/providers/${id}`, { method: "DELETE" });
    setBusy(false);
    await load();
  }

  return (
    <section className="card" aria-label="AI providers">
      <h2>AI providers</h2>
      <ul style={{ listStyle: "none", padding: 0, margin: "0 0 16px" }}>
        {providers.map((p) => (
          <li key={p.id} className="row" style={{ justifyContent: "space-between", padding: "8px 0", borderBottom: "1px solid var(--border)" }}>
            <span>
              <strong>{p.displayName}</strong> <span className="muted">({p.providerId})</span>{" "}
              {p.keyLast4 ? <span className="badge">key ••••{p.keyLast4}</span> : <span className="badge">no key</span>}
              {p.baseUrl ? <span className="muted"> {p.baseUrl}</span> : null}
            </span>
            <span className="row">
              <button className="btn btn-secondary" disabled={busy} onClick={() => test(p.id)}>
                Test
              </button>
              <button className="btn btn-danger" disabled={busy} onClick={() => remove(p.id)}>
                Remove
              </button>
            </span>
          </li>
        ))}
        {providers.length === 0 ? <li className="muted">No providers configured yet.</li> : null}
      </ul>

      <form onSubmit={save} className="stack">
        <div>
          <label className="label" htmlFor="provider">Provider</label>
          <select id="provider" className="select" value={providerId} onChange={(e) => setProviderId(e.target.value)}>
            {PROVIDER_OPTIONS.map((o) => (
              <option key={o.id} value={o.id}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="label" htmlFor="displayName">Display name</label>
          <input id="displayName" className="input" value={displayName} onChange={(e) => setDisplayName(e.target.value)} required />
        </div>
        <div>
          <label className="label" htmlFor="apiKey">API key {providerId === "ollama" ? "(optional for local Ollama)" : ""}</label>
          <input id="apiKey" className="input" type="password" value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder="Stored encrypted, never shown again" />
        </div>
        <div>
          <label className="label" htmlFor="baseUrl">Base URL (optional)</label>
          <input id="baseUrl" className="input" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="https://…" />
        </div>
        {message ? <p className="badge badge-success">{message}</p> : null}
        {error ? <p className="error-text">{error}</p> : null}
        <button className="btn" type="submit" disabled={busy}>
          Save provider
        </button>
      </form>
    </section>
  );
}
