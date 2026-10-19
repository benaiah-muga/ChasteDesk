import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: [
      "packages/*/src/**/*.test.ts",
      "apps/*/src/**/*.test.ts",
      "apps/worker/src/**/*.test.ts",
    ],
    environment: "node",
    hookTimeout: 120_000,
    testTimeout: 120_000,
  },
});
