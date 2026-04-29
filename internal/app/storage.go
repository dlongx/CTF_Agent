package app

import (
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func prepareTaskDirs(root string, taskID string) (string, error) {
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
	defer output.Close()
	_, err = io.Copy(output, source)
	return err
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
