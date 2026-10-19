// SPDX-License-Identifier: AGPL-3.0-only
import { betterAuth } from "better-auth";
import { organization } from "better-auth/plugins";
import { drizzleAdapter } from "better-auth/adapters/drizzle";
import { createDb, type RepoCtx } from "@chastedesk/db";
import { eq } from "drizzle-orm";
import * as schema from "@chastedesk/db";

const { db, pool } = createDb(process.env.DATABASE_URL ?? "");

async function orgIdForUser(userId: string): Promise<string | null> {
  const rows = await db.select({ organizationId: schema.member.organizationId }).from(schema.member).where(eq(schema.member.userId, userId)).limit(1);
  return rows[0]?.organizationId ?? null;
}
export { db, pool };

export const auth = betterAuth({
  appName: "ChasteDesk",
  secret: process.env.APP_SECRET,
  baseURL: process.env.APP_URL ?? "http://localhost:3000",
  trustedOrigins: (request?: Request) => {
    const origin = request?.headers.get("origin");
    return origin ? [origin] : [];
  },
  database: drizzleAdapter(db, { provider: "pg", schema }),
  emailAndPassword: {
    enabled: true,
    minPasswordLength: 8,
  },
  plugins: [organization()],
  advanced: {
    defaultCookieAttributes: {
      httpOnly: true,
      sameSite: "lax",
    },
  },
  databaseHooks: {
    session: {
      create: {
        after: async (session) => {
          try {
            const { writeAuditPool } = await import("@chastedesk/db");
            await writeAuditPool(pool, await orgIdForUser(session.userId), { actorType: "user", actorId: session.userId, type: "auth.login", targetType: "session", targetId: session.id, payload: {} });
          } catch {
            /* audit is best-effort; login must not fail */
          }
        },
      },
    },
  },
  rateLimit: {
    enabled: true,
    window: 900,
    max: 10,
    storage: "database",
  },
});

export interface SessionCtx extends RepoCtx {
  userId: string;
  role: string;
  email: string;
}

export async function getSessionCtx(req: Request): Promise<SessionCtx | null> {
  const sessionResult = await auth.api.getSession({ headers: req.headers });
  if (!sessionResult?.user) return null;
  const members = await db
    .select()
    .from(schema.member)
    .where(eq(schema.member.userId, sessionResult.user.id));
  const membership = members[0];
  if (!membership) return null;
  return {
    userId: sessionResult.user.id,
    email: sessionResult.user.email,
    orgId: membership.organizationId,
    role: membership.role,
  };
}
