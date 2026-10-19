// SPDX-License-Identifier: AGPL-3.0-only
export interface ChunkLocator {
  headings: string[];
  anchor?: string;
  charRange: [number, number];
}

export interface Evidence {
  chunkId: string;
  sourceId: string;
  sourceName: string;
  version: number;
  scope: "public" | "internal";
  locator: ChunkLocator;
  content: string;
  score: number;
  confidence: number;
}

export interface SearchQuery {
  text: string;
  scopes: Array<"public" | "internal">;
  limit?: number;
  embeddingModel?: string;
  queryEmbedding?: number[];
}

export interface RetrievalEngine {
  id: "builtin";
  search(orgId: string, q: SearchQuery): Promise<Evidence[]>;
}

export interface PgClientLike {
  query: (sqlText: string, params?: unknown[]) => Promise<{ rows: Record<string, unknown>[] }>;
}
