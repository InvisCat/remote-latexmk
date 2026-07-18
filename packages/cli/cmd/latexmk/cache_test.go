package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
