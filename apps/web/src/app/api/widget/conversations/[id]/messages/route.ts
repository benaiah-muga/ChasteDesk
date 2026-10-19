// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { pool } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { authenticateWidget } from "@/lib/widget-request";
import { verifyConversationToken } from "@/lib/widget-token";
import { getBossSender } from "@/lib/jobs";
import { enforceRateLimit } from "@/lib/rate-limit";
import { JOB_NAMES } from "@chastedesk/shared";

export async function POST(req: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    await connection();
    const url = new URL(req.url);
    const auth = await authenticateWidget(req, url);
    if (!auth.ok) return Response.json({ error: { code: auth.code, message: auth.message } }, { status: auth.status });
    const { id: conversationId } = await params;
    const convToken = url.searchParams.get("convToken") ?? "";
    if (!verifyConversationToken(auth.orgId, conversationId, convToken)) {
      throw new AppError("unauthorized", "invalid conversation token", 401);
    }

    const conv = await pool.query("select status from conversations where id = $1 and org_id = $2", [conversationId, auth.orgId]);
    if (!conv.rows[0]) throw new AppError("not_found", "conversation not found", 404);
    if (["resolved", "closed"].includes(conv.rows[0].status)) throw new AppError("conflict", "conversation is closed", 409);

    const body = (await req.json().catch(() => ({}))) as { content?: string };
    const content = typeof body.content === "string" ? body.content.trim().slice(0, 4000) : "";
    if (!content) throw new AppError("bad_request", "content is required", 400);

    await enforceRateLimit(`widget:msg:${conversationId}`, 20, 60);
    await enforceRateLimit(`widget:msg-ip:${req.headers.get("x-forwarded-for") ?? "local"}`, 60, 3600);

    await pool.query(
      `insert into messages (org_id, conversation_id, role, content) values ($1, $2, 'customer', $3)`,
      [auth.orgId, conversationId, content],
    );
    const boss = await getBossSender();
    await boss.send(
      JOB_NAMES.aiRespond,
      { orgId: auth.orgId, conversationId },
      { singletonKey: `conv:${conversationId}`, retryLimit: 1, retryDelay: 5 },
    );
    return okResponse({ received: true }, 202);
  } catch (err) {
    return errorResponse(err);
  }
}
