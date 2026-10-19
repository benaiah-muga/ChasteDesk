// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";
import { getSessionCtx } from "@/lib/auth";
import { errorResponse, okResponse } from "@/lib/app-error";
import { requireCapability } from "@/lib/rbac";
import { getWidgetConfig } from "@/lib/repo";
import { signWidgetToken } from "@/lib/widget-token";

export async function GET(req: Request) {
  try {
    await connection();
    const ctx = await getSessionCtx(req);
    const authed = requireCapability(ctx, "settings.read");
    const config = await getWidgetConfig(authed);
    const appUrl = process.env.APP_URL ?? "";
    const token = signWidgetToken(authed.orgId, config?.origins ?? []);
    return okResponse({
      token,
      embedSnippet: `<script src="${appUrl}/widget.js" data-org="${authed.orgId}" data-token="${token}" defer></script>`,
    });
  } catch (err) {
    return errorResponse(err);
  }
}
