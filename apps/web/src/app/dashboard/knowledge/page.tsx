"use client";

import { useCallback, useEffect, useRef, useState } from "react";

interface Source {
  id: string;
  type: string;
  name: string;
  scope: "public" | "internal";
  status: string;
  version: number;
  error: string | null;
}

export default function KnowledgePage() {
  const [sources, setSources] = useState<Source[]>([]);
  const [kind, setKind] = useState<"url" | "file" | "text">("url");
  const [url, setUrl] = useState("");
  const [name, setName] = useState("");
  const [text, setText] = useState("");
  const [scope, setScope] = useState<"public" | "internal">("public");
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    const res = await fetch("/api/knowledge/sources");
    if (res.ok) {
      const body = await res.json();
      setSources(body.data);
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 3000);
    return () => clearInterval(timer);
  }, [load]);

  async function add(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    let res: Response;
    if (kind === "file") {
      if (!file) {
        setBusy(false);
        setError("Choose a file first");
        return;
      }
      const form = new FormData();
      form.set("file", file);
      form.set("scope", scope);
      res = await fetch("/api/knowledge/sources", { method: "POST", body: form });
    } else if (kind === "url") {
      res = await fetch("/api/knowledge/sources", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ kind: "url", url, name: name || url, scope }),
      });
    } else {
      res = await fetch("/api/knowledge/sources", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ kind: "text", name: name || "Pasted text", text, scope }),
      });
    }
    setBusy(false);
    if (res.ok) {
      setUrl("");
      setText("");
      setFile(null);
      if (fileRef.current) fileRef.current.value = "";
      await load();
    } else {
      const body = await res.json().catch(() => null);
      setError(body?.error?.message ?? "Could not add source");
    }
  }

  async function toggleScope(source: Source) {
    await fetch("/api/knowledge/sources-update", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ sourceId: source.id, scope: source.scope === "public" ? "internal" : "public" }),
    });
    await load();
  }

  async function reindex(id: string) {
    await fetch("/api/knowledge/sources-reindex", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ sourceId: id }) });
    await load();
  }

  async function remove(id: string) {
    await fetch("/api/knowledge/sources-update", { method: "DELETE", headers: { "content-type": "application/json" }, body: JSON.stringify({ sourceId: id }) });
    await load();
  }

  return (
    <main style={{ maxWidth: 860, margin: "0 auto" }}>
      <h1>Knowledge</h1>
      <section className="card" style={{ marginBottom: 24 }} aria-label="Add knowledge source">
        <h2>Add a source</h2>
        <div className="row" style={{ marginBottom: 12 }}>
          {(["url", "file", "text"] as const).map((k) => (
            <button key={k} className={kind === k ? "btn" : "btn btn-secondary"} type="button" onClick={() => setKind(k)}>
              {k === "url" ? "Website" : k === "file" ? "File" : "Pasted text"}
            </button>
          ))}
        </div>
        <form onSubmit={add} className="stack">
          {kind === "url" ? (
            <input className="input" type="url" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://example.com/help" required />
          ) : null}
          {kind === "file" ? (
            <input ref={fileRef} className="input" type="file" accept=".pdf,.md,.txt,.docx" onChange={(e) => setFile(e.target.files?.[0] ?? null)} required />
          ) : null}
          {kind === "text" ? (
            <>
              <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Title" required />
              <textarea className="textarea" rows={5} value={text} onChange={(e) => setText(e.target.value)} placeholder="Paste the content…" required />
            </>
          ) : null}
          <div className="row">
            <label className="label" htmlFor="scope" style={{ margin: 0 }}>
              Scope
            </label>
            <select id="scope" className="select" style={{ width: 160 }} value={scope} onChange={(e) => setScope(e.target.value as "public" | "internal")}>
              <option value="public">Public (customer-facing)</option>
              <option value="internal">Internal only</option>
            </select>
            <button className="btn" type="submit" disabled={busy}>
              Add source
            </button>
          </div>
          {error ? <p className="error-text">{error}</p> : null}
        </form>
      </section>

      <section className="card" aria-label="Knowledge sources">
        <h2>Sources</h2>
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th style={{ textAlign: "left", padding: 8 }}>Name</th>
              <th style={{ textAlign: "left", padding: 8 }}>Type</th>
              <th style={{ textAlign: "left", padding: 8 }}>Status</th>
              <th style={{ textAlign: "left", padding: 8 }}>Scope</th>
              <th style={{ textAlign: "left", padding: 8 }}>Version</th>
              <th style={{ padding: 8 }}></th>
            </tr>
          </thead>
          <tbody>
            {sources.map((s) => (
              <tr key={s.id} data-source-name={s.name}>
                <td style={{ padding: 8 }}>{s.name}</td>
                <td style={{ padding: 8 }}>{s.type}</td>
                <td style={{ padding: 8 }}>
                  <span className={s.status === "ready" ? "badge badge-success" : s.status === "failed" ? "badge badge-warn" : "badge"}>
                    {s.status}
                  </span>
                  {s.error ? <span className="error-text"> {s.error}</span> : null}
                </td>
                <td style={{ padding: 8 }}>
                  <button className="btn btn-secondary" type="button" onClick={() => toggleScope(s)}>
                    {s.scope}
                  </button>
                </td>
                <td style={{ padding: 8 }}>v{s.version}</td>
                <td style={{ padding: 8, textAlign: "right" }}>
                  <button className="btn btn-secondary" type="button" onClick={() => reindex(s.id)}>
                    Reindex
                  </button>{" "}
                  <button className="btn btn-danger" type="button" onClick={() => remove(s.id)}>
                    Delete
                  </button>
                </td>
              </tr>
            ))}
            {sources.length === 0 ? (
              <tr>
                <td colSpan={6} className="muted" style={{ padding: 8 }}>
                  No sources yet.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </section>
    </main>
  );
}
