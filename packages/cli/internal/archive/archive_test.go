package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateExcludesDirectories(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "main.tex"), "test")
	mustWrite(t, filepath.Join(root, ".git", "config"), "secret")
	var buf bytes.Buffer
	stats, err := Create(&buf, Options{Root: root, Exclude: []string{".git"}})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 1 {
		t.Fatalf("files=%d", stats.Files)
	}
	gz, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	h, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "main.tex" {
		t.Fatalf("name=%q", h.Name)
	}
	if _, err := io.Copy(io.Discard, tr); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
