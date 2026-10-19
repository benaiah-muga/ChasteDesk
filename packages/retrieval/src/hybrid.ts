// SPDX-License-Identifier: AGPL-3.0-only
import type { ChunkLocator, Evidence, PgClientLike, RetrievalEngine, SearchQuery } from "./port.ts";

const RRF_K = 60;
const CONFIDENCE_DENOMINATOR = 0.02;

function toVectorLiteral(vec: number[]): string {
  return `[${vec.join(",")}]`;
}

function clamp01(n: number): number {
  return Math.max(0, Math.min(1, n));
}

interface RawRow {
  id: string;
  source_id: string;
  source_name: string;
  version: number;
  scope: string;
  locator: ChunkLocator;
  content: string;
  score: number;
}

export class BuiltinEngine implements RetrievalEngine {
  readonly id = "builtin" as const;
  private readonly client: PgClientLike;

  constructor(client: PgClientLike) {
    this.client = client;
  }

  async search(orgId: string, q: SearchQuery): Promise<Evidence[]> {
    const limit = q.limit ?? 8;
    const scopes = q.scopes.length > 0 ? q.scopes : (["public"] as const);
    const embedding = q.queryEmbedding;
    const model = q.embeddingModel;
    const useVector = embedding !== undefined && model !== undefined && embedding.length > 0;

    if (!useVector || !embedding || !model) {
      const ftsRows = await this.ftsSearch(orgId, q.text, scopes, limit);
      return ftsRows.map((r) => this.toEvidence(r, r.score));
    }

    const [ftsRows, vecRows] = await Promise.all([
      this.ftsSearch(orgId, q.text, scopes, 50),
      this.vectorSearch(orgId, embedding, model, scopes, 50),
    ]);

    const scores = new Map<string, { row: RawRow; score: number }>();
    const addRank = (rows: RawRow[], map: Map<string, number>) => {
      rows.forEach((row, i) => {
        if (row.score > 0) map.set(row.id, 1);
        const existing = scores.get(row.id);
        if (existing) {
          existing.score += 1 / (RRF_K + i + 1);
        } else {
          scores.set(row.id, { row, score: 1 / (RRF_K + i + 1) });
        }
      });
    };
    const ftsMatch = new Map<string, number>();
    const vecMatch = new Map<string, number>();
    addRank(ftsRows, ftsMatch);
    addRank(vecRows, vecMatch);

    return [...scores.values()]
      .filter(({ row }) => ftsMatch.has(row.id) || vecMatch.has(row.id))
      .sort((a, b) => b.score - a.score)
      .slice(0, limit)
      .map(({ row, score }) => this.toEvidence(row, score));
  }

  private toEvidence(row: RawRow, score: number): Evidence {
    return {
      chunkId: row.id,
      sourceId: row.source_id,
      sourceName: row.source_name,
      version: row.version,
      scope: row.scope === "internal" ? "internal" : "public",
      locator: row.locator,
      content: row.content,
      score,
      confidence: clamp01(score / (score + CONFIDENCE_DENOMINATOR)),
    };
  }

  private async ftsSearch(
    orgId: string,
    text: string,
    scopes: ReadonlyArray<"public" | "internal">,
    limit: number,
  ): Promise<RawRow[]> {
    const scopeList = scopes as string[];
    const terms = text
      .toLowerCase()
      .split(/[^a-z0-9_]+/)
      .filter((w) => w.length >= 2)
      .slice(0, 24);
    if (terms.length === 0) return [];
    const tsquery = terms.map((w) => w.replace(/'/g, "") + ":*").join(" | ");
    const result = await this.client.query(
      `SELECT c.id, c.source_id, s.name AS source_name, c.version, s.scope, c.locator, c.content,
              ts_rank_cd(c.tsv, query) AS score
       FROM knowledge_chunks c
       JOIN knowledge_sources s ON s.id = c.source_id
       CROSS JOIN to_tsquery('english', $2) query
       WHERE c.org_id = $1 AND s.scope = ANY($3) AND c.tsv @@ query
       ORDER BY score DESC
       LIMIT $4`,
      [orgId, tsquery, scopeList, limit],
    );
    return result.rows as unknown as RawRow[];
  }

  private async vectorSearch(
    orgId: string,
    embedding: number[],
    model: string,
    scopes: ReadonlyArray<"public" | "internal">,
    limit: number,
  ): Promise<RawRow[]> {
    const scopeList = scopes as string[];
    const result = await this.client.query(
      `SELECT c.id, c.source_id, s.name AS source_name, c.version, s.scope, c.locator, c.content,
              1 - (c.embedding <=> $2::vector) AS score
       FROM knowledge_chunks c
       JOIN knowledge_sources s ON s.id = c.source_id
       WHERE c.org_id = $1 AND s.scope = ANY($3)
         AND c.embedding_model = $4 AND c.embedding IS NOT NULL
       ORDER BY c.embedding <=> $2::vector
       LIMIT $5`,
      [orgId, toVectorLiteral(embedding), scopeList, model, limit],
    );
    return result.rows as unknown as RawRow[];
  }

  async sourcesNeedingReindex(
    orgId: string,
    currentEmbeddingModel: string,
  ): Promise<Array<{ sourceId: string; embeddingModel: string | null }>> {
    const result = await this.client.query(
      `SELECT DISTINCT s.id AS source_id, s.embedding_model
       FROM knowledge_sources s
       WHERE s.org_id = $1 AND s.status = 'ready'
         AND (s.embedding_model IS NULL OR s.embedding_model <> $2)`,
      [orgId, currentEmbeddingModel],
    );
    return result.rows as unknown as Array<{ sourceId: string; embeddingModel: string | null }>;
  }
}

export function createBuiltinEngine(client: PgClientLike): BuiltinEngine {
  return new BuiltinEngine(client);
}

// Chunking a parsed document into writable chunk rows, reusing the chunker.
import { chunkMarkdown } from "./chunker.ts";

export interface ParsedDocumentLike {
  title: string;
  markdown: string;
}

export interface ChunkWrite {
  id: string;
  sourceId: string;
  version: number;
  chunkIndex: number;
  content: string;
  locator: ChunkLocator;
  tokenCount: number;
}

export function chunkDocument(doc: ParsedDocumentLike, sourceId: string, version: number): ChunkWrite[] {
  const withTitle = `# ${doc.title}\n\n${doc.markdown}`;
  return chunkMarkdown(sourceId, version, withTitle).map((c) => ({
    id: c.id,
    sourceId: c.sourceId,
    version: c.version,
    chunkIndex: c.chunkIndex,
    content: c.content,
    locator: c.locator,
    tokenCount: c.tokenCount,
  }));
}
