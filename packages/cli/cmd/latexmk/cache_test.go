package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func useTestCleanupPlansDir(t *testing.T) {
	t.Helper()
	old := cleanupPlansDir
	dir := t.TempDir()
	cleanupPlansDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { cleanupPlansDir = old })
}

func TestLocalCleanupPlanPreviewAndApplyPreservesProjectID(t *testing.T) {
	useTestCleanupPlansDir(t)
	root := t.TempDir()
	cacheDir := filepath.Join(root, ".latexmk-cache")
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "dependencies.json"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "project-id"), []byte("keep-me\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := createLocalCleanupPlan(root, "local-client-cache")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Targets) != 1 || plan.Targets[0].Path != ".latexmk-cache/dependencies.json" {
		t.Fatalf("targets = %#v", plan.Targets)
	}
	result, err := applyLocalCleanupPlan(root, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 {
		t.Fatalf("removed = %d", result.Removed)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "dependencies.json")); !os.IsNotExist(err) {
		t.Fatalf("dependency cache still exists: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(cacheDir, "project-id")); err != nil || string(content) != "keep-me\n" {
		t.Fatalf("project ID changed: %q, %v", content, err)
	}
}

func TestLocalCleanupPlanRejectsChangedTargetWithoutDeleting(t *testing.T) {
	useTestCleanupPlansDir(t)
	root := t.TempDir()
	path := filepath.Join(root, "main.aux")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := createLocalCleanupPlan(root, "local-generated")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := applyLocalCleanupPlan(root, plan.ID); err == nil || !strings.Contains(err.Error(), "changed since preview") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("changed target was deleted: %v", err)
	}
}

func TestCollectGeneratedTargetsDoesNotFollowSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.log"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	targets, err := collectCleanupTargets(root, "local-generated")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("followed symlink: %#v", targets)
	}
}

func TestParseCacheArgsRequiresTwoPhaseApply(t *testing.T) {
	opts := cacheCommandOptions{}
	if err := parseCacheArgs("clean", []string{"--scope", "local-generated", "--yes"}, &opts); err == nil {
		t.Fatal("expected direct --scope --yes to be rejected")
	}
	opts = cacheCommandOptions{}
	if err := parseCacheArgs("clean", []string{"--plan-id", "not-an-id", "--yes"}, &opts); err == nil {
		t.Fatal("expected invalid plan ID to be rejected")
	}
	opts = cacheCommandOptions{}
	if err := parseCacheArgs("clean", []string{"--plan-id", strings.Repeat("a", 32), "--yes"}, &opts); err != nil {
		t.Fatalf("valid apply arguments rejected: %v", err)
	}
	opts = cacheCommandOptions{}
	if err := parseCacheArgs("clean", []string{"--plan-id", strings.Repeat("a", 32), "--yes", "--dry-run"}, &opts); err == nil {
		t.Fatal("expected --yes --dry-run conflict to be rejected")
	}
}

func TestParseCacheIgnoreAcceptsOnlyLocalOptions(t *testing.T) {
	opts := cacheCommandOptions{}
	if err := parseCacheArgs("ignore", []string{"--project-root", ".", "--json"}, &opts); err != nil {
		t.Fatalf("valid cache ignore options rejected: %v", err)
	}
	opts = cacheCommandOptions{}
	if err := parseCacheArgs("ignore", []string{"--scope", "local-client-cache"}, &opts); err == nil {
		t.Fatal("cache ignore accepted cleanup options")
	}
}

func TestRunCacheIgnoreUpdatesGitIgnore(t *testing.T) {
	root := t.TempDir()
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	if code := runCache([]string{"ignore"}); code != 0 {
		t.Fatalf("runCache(ignore) exit = %d", code)
	}
	payload, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), ".latexmk-cache/") {
		t.Fatalf(".gitignore = %q", payload)
	}
}

func TestCleanupPreviewListsBoundedTargetPaths(t *testing.T) {
	plan := cleanupPlan{ID: strings.Repeat("a", 32), Scope: "local-generated", ExpiresAt: time.Now().UTC()}
	for i := 0; i < cleanupPreviewMax+2; i++ {
		plan.Targets = append(plan.Targets, cleanupTarget{Path: fmt.Sprintf("build/file-%02d.aux", i), Size: int64(i + 1)})
	}
	var output bytes.Buffer
	if err := writeCleanupPreview(&output, plan); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "build/file-00.aux") || !strings.Contains(text, "build/file-19.aux") {
		t.Fatalf("preview omitted shown paths: %s", text)
	}
	if strings.Contains(text, "build/file-20.aux") || !strings.Contains(text, "2 more target(s)") {
		t.Fatalf("preview was not bounded: %s", text)
	}
}

func TestLocalCleanupReportsPartialApplyDetails(t *testing.T) {
	useTestCleanupPlansDir(t)
	root := t.TempDir()
	for _, name := range []string{"a.aux", "b.aux"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := createLocalCleanupPlan(root, "local-generated")
	if err != nil {
		t.Fatal(err)
	}
	oldRemove := cleanupRemoveFile
	cleanupRemoveFile = func(path string) error {
		if filepath.Base(path) == "b.aux" {
			return errors.New("injected remove failure")
		}
		return os.Remove(path)
	}
	t.Cleanup(func() { cleanupRemoveFile = oldRemove })

	result, err := applyLocalCleanupPlan(root, plan.ID)
	var applyErr *cleanupApplyError
	if !errors.As(err, &applyErr) {
		t.Fatalf("error = %T %v", err, err)
	}
	if result.Removed != 1 || applyErr.FailedPath != "b.aux" {
		t.Fatalf("partial result = %#v, error = %#v", result, applyErr)
	}
	code, details, _, _ := classifyAgentError(err)
	if code != "cleanup_apply_failed" || details["removed"] != 1 || details["remainingTargets"] != 1 {
		t.Fatalf("classified error = %q %#v", code, details)
	}
	if _, err := os.Stat(filepath.Join(root, "a.aux")); !os.IsNotExist(err) {
		t.Fatalf("first target was not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.aux")); err != nil {
		t.Fatalf("failed target disappeared: %v", err)
	}
}
