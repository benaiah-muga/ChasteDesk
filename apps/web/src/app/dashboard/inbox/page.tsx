"use client";

import { useCallback, useEffect, useState } from "react";

interface ConversationRow {
  id: string;
  status: string;
  channel: string;
  customerName: string | null;
  customerEmail: string | null;
  createdAt: string;
}

interface Message {
  id: string;
  role: string;
  content: string;
  citations: Array<{ chunkId: string; quote: string; version?: number }> | null;
  createdAt: string;
}

interface Detail {
  conversation: { id: string; status: string; handoff: Handoff | null; firstResponseAt: string | null };
  customer: { id: string; name: string | null; email: string | null; attributes: Record<string, unknown> } | null;
  messages: Message[];
  recentConversations: Array<{ id: string; status: string; createdAt: string }>;
  openConversations: number;
  actions: Array<{ id: string; toolId: string; args: Record<string, unknown>; status: string }>;
}

interface Handoff {
  problem: string;
  customerContext: { name: string | null; email: string | null; attributes: Record<string, unknown>; recentConversations: string[] };
  investigated: string[];
  evidence: Array<{ source: string; version: number; quote: string }>;
  actionsTaken: string[];
  recommendedNextStep: string;
}

export default function InboxPage() {
  const [list, setList] = useState<ConversationRow[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [detail, setDetail] = useState<Detail | null>(null);
  const [reply, setReply] = useState("");
  const [internal, setInternal] = useState(false);
  const [copilot, setCopilot] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const loadList = useCallback(async () => {
    const res = await fetch("/api/inbox/conversations");
    if (res.ok) setList((await res.json()).data);
  }, []);

  const loadDetail = useCallback(async (id: string) => {
    const res = await fetch(`/api/inbox/conversations/${id}`);
    if (res.ok) setDetail((await res.json()).data);
  }, []);

  useEffect(() => {
    void loadList();
    const timer = setInterval(() => void loadList(), 4000);
    return () => clearInterval(timer);
  }, [loadList]);

  useEffect(() => {
    if (!selected) return;
    void loadDetail(selected);
    const timer = setInterval(() => void loadDetail(selected), 4000);
    return () => clearInterval(timer);
  }, [selected, loadDetail]);

  async function sendReply() {
    if (!selected || !reply.trim()) return;
    setBusy(true);
    await fetch(`/api/inbox/conversations/${selected}/reply`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ content: reply, internal }),
    });
    setReply("");
    setBusy(false);
    await loadDetail(selected);
    await loadList();
  }

  async function resolveConversation() {
    if (!selected) return;
    await fetch(`/api/inbox/conversations/${selected}/resolve`, { method: "POST", body: JSON.stringify({ action: "resolve" }) });
    await loadDetail(selected);
    await loadList();
  }

  async function decide(actionId: string, decision: "approve" | "reject") {
    await fetch("/api/actions/decide", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ actionId, decision }) });
    if (selected) await loadDetail(selected);
  }

  async function draft() {
    if (!selected) return;
    setBusy(true);
    const res = await fetch("/api/copilot/draft", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: selected }) });
    const body = await res.json();
    setBusy(false);
    setCopilot(res.ok ? body.data.draft : "Copilot could not draft a reply.");
  }

  async function sendCopilot() {
    if (!selected || !copilot) return;
    setBusy(true);
    await fetch("/api/copilot/send", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: selected, content: copilot }) });
    setCopilot(null);
    setBusy(false);
    await loadDetail(selected);
  }

  return (
    <main style={{ display: "flex", gap: 16, height: "calc(100vh - 60px)" }}>
      <aside style={{ width: 320, borderRight: "1px solid var(--border)", overflowY: "auto", padding: 12 }} aria-label="Conversations">
        <h2>Inbox</h2>
        {list.map((c) => (
          <div
            key={c.id}
            onClick={() => setSelected(c.id)}
            style={{ padding: 10, borderBottom: "1px solid var(--border)", cursor: "pointer", background: selected === c.id ? "var(--accent-soft)" : "transparent" }}
            data-conversation={c.id}
          >
            <div className="row" style={{ justifyContent: "space-between" }}>
              <strong>{c.customerName ?? c.customerEmail ?? "Visitor"}</strong>
              <span className={c.status === "resolved" ? "badge badge-success" : c.status === "awaiting_human" ? "badge badge-warn" : "badge"}>{c.status.replace(/_/g, " ")}</span>
            </div>
            <div className="muted" style={{ fontSize: 12 }}>
              {c.customerEmail} · {new Date(c.createdAt).toLocaleString()}
            </div>
          </div>
        ))}
        {list.length === 0 ? <p className="muted">No conversations yet.</p> : null}
      </aside>

      <section style={{ flex: 1, overflowY: "auto", padding: 12 }} aria-label="Conversation detail">
        {!detail ? (
          <p className="muted">Select a conversation.</p>
        ) : (
          <div className="stack">
            <div className="row" style={{ justifyContent: "space-between" }}>
              <h2 style={{ margin: 0 }}>Conversation</h2>
              <div className="row">
                {detail.conversation.status === "awaiting_human" || detail.conversation.status === "human_active" ? (
                  <button className="btn btn-secondary" onClick={() => resolveConversation()} disabled={busy}>
                    Resolve
                  </button>
                ) : null}
              </div>
            </div>

            {detail.conversation.handoff ? (
              <div className="card" style={{ background: "var(--accent-soft)" }} aria-label="Handoff summary">
                <h2>Why the AI escalated</h2>
                <p>
                  <strong>Problem:</strong> {detail.conversation.handoff.problem}
                </p>
                <p>
                  <strong>Recommended next step:</strong> {detail.conversation.handoff.recommendedNextStep}
                </p>
                {detail.conversation.handoff.evidence.length > 0 ? (
                  <p className="muted">
                    Evidence: {detail.conversation.handoff.evidence.map((e) => `${e.source} v${e.version}`).join(", ")}
                  </p>
                ) : null}
              </div>
            ) : null}

            <div className="card" aria-label="Messages">
              <h2>Messages</h2>
              {detail.messages.map((m) => (
                <div key={m.id} style={{ margin: "8px 0", padding: 8, borderLeft: `3px solid ${m.role === "customer" ? "var(--border)" : m.role === "internal_note" ? "var(--warn)" : "var(--accent)"}` }}>
                  <div className="muted" style={{ fontSize: 11 }}>
                    {m.role.replace(/_/g, " ")}
                  </div>
                  <div>{m.content}</div>
                  {m.citations && m.citations.length > 0 ? (
                    <div className="muted" style={{ fontSize: 11, marginTop: 4 }}>
                      Cited sources: {m.citations.map((c, i) => `[${i + 1}] ${c.quote?.slice(0, 60)}…`).join(" | ")}
                    </div>
                  ) : null}
                </div>
              ))}
            </div>

            <div className="card">
              <h2>Reply</h2>
              <textarea className="textarea" rows={4} value={reply} onChange={(e) => setReply(e.target.value)} placeholder="Write a reply…" />
              <div className="row" style={{ marginTop: 8 }}>
                <label className="row">
                  <input type="checkbox" checked={internal} onChange={(e) => setInternal(e.target.checked)} /> internal note
                </label>
                <button className="btn" onClick={sendReply} disabled={busy || !reply.trim()}>
                  Send
                </button>
                <button className="btn btn-secondary" onClick={draft} disabled={busy}>
                  Draft with copilot
                </button>
              </div>
              {copilot ? (
                <div className="card" style={{ marginTop: 8 }}>
                  <h2>Copilot draft (not sent)</h2>
                  <pre style={{ whiteSpace: "pre-wrap", fontSize: 13 }}>{copilot}</pre>
                  <button className="btn" onClick={sendCopilot} disabled={busy}>
                    Send draft
                  </button>
                </div>
              ) : null}
            </div>
          </div>
        )}
      </section>

      <aside style={{ width: 300, borderLeft: "1px solid var(--border)", overflowY: "auto", padding: 12 }} aria-label="Approvals and customer">
        <h2>Approvals</h2>
        {(detail?.actions ?? []).filter((a) => a.status === "pending_approval").map((a) => (
          <div key={a.id} className="card" style={{ marginBottom: 8 }}>
            <strong>{a.toolId}</strong>
            <pre style={{ fontSize: 11, whiteSpace: "pre-wrap" }}>{JSON.stringify(a.args, null, 2)}</pre>
            <div className="row">
              <button className="btn" onClick={() => decide(a.id, "approve")}>
                Approve
              </button>
              <button className="btn btn-danger" onClick={() => decide(a.id, "reject")}>
                Reject
              </button>
            </div>
          </div>
        ))}
        {(detail?.actions ?? []).filter((a) => a.status === "pending_approval").length === 0 ? <p className="muted">No pending approvals.</p> : null}

        {detail?.customer ? (
          <div className="card" style={{ marginTop: 16 }} aria-label="Customer context">
            <h2>Customer</h2>
            <p>
              <strong>{detail.customer.name}</strong>
              <br />
              {detail.customer.email}
            </p>
            <p className="muted">
              {detail.openConversations} other open conversation{detail.openConversations === 1 ? "" : "s"}
            </p>
            {Object.entries(detail.customer.attributes ?? {}).map(([k, v]) => (
              <p key={k} style={{ fontSize: 12 }}>
                {k}: {String(v)}
              </p>
            ))}
            <ul style={{ paddingLeft: 16, fontSize: 12 }}>
              {detail.recentConversations.map((c) => (
                <li key={c.id}>
                  {c.status} · {new Date(c.createdAt).toLocaleDateString()}
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </aside>
    </main>
  );
}
