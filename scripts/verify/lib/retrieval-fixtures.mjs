// SPDX-License-Identifier: AGPL-3.0-only
// Deterministic corpus + concept embeddings for retrieval gates.
import { randomBytes } from "node:crypto";

export const EMBED_MODEL = "test-embed-v1";
export const CONCEPTS = ["access", "billing", "api", "setup"];

const LEXICON = {
  access: ["password", "credential", "login", "reset", "secret", "phrase", "sign in", "signin", "security"],
  billing: ["invoice", "refund", "billing", "plan", "payment", "pricing", "subscription"],
  api: ["api", "endpoint", "request", "webhook", "token", "payload"],
  setup: ["install", "setup", "getting started", "onboarding", "deploy", "configure"],
};

export function embed(text) {
  const t = text.toLowerCase();
  const counts = { access: 0, billing: 0, api: 0, setup: 0 };
  for (const [concept, words] of Object.entries(LEXICON)) {
    for (const w of words) {
      counts[concept] += t.split(w).length - 1;
    }
  }
  const total = Object.values(counts).reduce((a, b) => a + b, 0) || 1;
  return CONCEPTS.map((c) => counts[c] / total);
}

export function cosine(a, b) {
  const normA = Math.sqrt(a.reduce((s, v) => s + v * v, 0));
  const normB = Math.sqrt(b.reduce((s, v) => s + v * v, 0));
  if (normA === 0 || normB === 0) return 0;
  let dot = 0;
  for (let i = 0; i < a.length; i++) dot += a[i] * b[i];
  return dot / (normA * normB);
}

export function fillSentence(concept, seed, length = 40) {
  const fillerWords = ["lorem", "ipsum", "dolor", "amet", "kale", "provision", " Account", "region", "tenant", "notice"];
  const words = [];
  let state = seed;
  for (let i = 0; i < length; i++) {
    state = (state * 1103515245 + 12345) % 2147483648;
    words.push(fillerWords[state % fillerWords.length]);
  }
  return `${concept} note: ${words.join(" ")}.`;
}

export function buildFixtureDoc() {
  const section = (title, seed) => {
    const lines = [];
    for (let i = 0; i < 7; i++) {
      lines.push(fillSentence(`${title} detail`, seed + i, 55));
    }
    return lines.join("\n\n");
  };
  const tableRows = [];
  for (let i = 0; i < 25; i++) {
    tableRows.push(`| Plan ${i + 1} | feature ${i + 1} | limit ${i * 10 + 5} | price ${i + 1}9 |`);
  }
  return [
    "# Guide",
    "This guide explains the workspace basics for administrators and members.",
    "## Section A",
    section("alpha", 7),
    "## Section B",
    section("beta", 91),
    "## Plans",
    "| Plan | Feature | Limit | Price |",
    "| --- | --- | --- | --- |",
    ...tableRows,
    "## Section C",
    section("gamma", 233),
  ].join("\n");
}

export const SOURCES = {
  para: "00000000-0000-4000-8000-000000000001",
  fillers: "00000000-0000-4000-8000-000000000002",
  lexical: "00000000-0000-4000-8000-000000000003",
  unrelated: "00000000-0000-4000-8000-000000000004",
  internal: "00000000-0000-4000-8000-000000000005",
  modelB: "00000000-0000-4000-8000-000000000006",
};

export async function seedCorpus(client, { orgId = "org-retrieval" } = {}) {
  await client.query("insert into organization (id, name, slug) values ($1, 'Retrieval Org', 'retrieval-org')", [orgId]);
  const sources = [
    { id: SOURCES.para, name: "Password Help", scope: "public" },
    { id: SOURCES.fillers, name: "Login Guides", scope: "public" },
    { id: SOURCES.lexical, name: "API Reference", scope: "public" },
    { id: SOURCES.unrelated, name: "Unrelated Docs", scope: "public" },
    { id: SOURCES.internal, name: "Internal Runbook", scope: "internal" },
    { id: SOURCES.modelB, name: "Model B Notes", scope: "public" },
  ];
  for (const s of sources) {
    await client.query(
      `insert into knowledge_sources (id, org_id, type, name, scope, status, version)
       values ($1, $2, 'text', $3, $4, 'ready', 3)`,
      [s.id, orgId, s.name, s.scope],
    );
  }
  return orgId;
}

export async function insertChunk(client, { orgId, sourceId, content, embeddingModel = EMBED_MODEL, version = 3 }) {
  const id = randomBytes(12).toString("hex");
  const vec = embed(content);
  const embeddingLiteral = embeddingModel ? `[${vec.join(",")}]` : null;
  await client.query(
    `insert into knowledge_chunks (id, org_id, source_id, version, chunk_index, content, locator, token_count, embedding_model, embedding)
     values ($1, $2, $3, $4, 0, $5, $6::jsonb, $7, $8, $9::vector)`,
    [
      id,
      orgId,
      sourceId,
      version,
      content,
      JSON.stringify({ headings: ["Doc"], charRange: [0, content.length] }),
      Math.ceil(content.length / 4),
      embeddingModel,
      embeddingLiteral,
    ],
  );
  return { id, vec };
}

export const PARAPHRASE_CONTENT =
  "qsigphrase credential. Forgot your secret phrase? Open the security settings page and choose a new credential. You can sign in afterwards with the new secret phrase.";
export const LEXICAL_CONTENT =
  "The password field of the API request body must be at least 8 characters. The API rejects password values shorter than 8 characters. Password handling in the API request validation rules.";
export const INTERNAL_CONTENT =
  "Internal runbook: when a customer forgets their secret phrase, verify identity, then reset the credential from the admin security console.";
