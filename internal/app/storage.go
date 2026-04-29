package app

import (
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

const (
	maxMultipartFormBytes   = 256 << 20
	maxMultipartMemoryBytes = 32 << 20
	maxUploadedFileBytes    = maxMultipartFormBytes
)

var errUploadTooLarge = errors.New("uploaded file exceeds 256MiB limit")

func prepareTaskDirs(root string, taskID string) (string, error) {
	if !isSafeTaskID(taskID) {
		return "", errors.New("invalid task id")
	}
	attachments := filepath.Join(root, taskID, "attachments")
	return attachments, os.MkdirAll(attachments, 0o755)
}

func saveUploadedFiles(files []*multipart.FileHeader, dst string) (int, error) {
	count := 0
	for _, header := range files {
		if header == nil || header.Filename == "" {
			continue
		}
		if err := saveUploadedFile(header, dst); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func saveUploadedFile(header *multipart.FileHeader, dst string) error {
	if header.Size > maxUploadedFileBytes {
		return errUploadTooLarge
	}
	source, err := header.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	name := safeFilename(header.Filename)
	target := filepath.Join(dst, name)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; fileExists(target); i++ {
		target = filepath.Join(dst, stem+"-"+strconvItoa(i)+ext)
	}

	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(output, io.LimitReader(source, maxUploadedFileBytes+1))
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return copyErr
	}
	if written > maxUploadedFileBytes {
		_ = os.Remove(target)
		return errUploadTooLarge
	}
	return closeErr
}

func safeFilename(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "attachment"
	}
	name = unsafeName.ReplaceAllString(name, "_")
	if len(name) > 180 {
		name = name[:180]
	}
	return name
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
