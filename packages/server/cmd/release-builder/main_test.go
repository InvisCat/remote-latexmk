package main

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteArchiveIsDeterministic(t *testing.T) {
	root := t.TempDir()
	files := []archiveFile{}
	for _, input := range []struct {
		name string
		mode os.FileMode
	}{{"remote-latexmk-server", 0o755}, {"install-server.sh", 0o755}, {"LICENSE", 0o644}} {
		path := filepath.Join(root, input.name)
		if err := os.WriteFile(path, []byte(input.name), input.mode); err != nil {
			t.Fatal(err)
		}
		files = append(files, archiveFile{name: input.name, path: path, mode: input.mode})
	}
	timestamp := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	first := filepath.Join(root, "first.tar.gz")
	second := filepath.Join(root, "second.tar.gz")
	if err := writeArchive(first, "remote-latexmk-server_1.2.3_linux_amd64", files, timestamp); err != nil {
		t.Fatal(err)
	}
	if err := writeArchive(second, "remote-latexmk-server_1.2.3_linux_amd64", files, timestamp); err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	firstDigest := sha256.Sum256(firstBytes)
	secondDigest := sha256.Sum256(secondBytes)
	if !bytes.Equal(firstDigest[:], secondDigest[:]) {
		t.Fatal("archive is not deterministic")
	}
}
