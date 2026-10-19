// SPDX-License-Identifier: AGPL-3.0-only
import { randomUUID } from "node:crypto";
import { connection } from "next/server";
import { pool } from "@/lib/auth";
import { errorResponse, okResponse } from "@/lib/app-error";
import { authenticateWidget } from "@/lib/widget-request";
import { signConversationToken } from "@/lib/widget-token";
import { getBossSender } from "@/lib/jobs";
import { writeAuditPool } from "@chastedesk/db";
import { JOB_NAMES } from "@chastedesk/shared";
import { enforceRateLimit } from "@/lib/rate-limit";

export async function POST(req: Request) {
  try {
    await connection();
    const url = new URL(req.url);
    const auth = await authenticateWidget(req, url);
    if (!auth.ok) return Response.json({ error: { code: auth.code, message: auth.message } }, { status: auth.status });
    await enforceRateLimit(`widget:new:${auth.orgId}`, 30, 3600);

    const body = (await req.json().catch(() => ({}))) as { visitorName?: string; visitorEmail?: string; firstMessage?: string };
    const email = typeof body.visitorEmail === "string" && body.visitorEmail.includes("@") ? body.visitorEmail : null;
    let customerId: string;
    if (email) {
      const existing = await pool.query(
        "select id from customers where org_id = $1 and lower(email) = lower($2) limit 1",
        [auth.orgId, email],
      );
      customerId = (existing.rows[0] as { id: string } | undefined)?.id ?? randomUUID();
      if (!(existing.rows[0] as { id: string } | undefined)) {
        await pool.query(
          "insert into customers (id, org_id, email, name, attributes) values ($1, $2, $3, $4, '{}'::jsonb)",
          [customerId, auth.orgId, email, body.visitorName ?? "Website visitor"],
        );
      }
    } else {
      customerId = randomUUID();
      await pool.query(
        "insert into customers (id, org_id, name, attributes) values ($1, $2, $3, '{}'::jsonb)",
        [customerId, auth.orgId, body.visitorName ?? "Website visitor"],
      );
    }
    const conversationId = randomUUID();
    await pool.query(
      `insert into conversations (id, org_id, customer_id, channel, status) values ($1, $2, $3, 'widget', 'ai_active')`,
      [conversationId, auth.orgId, customerId],
    );
    await writeAuditPool(pool, auth.orgId, { actorType: "system", type: "conversation.created", targetType: "conversation", targetId: conversationId, payload: { channel: "widget" } });

    if (typeof body.firstMessage === "string" && body.firstMessage.trim()) {
      await pool.query(
        `insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', $3)`,
        [auth.orgId, conversationId, body.firstMessage.trim().slice(0, 4000)],
      );
      const boss = await getBossSender();
      await boss.send(JOB_NAMES.aiRespond, { orgId: auth.orgId, conversationId }, { singletonKey: `conv:${conversationId}` });
    }

    return okResponse({ conversationId, token: signConversationToken(auth.orgId, conversationId) }, 201);
  } catch (err) {
    return errorResponse(err);
  }
}
