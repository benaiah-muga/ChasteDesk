// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";

import { getSessionCtx } from "@/lib/auth";
import { errorResponse, okResponse } from "@/lib/app-error";
import { setOnboardingComplete } from "@/lib/repo";

export async function POST(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    if (!ctx) {
      return okResponse({ complete: false }, 401);
    }
    const body = (await req.json().catch(() => ({}))) as { complete?: boolean };
    await setOnboardingComplete(ctx, body.complete !== false);
    return okResponse({ complete: body.complete !== false });
  } catch (err) {
    return errorResponse(err);
  }
}
