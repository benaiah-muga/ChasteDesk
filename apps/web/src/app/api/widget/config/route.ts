// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { authenticateWidget } from "@/lib/widget-request";
import { errorResponse, okResponse } from "@/lib/app-error";

export async function GET(req: Request) {
  try {
    await connection();
    const url = new URL(req.url);
    const auth = await authenticateWidget(req, url);
    if (!auth.ok) return Response.json({ error: { code: auth.code, message: auth.message } }, { status: auth.status });
    return okResponse({ displayName: auth.theme.displayName ?? "Support", theme: auth.theme });
  } catch (err) {
    return errorResponse(err);
  }
}
