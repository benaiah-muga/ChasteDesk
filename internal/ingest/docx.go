// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"
)

var docxTokenRe = regexp.MustCompile(`(?s)<w:t[^>]*>(.*?)</w:t>|</w:p>`)

var xmlEntities = strings.NewReplacer(
	"&amp;", "&",
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", `"`,
	"&apos;", "'",
	"&#39;", "'",
)

func ExtractDocxText(raw []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", errors.New("docx: invalid archive")
	}
	var document *zip.File
	for _, file := range zr.File {
		if file.Name == "word/document.xml" {
			document = file
			break
		}
	}
	if document == nil {
		return "", errors.New("docx: missing word/document.xml")
	}
	rc, err := document.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	body, err := io.ReadAll(io.LimitReader(rc, maxFileBytes))
	if err != nil {
		return "", err
	}
	var text strings.Builder
	for _, match := range docxTokenRe.FindAllStringSubmatch(string(body), -1) {
		if match[0] == "</w:p>" {
			text.WriteString("\n")
			continue
		}
		text.WriteString(match[1])
	}
	out := strings.TrimSpace(xmlEntities.Replace(text.String()))
	if out == "" {
		return "", errors.New("docx: no extractable text")
	}
	return out, nil
}
