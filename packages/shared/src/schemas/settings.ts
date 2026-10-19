// SPDX-License-Identifier: AGPL-3.0-only
import { z } from "zod";

export const providerIds = ["openai", "anthropic", "google", "ollama", "openai-compatible"] as const;
export const providerIdSchema = z.enum(providerIds);
export type ProviderId = (typeof providerIds)[number];

export const providerConfigInput = z.object({
  providerId: providerIdSchema,
  displayName: z.string().min(1).max(100),
  apiKey: z.string().max(500).optional(),
  baseUrl: z.string().url().max(500).optional(),
});

export const widgetConfigInput = z.object({
  origins: z.array(z.string().url().max(200)).max(20),
  theme: z
    .object({
      primaryColor: z.string().max(20).optional(),
      displayName: z.string().max(60).optional(),
    })
    .optional(),
});
