// SPDX-License-Identifier: AGPL-3.0-only
package httpapi

import (
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"chastedesk/internal/apperr"
	"chastedesk/internal/id"
)

const maxUploadBytes = 10 * 1024 * 1024

var uploadExtensions = map[string]bool{"pdf": true, "md": true, "txt": true, "docx": true}

func uploadExtension(filename string) (string, bool) {
	idx := strings.LastIndex(filename, ".")
	if idx < 0 {
		return "", false
	}
	ext := strings.ToLower(filename[idx+1:])
	return ext, uploadExtensions[ext]
}

func (s *Server) saveUpload(originalName string, file multipart.File) (string, error) {
	ext, ok := uploadExtension(originalName)
	if !ok {
		return "", apperr.BadRequest("file type not allowed: " + originalName)
	}
	if err := os.MkdirAll(s.cfg.UploadsDir, 0o755); err != nil {
		return "", err
	}
	storedName := id.Token(12) + "." + ext
	destination, err := os.Create(filepath.Join(s.cfg.UploadsDir, storedName))
	if err != nil {
		return "", err
	}
	defer destination.Close()
	written, err := io.Copy(destination, io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		return "", err
	}
	if written > maxUploadBytes {
		_ = os.Remove(destination.Name())
		return "", apperr.BadRequest("file exceeds 10 MiB")
	}
	return storedName, nil
}
