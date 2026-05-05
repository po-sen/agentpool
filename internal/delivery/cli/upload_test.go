package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestCollectUploadInputsPreservesFileBehavior(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "README.md")
	writeUploadTestFile(t, filePath, "# Demo\n")

	uploads, err := collectUploadInputs(CreateRunRequest{Files: []string{filePath}})
	if err != nil {
		t.Fatalf("collect uploads: %v", err)
	}
	assertUploadNames(t, uploads, []string{"README.md"})
	if string(uploads[0].content) != "# Demo\n" {
		t.Fatalf("content = %q, want README content", string(uploads[0].content))
	}
}

func TestCollectUploadInputsExpandsDirectoryRelativePaths(t *testing.T) {
	root := t.TempDir()
	writeUploadTestFile(t, filepath.Join(root, "README.md"), "# Demo\n")
	writeUploadTestFile(t, filepath.Join(root, "cmd", "agentpool", "main.go"), "package main\n")

	uploads, err := collectUploadInputs(CreateRunRequest{Dirs: []string{root}})
	if err != nil {
		t.Fatalf("collect uploads: %v", err)
	}
	assertUploadNames(t, uploads, []string{"README.md", "cmd/agentpool/main.go"})
}

func TestCollectUploadInputsSkipsExcludedDirectories(t *testing.T) {
	root := t.TempDir()
	writeUploadTestFile(t, filepath.Join(root, "README.md"), "# Demo\n")
	writeUploadTestFile(t, filepath.Join(root, "node_modules", "leftpad", "index.js"), "module.exports = 1\n")
	writeUploadTestFile(t, filepath.Join(root, ".git", "config"), "[core]\n")
	writeUploadTestFile(t, filepath.Join(root, "build", "bundle.js"), "console.log(1)\n")

	uploads, err := collectUploadInputs(CreateRunRequest{Dirs: []string{root}})
	if err != nil {
		t.Fatalf("collect uploads: %v", err)
	}
	assertUploadNames(t, uploads, []string{"README.md"})
}

func TestCollectUploadInputsExpandsTarGzArchive(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "project.tar.gz")
	writeTarGzArchive(t, archivePath, map[string]string{
		"README.md":       "# Demo\n",
		"internal/app.go": "package internal\n",
		"vendor/lib.js":   "console.log(1)\n",
	})

	uploads, err := collectUploadInputs(CreateRunRequest{Archives: []string{archivePath}})
	if err != nil {
		t.Fatalf("collect uploads: %v", err)
	}
	assertUploadNames(t, uploads, []string{"README.md", "internal/app.go"})
}

func TestCollectUploadInputsRejectsUnsafePaths(t *testing.T) {
	if _, err := collectUploadInputs(CreateRunRequest{Files: []string{"../README.md"}}); err == nil {
		t.Fatal("collect uploads error = nil, want unsafe file path error")
	}

	archivePath := filepath.Join(t.TempDir(), "unsafe.tar.gz")
	writeTarGzArchive(t, archivePath, map[string]string{"../secret.txt": "secret\n"})
	if _, err := collectUploadInputs(CreateRunRequest{Archives: []string{archivePath}}); err == nil {
		t.Fatal("collect archive uploads error = nil, want unsafe archive path error")
	}
}

func writeUploadTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir test file parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}

func writeTarGzArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()

	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		content := []byte(files[name])
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		}); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
}

func assertUploadNames(t *testing.T, uploads []uploadFile, want []string) {
	t.Helper()

	got := make([]string, 0, len(uploads))
	for _, upload := range uploads {
		got = append(got, upload.filename)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("upload names = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("upload names = %#v, want %#v", got, want)
		}
	}
}
