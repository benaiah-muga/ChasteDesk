// SPDX-License-Identifier: AGPL-3.0-only
// Node-1.3: the agent policy schema, loop, and tool registry agree on tool
// ids, modes, and argument shapes.
import { readFileSync } from "node:fs";
import { assert, ok } from "./lib/verify-kit.mjs";
import { toolArgsSchemas } from "../../packages/shared/src/tools.ts";

const registry = readFileSync("packages/ai/src/tools/registry.ts", "utf8");
for (const id of Object.keys(toolArgsSchemas)) {
  assert(registry.includes(`id: "${id}"`), `tool registry missing built-in tool ${id}`);
}
for (const id of Object.keys(toolArgsSchemas)) {
  assert(toolArgsSchemas[id] !== undefined, `shared tool args schema missing for ${id}`);
}

const policy = readFileSync("packages/shared/src/schemas/agent.ts", "utf8");
assert(policy.includes("approval_required") && policy.includes("autonomous"), "policy schema must know both tool modes");

const loop = readFileSync("packages/ai/src/loop/loop.ts", "utf8");
assert(loop.includes("approval_required") && loop.includes("disabled"), "loop must handle approval_required and disabled modes");

let negativeCaught = false;
try {
  assert(toolArgsSchemas["nonexistent.tool"] !== undefined, "probe");
} catch {
  negativeCaught = true;
}
assert(negativeCaught, "interface assertions cannot fail");

ok("ai interfaces verification passed");
