// SPDX-License-Identifier: AGPL-3.0-only
package ingest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"chastedesk/internal/retrieval"
)

func resolveUploadPath(uploadsDir, filePath string) string {
	return filepath.Join(uploadsDir, filepath.Base(filePath))
}

const maxFileBytes = 10 * 1024 * 1024

var allowedExtensions = map[string]bool{"pdf": true, "md": true, "txt": true, "docx": true}

func ExtensionOf(filename string) (string, bool) {
	idx := strings.LastIndex(filename, ".")
	if idx < 0 {
		return "", false
	}
	ext := strings.ToLower(filename[idx+1:])
	return ext, allowedExtensions[ext]
}

func ParseFile(path, filename string) (retrieval.ParsedDocument, error) {
	ext, ok := ExtensionOf(filename)
	if !ok {
		return retrieval.ParsedDocument{}, fmt.Errorf("file type not allowed: %s", filename)
	}
	info, err := os.Stat(path)
	if err != nil {
		return retrieval.ParsedDocument{}, err
	}
	if info.Size() > maxFileBytes {
		return retrieval.ParsedDocument{}, errors.New("file exceeds 10 MiB")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return retrieval.ParsedDocument{}, err
	}
	switch ext {
	case "pdf":
		if !bytes.HasPrefix(raw, []byte("%PDF")) {
			return retrieval.ParsedDocument{}, errors.New("pdf magic bytes missing")
		}
		text, err := ExtractPDFText(raw)
		if err != nil {
			return retrieval.ParsedDocument{}, err
		}
		return retrieval.ParsedDocument{Title: filename, Markdown: text}, nil
	case "docx":
		if !bytes.HasPrefix(raw, []byte{0x50, 0x4b, 0x03, 0x04}) {
			return retrieval.ParsedDocument{}, errors.New("docx magic bytes missing")
		}
		text, err := ExtractDocxText(raw)
		if err != nil {
			return retrieval.ParsedDocument{}, err
		}
		return retrieval.ParsedDocument{Title: filename, Markdown: text}, nil
	default:
		return retrieval.ParsedDocument{Title: filename, Markdown: string(raw)}, nil
	}
}
