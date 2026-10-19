// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { getSessionCtx, db } from "@/lib/auth";
import { errorResponse, okResponse } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { conversations, customers } from "@chastedesk/db";
import { and, desc, eq } from "drizzle-orm";

export async function GET(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "inbox.use");
    const url = new URL(req.url);
    const status = url.searchParams.get("status");
    const q = url.searchParams.get("q");
    const conditions = [eq(conversations.orgId, authed.orgId)];
    if (status) conditions.push(eq(conversations.status, status as never));
    const rows = await db
      .select({
        id: conversations.id,
        status: conversations.status,
        channel: conversations.channel,
        createdAt: conversations.createdAt,
        firstResponseAt: conversations.firstResponseAt,
        handoff: conversations.handoff,
        customerName: customers.name,
        customerEmail: customers.email,
      })
      .from(conversations)
      .innerJoin(customers, eq(customers.id, conversations.customerId))
      .where(and(...conditions))
      .orderBy(desc(conversations.updatedAt))
      .limit(100);
    const filtered = q
      ? rows.filter(
          (r) =>
            (r.customerName ?? "").toLowerCase().includes(q.toLowerCase()) ||
            (r.customerEmail ?? "").toLowerCase().includes(q.toLowerCase()),
        )
      : rows;
    return okResponse(filtered);
  } catch (err) {
    return errorResponse(err);
  }
}
