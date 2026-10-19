"use client";

import { useCallback, useEffect, useState } from "react";

interface WidgetConfig {
  origins: string[];
  enabled: boolean;
  theme: Record<string, string>;
}

export default function WidgetCard() {
  const [config, setConfig] = useState<WidgetConfig | null>(null);
  const [originsText, setOriginsText] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    const res = await fetch("/api/settings/widget");
    if (res.ok) {
      const body = await res.json();
      setConfig(body.data);
      setOriginsText((body.data.origins ?? []).join("\n"));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    const origins = originsText.split("\n").map((s) => s.trim()).filter(Boolean);
    const res = await fetch("/api/settings/widget", {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ origins, theme: config?.theme ?? {} }),
    });
    setBusy(false);
    setMessage(res.ok ? "Widget settings saved." : "Save failed");
    if (res.ok) await load();
  }

  if (!config) return null;

  return (
    <section className="card" aria-label="Chat widget">
      <h2>Chat widget</h2>
      <form onSubmit={save} className="stack">
        <div>
          <label className="label" htmlFor="origins">Allowed origins (one per line)</label>
          <textarea id="origins" className="textarea" rows={3} value={originsText} onChange={(e) => setOriginsText(e.target.value)} placeholder={"https://example.com"} />
        </div>
        {message ? <p className="badge badge-success">{message}</p> : null}
        <button className="btn" type="submit" disabled={busy}>
          Save widget settings
        </button>
      </form>
    </section>
  );
}
