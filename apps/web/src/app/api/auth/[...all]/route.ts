// SPDX-License-Identifier: AGPL-3.0-only
import { connection } from "next/server";

import { auth } from "@/lib/auth";

async function handle(req: Request) {
  await connection();
  return auth.handler(req);
}

export async function GET(req: Request) {
  return handle(req);
}

export async function POST(req: Request) {
  return handle(req);
}
