// SPDX-License-Identifier: AGPL-3.0-only
import { randomUUID } from "node:crypto";
import { connection } from "next/server";
import { db, pool } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { resolveApiKey, type ApiKeyCtx } from "@/lib/sse";
import { getBossSender } from "@/lib/jobs";
import { publishEventWeb } from "@/lib/publish";
import { customers, conversations, messages } from "@chastedesk/db";
import { JOB_NAMES } from "@chastedesk/shared";
import { and, desc, eq } from "drizzle-orm";
import { z } from "zod";

async function ctxFromApiKey(req: Request): Promise<ApiKeyCtx> {
  const ctx = await resolveApiKey(req);
  if (!ctx) throw new AppError("unauthorized", "valid API key required", 401);
  return ctx;
}

export async function POST(req: Request) {
  try {
    await connection();
    const api = await ctxFromApiKey(req);
    const input = z
      .object({
        customer: z.object({ email: z.string().email(), name: z.string().max(200).optional(), attributes: z.record(z.string(), z.unknown()).optional() }),
        message: z.string().min(1).max(4000),
      })
      .parse(await req.json());

    const existing = await db
      .select({ id: customers.id })
      .from(customers)
      .where(and(eq(customers.orgId, api.orgId), eq(customers.email, input.customer.email)))
      .limit(1);
    const customerId = existing[0]?.id ?? randomUUID();
    if (!existing[0]) {
      await db.insert(customers).values({ id: customerId, orgId: api.orgId, email: input.customer.email, name: input.customer.name ?? input.customer.email, attributes: input.customer.attributes ?? {} });
    }
    const conversationId = randomUUID();
    await db.insert(conversations).values({ id: conversationId, orgId: api.orgId, customerId, channel: "api", status: "ai_active" });
    await db.insert(messages).values({ orgId: api.orgId, conversationId, role: "customer", content: input.message });
        const { rows } = await pool.query(
      "insert into audit_events (org_id, actor_type, type, target_type, target_id, payload) values ($1, 'system', 'conversation.created', 'conversation', $2, $3::jsonb)",
      [api.orgId, conversationId, JSON.stringify({ channel: "api" })],
    );
    void rows;

    const boss = await getBossSender();
    await boss.send(JOB_NAMES.aiRespond, { orgId: api.orgId, conversationId }, { singletonKey: `conv:${conversationId}` });
    return okResponse({ conversationId }, 201);
  } catch (err) {
    return errorResponse(err);
  }
}

export async function GET(req: Request) {
  try {
    await connection();
    const api = await ctxFromApiKey(req);
    const rows = await db
      .select({ id: conversations.id, status: conversations.status, createdAt: conversations.createdAt })
      .from(conversations)
      .where(eq(conversations.orgId, api.orgId))
      .orderBy(desc(conversations.createdAt))
      .limit(50);
    return okResponse(rows);
  } catch (err) {
    return errorResponse(err);
  }
}

export async function PUT(req: Request) {
  try {
    await connection();
    const api = await ctxFromApiKey(req);
    const input = z.object({ conversationId: z.string().uuid(), content: z.string().min(1).max(4000) }).parse(await req.json());
    const conv = await db
      .select({ id: conversations.id })
      .from(conversations)
      .where(and(eq(conversations.id, input.conversationId), eq(conversations.orgId, api.orgId)))
      .limit(1);
    if (!conv[0]) throw new AppError("not_found", "conversation not found", 404);
    await db.insert(messages).values({ orgId: api.orgId, conversationId: input.conversationId, role: "customer", content: input.content });
    const boss = await getBossSender();
    await boss.send(JOB_NAMES.aiRespond, { orgId: api.orgId, conversationId: input.conversationId }, { singletonKey: `conv:${input.conversationId}` });
    await publishEventWeb({ orgId: api.orgId, type: "message.created", conversationId: input.conversationId });
    return okResponse({ received: true }, 202);
  } catch (err) {
    return errorResponse(err);
  }
}
