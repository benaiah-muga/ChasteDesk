// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { getSessionCtx } from "@/lib/auth";
import { subscribePg } from "@/lib/sse";

export async function GET(req: Request) {
  await connection();
  const ctx = await getSessionCtx(req);
  if (!ctx) return Response.json({ error: { code: "unauthorized", message: "login required" } }, { status: 401 });

  const encoder = new TextEncoder();
  const orgId = ctx.orgId;
  const stream = new ReadableStream({
    async start(controller) {
      let closed = false;
      const send = (event: string, data: unknown) => {
        if (closed) return;
        try {
          controller.enqueue(encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`));
        } catch {
          closed = true;
        }
      };
      send("ready", { orgId });

      const sub = await subscribePg((payload) => {
        if (payload.orgId !== orgId) return;
        send(payload.type, { conversationId: payload.conversationId, type: payload.type });
      });
      const heartbeat = setInterval(() => {
        if (!closed) {
          try {
            controller.enqueue(encoder.encode(": ping\n\n"));
          } catch {
            closed = true;
          }
        }
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
      setTimeout(cleanup, 10 * 60_000);
    },
  });

  return new Response(stream, {
    headers: { "content-type": "text/event-stream", "cache-control": "no-cache, no-transform" },
  });
}
