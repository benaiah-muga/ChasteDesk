// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";
import { connection } from "next/server";
import { getBossSender } from "@/lib/jobs";
import { JOB_NAMES } from "@chastedesk/shared";

export async function POST(req: Request) {
  try {
    await connection();
    const input = z.object({ orgId: z.string().min(1) }).parse(await req.json());
    const boss = await getBossSender();
    await boss.send(JOB_NAMES.analyticsRollup, { orgId: input.orgId }, { singletonKey: `rollup:${input.orgId}` });
    return Response.json({ data: { queued: true } });
  } catch (err) {
    return Response.json({ error: { code: "internal", message: String(err) } }, { status: 500 });
  }
}
