// SPDX-License-Identifier: AGPL-3.0-only
package retrieval

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const (
	targetMinTokens = 500
	targetMaxTokens = 800
	overlapTokens   = 80
)

type Chunk struct {
	ID         string       `json:"id"`
	SourceID   string       `json:"sourceId"`
	Version    int          `json:"version"`
	ChunkIndex int          `json:"chunkIndex"`
	Content    string       `json:"content"`
	Locator    ChunkLocator `json:"locator"`
	TokenCount int          `json:"tokenCount"`
}

func EstimateTokens(text string) int {
	return (len(text) + 3) / 4
}

func ChunkID(sourceID string, version, index int) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s:%d:%d", sourceID, version, index)))
	return hex.EncodeToString(sum[:])
}

type block struct {
	text     string
	isTable  bool
	headings []string
	start    int
	end      int
}

var headingRe = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)

func isTableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "" || strings.HasPrefix(trimmed, "|")
}

func parseBlocks(markdown string) []block {
	lines := strings.Split(markdown, "\n")
	blocks := []block{}
	headings := []string{}
	charCursor := 0
	buffer := []string{}
	bufferStart := 0
	bufferHeadings := append([]string{}, headings...)

	flush := func() {
		if len(buffer) == 0 {
			return
		}
		text := strings.Join(buffer, "\n")
		isTable := true
		for _, line := range buffer {
			if !isTableLine(line) {
				isTable = false
				break
			}
		}
		blocks = append(blocks, block{
			text:     text,
			isTable:  isTable,
			headings: append([]string{}, bufferHeadings...),
			start:    bufferStart,
			end:      bufferStart + len(text),
		})
		buffer = []string{}
	}

	for _, line := range lines {
		headingMatch := headingRe.FindStringSubmatch(line)
		if headingMatch != nil && headingMatch[2] != "" {
			flush()
			level := len(headingMatch[1])
			if level-1 <= len(headings) {
				headings = headings[:level-1]
			} else {
				for len(headings) < level-1 {
					headings = append(headings, "")
				}
			}
			headings = append(headings, strings.TrimSpace(headingMatch[2]))
			bufferHeadings = append([]string{}, headings...)
			charCursor += len(line) + 1
			continue
		}
		startingTable := len(buffer) > 0
		if startingTable {
			for _, buffered := range buffer {
				if !isTableLine(buffered) {
					startingTable = false
					break
				}
			}
		}
		lineIsTable := strings.HasPrefix(strings.TrimSpace(line), "|")
		if len(buffer) > 0 && !startingTable && lineIsTable {
			flush()
			bufferHeadings = append([]string{}, headings...)
		}
		if len(buffer) == 0 {
			bufferStart = charCursor
			bufferHeadings = append([]string{}, headings...)
		}
		buffer = append(buffer, line)
		charCursor += len(line) + 1
		if strings.TrimSpace(line) == "" && hasNonBlank(buffer) {
			flush()
		}
	}
	flush()
	out := make([]block, 0, len(blocks))
	for _, b := range blocks {
		if strings.TrimSpace(b.text) != "" {
			out = append(out, b)
		}
	}
	return out
}

func hasNonBlank(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}

func hardSplit(text string, maxChars, overlapChars int) []string {
	if len(text) <= maxChars {
		return []string{text}
	}
	pieces := []string{}
	cursor := 0
	for cursor < len(text) {
		end := cursor + maxChars
		if end > len(text) {
			end = len(text)
		}
		if end < len(text) {
			if lastBreak := strings.LastIndex(text[:end+1], "\n"); lastBreak > cursor+maxChars/2 {
				end = lastBreak
			}
		}
		pieces = append(pieces, text[cursor:end])
		if end >= len(text) {
			break
		}
		if end-overlapChars > cursor {
			cursor = end - overlapChars
		} else {
			cursor = end
		}
	}
	return pieces
}

type carryState struct {
	text     string
	headings []string
	start    int
	end      int
}

func ChunkMarkdown(sourceID string, version int, markdown string) []Chunk {
	blocks := parseBlocks(markdown)
	chunks := []Chunk{}
	index := 0
	var carry *carryState

	emit := func(text string, headings []string, start, end int) {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return
		}
		chunks = append(chunks, Chunk{
			ID:         ChunkID(sourceID, version, index),
			SourceID:   sourceID,
			Version:    version,
			ChunkIndex: index,
			Content:    trimmed,
			Locator:    ChunkLocator{Headings: headings, CharRange: [2]int{start, end}},
			TokenCount: EstimateTokens(trimmed),
		})
		index++
	}

	for _, b := range blocks {
		if b.isTable {
			tableTokens := EstimateTokens(b.text)
			if carry != nil {
				tableTokens += EstimateTokens(carry.text)
			}
			if tableTokens <= targetMaxTokens {
				if carry != nil {
					carry = &carryState{text: carry.text + "\n\n" + b.text, headings: b.headings, start: carry.start, end: b.end}
				} else {
					carry = &carryState{text: b.text, headings: b.headings, start: b.start, end: b.end}
				}
				continue
			}
			if carry != nil {
				emit(carry.text, carry.headings, carry.start, carry.end)
				carry = nil
			}
			for _, piece := range hardSplit(b.text, targetMaxTokens*4, overlapTokens*4) {
				emit(piece, b.headings, b.start, b.end)
			}
			continue
		}

		pieces := hardSplit(b.text, targetMaxTokens*4, overlapTokens*4)
		if carry != nil && len(pieces) > 0 && EstimateTokens(carry.text)+EstimateTokens(pieces[0]) <= targetMaxTokens {
			pieces = append([]string{carry.text + "\n\n" + pieces[0]}, pieces[1:]...)
			carry = nil
		} else if carry != nil {
			emit(carry.text, carry.headings, carry.start, carry.end)
			carry = nil
		}

		for i, piece := range pieces {
			size := EstimateTokens(piece)
			isLast := i == len(pieces)-1
			if isLast && size < targetMinTokens {
				carry = &carryState{text: piece, headings: b.headings, start: b.start, end: b.end}
			} else {
				emit(piece, b.headings, b.start, b.end)
			}
		}
	}
	if carry != nil {
		emit(carry.text, carry.headings, carry.start, carry.end)
	}
	return chunks
}

func ChunkPlainText(sourceID string, version int, text string) []Chunk {
	return ChunkMarkdown(sourceID, version, text)
}

type ParsedDocument struct {
	Title    string
	Markdown string
}

func ChunkDocument(doc ParsedDocument, sourceID string, version int) []Chunk {
	return ChunkMarkdown(sourceID, version, "# "+doc.Title+"\n\n"+doc.Markdown)
}
