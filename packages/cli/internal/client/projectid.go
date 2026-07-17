package client

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const projectIDRelativePath = ".latexmk-cache/project-id"

var ErrProjectIDNotFound = errors.New("local project ID is not initialized")

// ResolveProjectID keeps a random identity with the project. Unlike an
// absolute-path hash, this remains distinct when Docker mounts every paper at
// /workspace. The cache directory is always excluded from uploads.
func ResolveProjectID(root string, create bool) (string, error) {
	resolved, err := resolvedProjectRoot(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(resolved, filepath.FromSlash(projectIDRelativePath))
	id, err := readProjectID(path)
	if err == nil {
		return id, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if !create {
		return "", ErrProjectIDNotFound
	}
	cacheDir := filepath.Dir(path)
	if err := ensurePrivateCacheDir(cacheDir); err != nil {
		return "", err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate project ID: %w", err)
	}
	id = "project-" + hex.EncodeToString(random)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return readProjectID(path)
	}
	if err != nil {
		return "", fmt.Errorf("create project ID: %w", err)
	}
	if _, err := io.WriteString(f, id+"\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write project ID: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("sync project ID: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close project ID: %w", err)
	}
	return id, nil
}

// LegacyProjectID returns the path-derived identifier used before local
// project IDs. It exists only so users can explicitly clean old server data.
func LegacyProjectID(root string) (string, error) {
	resolved, err := resolvedProjectRoot(root)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(filepath.Clean(resolved)))
	return "project-" + hex.EncodeToString(digest[:16]), nil
}

func readProjectID(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("project ID must be a regular file")
	}
	if info.Size() > 256 {
		return "", errors.New("project ID file is too large")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(payload))
	if !validProjectID(id) {
		return "", errors.New("project ID file contains an invalid identifier")
	}
	return id, nil
}

func ensurePrivateCacheDir(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New(".latexmk-cache must be a real directory")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create .latexmk-cache: %w", err)
	}
	return nil
}

func resolvedProjectRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("project root is not a directory")
	}
	return resolved, nil
}

func validProjectID(value string) bool {
	if len(value) == 0 || len(value) > 128 || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
