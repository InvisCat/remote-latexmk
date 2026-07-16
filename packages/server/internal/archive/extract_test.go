package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestRejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "../escape", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	_ = gz.Close()
	_, err := ExtractTarGz(bytes.NewReader(buf.Bytes()), t.TempDir(), Limits{MaxFiles: 10, MaxBytes: 10})
	if err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestExtractsRegularFile(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "dir/main.tex", Mode: 0o644, Size: 3, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("abc"))
	_ = tw.Close()
	_ = gz.Close()
	root := t.TempDir()
	stats, err := ExtractTarGz(bytes.NewReader(buf.Bytes()), root, Limits{MaxFiles: 10, MaxBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 1 || stats.Bytes != 3 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	b, err := os.ReadFile(filepath.Join(root, "dir", "main.tex"))
	if err != nil || string(b) != "abc" {
		t.Fatalf("content=%q err=%v", b, err)
	}
}
