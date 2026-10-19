// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";

import { z } from "zod";
import { auth } from "@/lib/auth";
import { errorResponse, okResponse, AppError } from "@/lib/app-error";
import { orgExists, seedOrgDefaults } from "@/lib/repo";
import { enforceRateLimit } from "@/lib/rate-limit";

const bootstrapSchema = z.object({
  orgName: z.string().min(1).max(100),
  userName: z.string().min(1).max(100),
  email: z.string().email(),
  password: z.string().min(8).max(200),
});

export async function POST(req: Request) {
  try {
    await connection();
    await enforceRateLimit("bootstrap", 20, 3600);
    if (await orgExists()) {
      throw new AppError("conflict", "this installation already has an organization", 409);
    }
    const body = bootstrapSchema.parse(await req.json());
    const signUp = await auth.api.signUpEmail({
      body: { name: body.userName, email: body.email, password: body.password },
    });
    const org = await auth.api.createOrganization({
      body: { name: body.orgName, slug: "main", userId: signUp.user.id },
    });
    if (!org) {
      throw new AppError("internal", "organization creation failed", 500);
    }
    await seedOrgDefaults(org.id, signUp.user.id);
    return okResponse({ orgId: org.id, userId: signUp.user.id }, 201);
  } catch (err) {
    return errorResponse(err);
  }
}
