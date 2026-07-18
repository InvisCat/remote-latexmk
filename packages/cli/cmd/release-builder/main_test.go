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
	binary := filepath.Join(root, "latexmk")
	license := filepath.Join(root, "LICENSE")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(license, []byte("license"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []archiveFile{{name: "latexmk", path: binary, mode: 0o755}, {name: "LICENSE", path: license, mode: 0o644}}
	timestamp := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	for _, windows := range []bool{false, true} {
		first := filepath.Join(root, "first")
		second := filepath.Join(root, "second")
		if err := writeArchive(first, "latexmk_1.2.3_test", files, timestamp, windows); err != nil {
			t.Fatal(err)
		}
		if err := writeArchive(second, "latexmk_1.2.3_test", files, timestamp, windows); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(first)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Fatalf("archive mode = %o, want 644", info.Mode().Perm())
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
			t.Fatalf("archive is not deterministic (windows=%t)", windows)
		}
	}
}

func TestSupportedTargets(t *testing.T) {
	for _, target := range [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}} {
		if !supportedTarget(target[0], target[1]) {
			t.Errorf("expected %s/%s to be supported", target[0], target[1])
		}
	}
	if supportedTarget("plan9", "amd64") || supportedTarget("linux", "386") {
		t.Fatal("unexpected release target accepted")
	}
}
