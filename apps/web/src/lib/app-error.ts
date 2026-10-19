// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";
import { APP_ERROR_CODES, type AppErrorCode } from "@chastedesk/shared";

export const jsonBody = z.object({}).passthrough();

export class AppError extends Error {
  constructor(
    public code: AppErrorCode,
    message: string,
    public httpStatus: number,
  ) {
    super(message);
  }
}

export function errorResponse(err: unknown) {
  if (err instanceof AppError) {
    return Response.json({ error: { code: err.code, message: err.message } }, { status: err.httpStatus });
  }
  if (err instanceof z.ZodError) {
    const zodError = err as z.ZodError;
    return Response.json(
      { error: { code: "bad_request", message: zodError.issues.map((issue) => issue.message).join("; ") } },
      { status: 400 },
    );
  }
  console.error("unhandled route error", err, err instanceof Error ? err.stack?.slice(0, 400) : "");
  if (err instanceof Error && err.cause) console.error("cause:", String(err.cause).slice(0, 500));
  return Response.json(
    { error: { code: "internal", message: "internal error" } },
    { status: 500 },
  );
}

export function okResponse(data: unknown, status = 200) {
  return Response.json({ data }, { status });
}

export const codes = APP_ERROR_CODES;
