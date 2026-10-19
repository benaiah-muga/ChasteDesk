// SPDX-License-Identifier: AGPL-3.0-only
package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	RRFConstant           = 60
	ConfidenceDenominator = 0.02
)

type ChunkLocator struct {
	Headings  []string `json:"headings"`
	Anchor    string   `json:"anchor,omitempty"`
	CharRange [2]int   `json:"charRange"`
}

type Evidence struct {
	ChunkID    string       `json:"chunkId"`
	SourceID   string       `json:"sourceId"`
	SourceName string       `json:"sourceName"`
	Version    int          `json:"version"`
	Scope      string       `json:"scope"`
	Locator    ChunkLocator `json:"locator"`
	Content    string       `json:"content"`
	Score      float64      `json:"score"`
	Confidence float64      `json:"confidence"`
}

type SearchQuery struct {
	Text           string
	Scopes         []string
	Limit          int
	EmbeddingModel string
	QueryEmbedding []float64
}

type Engine struct {
	pool *pgxpool.Pool
}

func NewEngine(pool *pgxpool.Pool) *Engine {
	return &Engine{pool: pool}
}

type rawRow struct {
	chunkID    string
	sourceID   string
	sourceName string
	version    int
	scope      string
	locator    []byte
	content    string
	score      float64
}

func clamp01(n float64) float64 {
	if n < 0 {
		return 0
	}
	if n > 1 {
		return 1
	}
	return n
}

func (e *Engine) toEvidence(row rawRow, score float64) (Evidence, error) {
	var locator ChunkLocator
	if err := json.Unmarshal(row.locator, &locator); err != nil {
		return Evidence{}, fmt.Errorf("retrieval: decode locator: %w", err)
	}
	scope := "public"
	if row.scope == "internal" {
		scope = "internal"
	}
	return Evidence{
		ChunkID:    row.chunkID,
		SourceID:   row.sourceID,
		SourceName: row.sourceName,
		Version:    row.version,
		Scope:      scope,
		Locator:    locator,
		Content:    row.content,
		Score:      score,
		Confidence: clamp01(score / (score + ConfidenceDenominator)),
	}, nil
}

func (e *Engine) Search(ctx context.Context, orgID string, q SearchQuery) ([]Evidence, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 8
	}
	scopes := q.Scopes
	if len(scopes) == 0 {
		scopes = []string{"public"}
	}
	useVector := len(q.QueryEmbedding) > 0 && q.EmbeddingModel != ""

	if !useVector {
		rows, err := e.ftsSearch(ctx, orgID, q.Text, scopes, limit)
		if err != nil {
			return nil, err
		}
		out := make([]Evidence, 0, len(rows))
		for _, row := range rows {
			evidence, err := e.toEvidence(row, row.score)
			if err != nil {
				return nil, err
			}
			out = append(out, evidence)
		}
		return out, nil
	}

	ftsRows, err := e.ftsSearch(ctx, orgID, q.Text, scopes, 50)
	if err != nil {
		return nil, err
	}
	vecRows, err := e.vectorSearch(ctx, orgID, q.QueryEmbedding, q.EmbeddingModel, scopes, 50)
	if err != nil {
		return nil, err
	}

	type scored struct {
		row   rawRow
		score float64
	}
	scores := map[string]*scored{}
	order := []string{}
	ftsMatch := map[string]bool{}
	vecMatch := map[string]bool{}
	addRank := func(rows []rawRow, match map[string]bool) {
		for i, row := range rows {
			match[row.chunkID] = true
			increment := 1.0 / float64(RRFConstant+i+1)
			if existing, ok := scores[row.chunkID]; ok {
				existing.score += increment
			} else {
				scores[row.chunkID] = &scored{row: row, score: increment}
				order = append(order, row.chunkID)
			}
		}
	}
	addRank(ftsRows, ftsMatch)
	addRank(vecRows, vecMatch)

	ordered := make([]*scored, 0, len(order))
	for _, id := range order {
		if ftsMatch[id] || vecMatch[id] {
			ordered = append(ordered, scores[id])
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].score > ordered[j].score })
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}
	out := make([]Evidence, 0, len(ordered))
	for _, entry := range ordered {
		evidence, err := e.toEvidence(entry.row, entry.score)
		if err != nil {
			return nil, err
		}
		out = append(out, evidence)
	}
	return out, nil
}

func splitTerms(text string) []string {
	raw := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_'
	})
	terms := []string{}
	for _, term := range raw {
		if len(term) < 2 {
			continue
		}
		terms = append(terms, strings.ReplaceAll(term, "'", "")+":*")
		if len(terms) == 24 {
			break
		}
	}
	return terms
}

func (e *Engine) ftsSearch(ctx context.Context, orgID, text string, scopes []string, limit int) ([]rawRow, error) {
	terms := splitTerms(text)
	if len(terms) == 0 {
		return nil, nil
	}
	tsquery := strings.Join(terms, " | ")
	rows, err := e.pool.Query(ctx, `select c.id, c.source_id, s.name as source_name, c.version, s.scope::text, c.locator, c.content,
			ts_rank_cd(c.tsv, query) as score
		from knowledge_chunks c
		join knowledge_sources s on s.id = c.source_id
		cross join to_tsquery('english', $2) query
		where c.org_id = $1 and s.scope = any($3) and c.tsv @@ query
		order by score desc
		limit $4`, orgID, tsquery, scopes, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rawRow{}
	for rows.Next() {
		var row rawRow
		if err := rows.Scan(&row.chunkID, &row.sourceID, &row.sourceName, &row.version, &row.scope, &row.locator, &row.content, &row.score); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func vectorLiteral(vec []float64) string {
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func (e *Engine) vectorSearch(ctx context.Context, orgID string, embedding []float64, model string, scopes []string, limit int) ([]rawRow, error) {
	rows, err := e.pool.Query(ctx, `select c.id, c.source_id, s.name as source_name, c.version, s.scope::text, c.locator, c.content,
			1 - (c.embedding <=> $2::vector) as score
		from knowledge_chunks c
		join knowledge_sources s on s.id = c.source_id
		where c.org_id = $1 and s.scope = any($3)
			and c.embedding_model = $4 and c.embedding is not null
		order by c.embedding <=> $2::vector
		limit $5`, orgID, vectorLiteral(embedding), scopes, model, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rawRow{}
	for rows.Next() {
		var row rawRow
		if err := rows.Scan(&row.chunkID, &row.sourceID, &row.sourceName, &row.version, &row.scope, &row.locator, &row.content, &row.score); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type StaleSource struct {
	SourceID       string  `json:"sourceId"`
	EmbeddingModel *string `json:"embeddingModel"`
}

func (e *Engine) SourcesNeedingReindex(ctx context.Context, orgID, currentEmbeddingModel string) ([]StaleSource, error) {
	rows, err := e.pool.Query(ctx, `select distinct s.id, s.embedding_model
		from knowledge_sources s
		where s.org_id = $1 and s.status = 'ready'
			and (s.embedding_model is null or s.embedding_model <> $2)`, orgID, currentEmbeddingModel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StaleSource{}
	for rows.Next() {
		var stale StaleSource
		if err := rows.Scan(&stale.SourceID, &stale.EmbeddingModel); err != nil {
			return nil, err
		}
		out = append(out, stale)
	}
	return out, rows.Err()
}
