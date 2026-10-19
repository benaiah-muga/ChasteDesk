// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"strconv"
	"strings"
)

func ExtractPDFText(raw []byte) (string, error) {
	var text strings.Builder
	foundStream := false
	cursor := 0
	for {
		idx := bytes.Index(raw[cursor:], []byte("stream"))
		if idx < 0 {
			break
		}
		streamStart := cursor + idx
		if streamStart > 0 && raw[streamStart-1] == 'd' {
			cursor = streamStart + len("stream")
			continue
		}
		start := streamStart + len("stream")
		if start < len(raw) && raw[start] == '\r' {
			start++
		}
		if start < len(raw) && raw[start] == '\n' {
			start++
		}
		end := bytes.Index(raw[start:], []byte("endstream"))
		if end < 0 {
			break
		}
		data := raw[start : start+end]
		if dictStart := bytes.LastIndex(raw[:streamStart], []byte("<<")); dictStart >= 0 {
			if bytes.Contains(raw[dictStart:streamStart], []byte("/FlateDecode")) {
				if zr, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
					if decoded, err := io.ReadAll(io.LimitReader(zr, 8*1024*1024)); err == nil {
						data = decoded
					}
					_ = zr.Close()
				}
			}
		}
		for _, literal := range pdfStringLiterals(data) {
			text.WriteString(literal)
			text.WriteString("\n")
		}
		foundStream = true
		cursor = start + end + len("endstream")
	}
	if !foundStream {
		return "", errors.New("pdf: no extractable text")
	}
	out := strings.TrimSpace(text.String())
	if out == "" {
		return "", errors.New("pdf: no extractable text")
	}
	return out, nil
}

func pdfStringLiterals(content []byte) []string {
	out := []string{}
	i := 0
	for i < len(content) {
		if content[i] != '(' {
			i++
			continue
		}
		depth := 1
		i++
		var sb []byte
		for i < len(content) && depth > 0 {
			c := content[i]
			if c == '\\' && i+1 < len(content) {
				next := content[i+1]
				switch next {
				case 'n':
					sb = append(sb, '\n')
					i += 2
				case 'r':
					sb = append(sb, '\r')
					i += 2
				case 't':
					sb = append(sb, '\t')
					i += 2
				case '(', ')', '\\':
					sb = append(sb, next)
					i += 2
				default:
					if next >= '0' && next <= '7' {
						end := i + 1
						for end < len(content) && end < i+4 && content[end] >= '0' && content[end] <= '7' {
							end++
						}
						if value, err := strconv.ParseInt(string(content[i+1:end]), 8, 32); err == nil {
							sb = append(sb, byte(value))
						}
						i = end
					} else {
						sb = append(sb, next)
						i += 2
					}
				}
				continue
			}
			if c == '(' {
				depth++
			}
			if c == ')' {
				depth--
				i++
				if depth == 0 {
					break
				}
				sb = append(sb, c)
				continue
			}
			sb = append(sb, c)
			i++
		}
		out = append(out, string(sb))
	}
	return out
}
