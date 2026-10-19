// SPDX-License-Identifier: AGPL-3.0-only
package retrieval

import (
	"reflect"
	"strings"
	"testing"
)

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestChunkMarkdownFixture(t *testing.T) {
	runChunker := func() []Chunk {
		return ChunkMarkdown("src-fixture", 1, buildFixtureDoc())
	}
	chunks := runChunker()
	if len(chunks) < 3 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	totalInputTokens := EstimateTokens(buildFixtureDoc())
	totalChunkTokens := 0
	for _, chunk := range chunks {
		totalChunkTokens += chunk.TokenCount
	}
	if float64(totalChunkTokens) < float64(totalInputTokens)*0.85 {
		t.Fatalf("chunk coverage too low: %d of %d tokens", totalChunkTokens, totalInputTokens)
	}

	for _, chunk := range chunks {
		if chunk.TokenCount > 800 {
			t.Errorf("chunk %d exceeds 800 tokens (%d)", chunk.ChunkIndex, chunk.TokenCount)
		}
		if chunk.TokenCount < 200 {
			t.Errorf("sliver chunk %d under 200 tokens (%d)", chunk.ChunkIndex, chunk.TokenCount)
		}
		if len(chunk.Locator.Headings) == 0 {
			t.Errorf("chunk %d missing heading breadcrumb", chunk.ChunkIndex)
		}
	}

	withTable := []Chunk{}
	for _, chunk := range chunks {
		if strings.Contains(chunk.Content, "| Plan 13 |") {
			withTable = append(withTable, chunk)
		}
	}
	if len(withTable) != 1 {
		t.Fatalf("table must be preserved inside exactly one chunk, found %d", len(withTable))
	}
	if !strings.Contains(withTable[0].Content, "| Plan 1 |") || !strings.Contains(withTable[0].Content, "| Plan 25 |") {
		t.Error("table chunk is missing table rows")
	}

	var sectionB *Chunk
	for i := range chunks {
		if containsString(chunks[i].Locator.Headings, "Section B") {
			sectionB = &chunks[i]
			break
		}
	}
	if sectionB == nil {
		t.Fatal("no chunk carries the Section B breadcrumb")
	}
	if sectionB.Locator.Headings[0] != "Guide" {
		t.Fatalf("breadcrumb root must be the document title, got %q", sectionB.Locator.Headings[0])
	}

	again := runChunker()
	if !reflect.DeepEqual(again, chunks) {
		t.Error("chunking is not deterministic")
	}
	seen := map[string]bool{}
	for _, chunk := range chunks {
		if seen[chunk.ID] {
			t.Errorf("duplicate chunk id %s", chunk.ID)
		}
		seen[chunk.ID] = true
	}
}

func TestChunkMarkdownHardSplit(t *testing.T) {
	giant := strings.Repeat("word ", 20000)
	giantChunks := ChunkMarkdown("src-giant", 1, giant)
	if len(giantChunks) <= 1 {
		t.Fatal("oversized single block must be hard-split")
	}
	for _, chunk := range giantChunks {
		if chunk.TokenCount > 800 {
			t.Errorf("hard-split chunk exceeds 800 tokens (%d)", chunk.TokenCount)
		}
	}
	if !strings.Contains(giantChunks[0].Content, "word") {
		t.Error("hard-split lost content")
	}
	coverage := 0
	for _, chunk := range giantChunks {
		coverage += chunk.TokenCount
	}
	if float64(coverage) < float64(EstimateTokens(giant))*0.9 {
		t.Error("hard-split dropped most content")
	}
}
