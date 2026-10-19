// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { pool } from "@/lib/auth";
import { authenticateWidget } from "@/lib/widget-request";
import { verifyConversationToken } from "@/lib/widget-token";
import { subscribePg } from "@/lib/sse";


export async function GET(req: Request, { params }: { params: Promise<{ id: string }> }) {
  await connection();
  const url = new URL(req.url);
  const auth = await authenticateWidget(req, url);
  if (!auth.ok) return Response.json({ error: { code: auth.code, message: auth.message } }, { status: auth.status });
  const { id: conversationId } = await params;
  const convToken = url.searchParams.get("convToken") ?? "";
  if (!verifyConversationToken(auth.orgId, conversationId, convToken)) {
    return Response.json({ error: { code: "unauthorized", message: "invalid conversation token" } }, { status: 401 });
  }

  const encoder = new TextEncoder();
  const stream = new ReadableStream({
    async start(controller) {
      let closed = false;
      const send = (event: string, data: unknown) => {
        if (closed) return;
        controller.enqueue(encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`));
      };
      send("ready", { conversationId });

      const pushLatestReply = async () => {
        const res = await pool.query(
          `select id, content, citations, created_at from messages
           where conversation_id = $1 and role = 'agent_ai' order by created_at desc limit 1`,
          [conversationId],
        );
        const row = res.rows[0] as { id: string; content: string; citations: unknown } | undefined;
        if (row) {
          send("message", { id: row.id, role: "agent_ai", content: row.content, citations: row.citations });
        }
      };

      // Replay the latest agent message so late subscribers catch up.
      await pushLatestReply();

      const sub = await subscribePg((payload) => {
        if (payload.orgId !== auth.orgId || payload.conversationId !== conversationId) return;
        if (payload.type === "message.created" || payload.type === "action.pending") {
          void pushLatestReply();
        }
      });

      const heartbeat = setInterval(() => {
        if (!closed) send("ping", { t: Date.now() });
      }, 15_000);

      const cleanup = () => {
        if (closed) return;
        closed = true;
        clearInterval(heartbeat);
        void sub.stop();
        try {
          controller.close();
        } catch {
          /* already closed */
        }
      };
      req.signal.addEventListener("abort", cleanup);
      // Hard cap the stream at 10 minutes; the widget reconnects.
      setTimeout(cleanup, 10 * 60_000);
    },
  });

  return new Response(stream, {
    headers: {
      "content-type": "text/event-stream",
      "cache-control": "no-cache, no-transform",
      connection: "keep-alive",
    },
  });
}
