package client

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const projectIDRelativePath = ".latexmk-cache/project-id"

const ProjectCacheGitAdvice = ".latexmk-cache/project-id stores this paper's local project identity. If this project uses Git, run \"latexmk cache ignore\" to add .latexmk-cache/ to .gitignore. Warning: \"git clean -fdX\" deletes ignored cache files; the next compile will create a new project ID."

var ErrProjectIDNotFound = errors.New("local project ID is not initialized")

type ProjectIDResolution struct {
	ID      string
	Created bool
}

type ProjectCacheGitStatus struct {
	InWorkTree bool
	Ignored    bool
}

type ProjectCacheIgnoreResult struct {
	ProjectRoot string `json:"projectRoot"`
	GitIgnore   string `json:"gitIgnore"`
	Changed     bool   `json:"changed"`
}

// ResolveProjectID keeps a random identity with the project. Unlike an
// absolute-path hash, this remains distinct when Docker mounts every paper at
// /workspace. The cache directory is always excluded from uploads.
func ResolveProjectID(root string, create bool) (string, error) {
	result, err := ResolveProjectIDWithStatus(root, create)
	return result.ID, err
}

// ResolveProjectIDWithStatus also reports whether this call created the ID.
func ResolveProjectIDWithStatus(root string, create bool) (ProjectIDResolution, error) {
	resolved, err := resolvedProjectRoot(root)
	if err != nil {
		return ProjectIDResolution{}, err
	}
	path := filepath.Join(resolved, filepath.FromSlash(projectIDRelativePath))
	id, err := readProjectID(path)
	if err == nil {
		return ProjectIDResolution{ID: id}, nil
	}
	if !os.IsNotExist(err) {
		return ProjectIDResolution{}, err
	}
	if !create {
		return ProjectIDResolution{}, ErrProjectIDNotFound
	}
	cacheDir := filepath.Dir(path)
	if err := ensurePrivateCacheDir(cacheDir); err != nil {
		return ProjectIDResolution{}, err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return ProjectIDResolution{}, fmt.Errorf("generate project ID: %w", err)
	}
	id = "project-" + hex.EncodeToString(random)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		id, readErr := readProjectID(path)
		return ProjectIDResolution{ID: id}, readErr
	}
	if err != nil {
		return ProjectIDResolution{}, fmt.Errorf("create project ID: %w", err)
	}
	if _, err := io.WriteString(f, id+"\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return ProjectIDResolution{}, fmt.Errorf("write project ID: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return ProjectIDResolution{}, fmt.Errorf("sync project ID: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return ProjectIDResolution{}, fmt.Errorf("close project ID: %w", err)
	}
	return ProjectIDResolution{ID: id, Created: true}, nil
}

// InspectProjectCacheGitIgnore checks the effective Git ignore policy without
// changing the repository. A missing Git work tree is not an error.
func InspectProjectCacheGitIgnore(root string) (ProjectCacheGitStatus, error) {
	resolved, err := resolvedProjectRoot(root)
	if err != nil {
		return ProjectCacheGitStatus{}, err
	}
	checkTree := exec.Command("git", "-C", resolved, "rev-parse", "--is-inside-work-tree")
	checkTree.Env = append(os.Environ(), "LC_ALL=C")
	output, err := checkTree.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && strings.Contains(string(output), "not a git repository") {
			return ProjectCacheGitStatus{}, nil
		}
		return ProjectCacheGitStatus{}, fmt.Errorf("inspect Git work tree: %w", err)
	}
	if strings.TrimSpace(string(output)) != "true" {
		return ProjectCacheGitStatus{}, nil
	}
	status := ProjectCacheGitStatus{InWorkTree: true}
	checkIgnore := exec.Command("git", "-C", resolved, "check-ignore", "-q", "--no-index", "--", filepath.FromSlash(projectIDRelativePath))
	checkIgnore.Env = append(os.Environ(), "LC_ALL=C")
	err = checkIgnore.Run()
	if err == nil {
		status.Ignored = true
		return status, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return status, nil
	}
	return ProjectCacheGitStatus{}, fmt.Errorf("check Git ignore rules: %w", err)
}

// AddProjectCacheGitIgnore appends the cache rule only after an explicit user
// command. It never replaces or truncates an existing ignore file.
func AddProjectCacheGitIgnore(root string) (ProjectCacheIgnoreResult, error) {
	resolved, err := resolvedProjectRoot(root)
	if err != nil {
		return ProjectCacheIgnoreResult{}, err
	}
	result := ProjectCacheIgnoreResult{ProjectRoot: resolved, GitIgnore: filepath.Join(resolved, ".gitignore")}
	status, statusErr := InspectProjectCacheGitIgnore(resolved)
	if statusErr == nil && status.InWorkTree && status.Ignored {
		return result, nil
	}
	knownNotIgnored := statusErr == nil && status.InWorkTree

	var existing []byte
	info, err := os.Lstat(result.GitIgnore)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return result, errors.New(".gitignore must be a regular file")
		}
		if info.Size() > 1<<20 {
			return result, errors.New(".gitignore is too large to update safely")
		}
		existing, err = os.ReadFile(result.GitIgnore)
		if err != nil {
			return result, err
		}
	} else if !os.IsNotExist(err) {
		return result, err
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == ".latexmk-cache/" && !knownNotIgnored {
			return result, nil
		}
	}

	addition := []byte("# latexmk local project identity and dependency cache\n.latexmk-cache/\n")
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		addition = append([]byte("\n"), addition...)
	}
	f, err := os.OpenFile(result.GitIgnore, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return result, fmt.Errorf("open .gitignore: %w", err)
	}
	if _, err := f.Write(addition); err != nil {
		_ = f.Close()
		return result, fmt.Errorf("update .gitignore: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return result, fmt.Errorf("sync .gitignore: %w", err)
	}
	if err := f.Close(); err != nil {
		return result, fmt.Errorf("close .gitignore: %w", err)
	}
	result.Changed = true
	return result, nil
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
