"use client";

import { useCallback, useEffect, useState } from "react";
import ProvidersCard from "../dashboard/settings/providers-card";

interface OnboardingStatus {
  orgCreated: boolean;
  providerConfigured: boolean;
  knowledgeAdded: boolean;
  testSent: boolean;
  complete: boolean;
}

export default function OnboardingPage() {
  const [status, setStatus] = useState<OnboardingStatus | null>(null);
  const [url, setUrl] = useState("https://example.com/help");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  const load = useCallback(async () => {
    const res = await fetch("/api/onboarding/status");
    if (res.ok) {
      const body = await res.json();
      setStatus(body.data);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function addKnowledge() {
    setBusy(true);
    await fetch("/api/knowledge/sources", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ kind: "url", url, scope: "public" }),
    });
    setBusy(false);
    await load();
  }

  async function finish() {
    setBusy(true);
    await fetch("/api/onboarding/complete", { method: "POST", body: JSON.stringify({ complete: true }) });
    setBusy(false);
    setDone(true);
    await load();
  }

  if (done || status?.complete) {
    return (
      <main style={{ maxWidth: 640, margin: "64px auto", padding: 24 }}>
        <div className="card">
          <h1>Your desk is ready</h1>
          <p className="muted">Head to the inbox to watch your AI handle conversations, or keep tuning knowledge and providers in settings.</p>
          <a className="btn" href="/dashboard/inbox">
            Open inbox
          </a>
        </div>
      </main>
    );
  }

  return (
    <main style={{ maxWidth: 640, margin: "64px auto", padding: 24 }}>
      <h1>Let’s get your support desk running</h1>
      <p className="muted">I’ll check your setup, connect an AI provider, and index your first knowledge source.</p>
      <div className="stack">
        <section className="card" aria-label="Step provider">
          <h2>
            Step 1 — Connect an AI provider{" "}
            {status?.providerConfigured ? <span className="badge badge-success">done</span> : <span className="badge">pending</span>}
          </h2>
          <ProvidersCard />
        </section>

        <section className="card" aria-label="Step knowledge">
          <h2>
            Step 2 — Add your first knowledge source{" "}
            {status?.knowledgeAdded ? <span className="badge badge-success">done</span> : <span className="badge">pending</span>}
          </h2>
          <p className="muted">This can be a public help page — the AI will cite it when answering customers.</p>
          <div className="row">
            <input className="input" type="url" value={url} onChange={(e) => setUrl(e.target.value)} aria-label="Knowledge URL" />
            <button className="btn" type="button" onClick={addKnowledge} disabled={busy}>
              Add
            </button>
            <a className="btn btn-secondary" href="/dashboard/knowledge">
              Open knowledge
            </a>
          </div>
        </section>

        <section className="card" aria-label="Step finish">
          <h2>Step 3 — Finish</h2>
          <p className="muted">You can change everything later in settings. The AI only does what your resolution policy allows.</p>
          <button className="btn" type="button" onClick={finish} disabled={busy}>
            Finish setup
          </button>
        </section>
      </div>
    </main>
  );
}
