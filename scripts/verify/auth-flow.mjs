// SPDX-License-Identifier: AGPL-3.0-only
// Gate: signup, login, session read, logout, and revocation with secure cookie flags.
import { withEphemeralPg } from "./lib/ephemeral-pg.mjs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { Jar, ensureWebBuilt, req, startWebApp } from "./lib/web-app.mjs";

ensureWebBuilt();
const PORT = 4311;

await withEphemeralPg(async (pgx) => {
  const app = await startWebApp({ dbUrl: pgx.connectionString, port: PORT, appUrl: `https://127.0.0.1:${PORT}` });
  try {
    const boot = await req(`http://127.0.0.1:${PORT}/api/bootstrap`, {
      method: "POST",
      body: { orgName: "Flow Org", userName: "Owner", email: "owner@flow.test", password: "password123" },
    });
    assert(boot.res.status === 201, `bootstrap failed: ${boot.res.status} ${JSON.stringify(boot.json)}`);

    const jar = new Jar();
    const signup = await req(`http://127.0.0.1:${PORT}/api/auth/sign-up/email`, {
      method: "POST",
      body: { name: "Agent One", email: "agent@flow.test", password: "password123" },
      jar,
    });
    assert(signup.res.status === 200 || signup.res.status === 201, `signup failed: ${signup.res.status} ${JSON.stringify(signup.json)}`);
    const setters = jar.absorbResponse ?? null;
    const sessionCookieNames = [...jar.cookies.keys()].filter((n) => n.includes("session"));
    assert(sessionCookieNames.length > 0, `no session cookie set on signup; jar=${JSON.stringify([...jar.cookies.keys()])}`);

    const session1 = await req(`http://127.0.0.1:${PORT}/api/auth/get-session`, { jar });
    assert(session1.json?.user?.email === "agent@flow.test", "get-session did not return the signed-up user");

    const rawSetCookie = (await fetch(`http://127.0.0.1:${PORT}/api/auth/get-session`, { headers: { cookie: jar.header() } })).headers;
    void rawSetCookie;
    const loginRes = await fetch(`http://127.0.0.1:${PORT}/api/auth/sign-in/email`, {
      method: "POST",
      headers: { "content-type": "application/json", origin: `http://127.0.0.1:${PORT}` },
      body: JSON.stringify({ email: "owner@flow.test", password: "password123" }),
    });
    const setCookies = loginRes.headers.getSetCookie();
    const sessionCookie = setCookies.find((c) => c.includes("session") && c.toLowerCase().includes("httponly"));
    assert(sessionCookie !== undefined, `login set-cookie missing HttpOnly session cookie: ${JSON.stringify(setCookies)}`);
    assert(sessionCookie.toLowerCase().includes("samesite=lax"), "session cookie must be SameSite=Lax");
    assert(sessionCookie.toLowerCase().includes("secure"), "session cookie must be Secure when APP_URL is https");

    const ownerJar = new Jar();
    ownerJar.absorb(loginRes);
    const ownerSession = await req(`http://127.0.0.1:${PORT}/api/auth/get-session`, { jar: ownerJar });
    assert(ownerSession.json?.user?.email === "owner@flow.test", "owner login session mismatch");

    const tokenBefore = [...ownerJar.cookies.entries()].find(([name]) => name.includes("session_token"))?.[1];
    assert(tokenBefore !== undefined, "no session token captured");
    const logout = await req(`http://127.0.0.1:${PORT}/api/auth/sign-out`, { method: "POST", body: {}, jar: ownerJar });
    assert(logout.res.status === 200, `sign-out failed: ${logout.res.status}`);

    const postLogout = await fetch(`http://127.0.0.1:${PORT}/api/auth/get-session`, {
      headers: { cookie: `__Secure-better-auth.session_token=${tokenBefore}` },
    });
    const postLogoutJson = await postLogout.json().catch(() => null);
    assert(postLogoutJson === null || postLogoutJson?.user === null || postLogoutJson?.session === null, "old session token still valid after sign-out (revocation failed)");

    const badLogin = await req(`http://127.0.0.1:${PORT}/api/auth/sign-in/email`, {
      method: "POST",
      body: { email: "owner@flow.test", password: "wrong-password" },
    });
    assert(badLogin.res.status === 401 || badLogin.res.status === 400 || badLogin.res.status === 429, `unexpected bad login status ${badLogin.res.status}`);
  } finally {
    await app.stop();
  }
});

ok("auth flow verification passed");
