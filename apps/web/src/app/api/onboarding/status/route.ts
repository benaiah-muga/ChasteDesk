// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";

import { getSessionCtx } from "@/lib/auth";
import { errorResponse, okResponse } from "@/lib/app-error";
import { getOnboardingState } from "@/lib/repo";

export async function GET(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    if (!ctx) {
      return okResponse({ orgCreated: false, providerConfigured: false, knowledgeAdded: false, testSent: false, complete: false });
    }
    return okResponse(await getOnboardingState(ctx));
  } catch (err) {
    return errorResponse(err);
  }
}
