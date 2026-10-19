// SPDX-License-Identifier: AGPL-3.0-only
// Node-1.2: the full unit and integration suite passes on the joined knowledge
// pipeline.
import { mustRun, ok } from "./lib/verify-kit.mjs";

mustRun("pnpm", ["test"], { timeout: 600_000 });
mustRun("pnpm", ["typecheck"], { timeout: 600_000 });
mustRun("pnpm", ["lint"], { timeout: 600_000 });

ok("regression verification passed");
