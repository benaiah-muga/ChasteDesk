// SPDX-License-Identifier: AGPL-3.0-only
// Job registry: workers register handlers here; jobs are named in packages/shared constants.
const handlers = new Map<string, (data: unknown) => Promise<void>>();

export function registerJob(name: string, handler: (data: unknown) => Promise<void>) {
  handlers.set(name, handler);
}

export function getHandlers() {
  return handlers;
}

export async function startWorker() {
  const { getHandlers } = await import("./registry");
  const all = getHandlers();
  console.log(`worker started with ${all.size} job handlers:`, [...all.keys()].join(", "));
  // pg-boss wiring lands with the ingestion leaf; registry exists so jobs can register from boot.
}
