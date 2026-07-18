package client

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProjectIDPersistsDistinctIdentity(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	first, err := ResolveProjectID(firstRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ResolveProjectID(firstRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveProjectID(secondRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatalf("project ID changed: %q != %q", first, again)
	}
	if first == second {
		t.Fatalf("separate projects share ID %q", first)
	}
	info, err := os.Stat(filepath.Join(firstRoot, filepath.FromSlash(projectIDRelativePath)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("project ID permissions are too broad: %o", info.Mode().Perm())
	}
}

func TestResolveProjectIDWithStatusReportsOnlyCreation(t *testing.T) {
	root := t.TempDir()
	first, err := ResolveProjectIDWithStatus(root, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveProjectIDWithStatus(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || second.Created || first.ID != second.ID {
		t.Fatalf("resolutions = %#v, %#v", first, second)
	}
}

func TestResolveProjectIDDoesNotCreateDuringLookup(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveProjectID(root, false); !errors.Is(err, ErrProjectIDNotFound) {
		t.Fatalf("lookup error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".latexmk-cache")); !os.IsNotExist(err) {
		t.Fatalf("lookup created cache directory: %v", err)
	}
}

func TestResolveProjectIDRejectsSymlinkedCache(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, ".latexmk-cache")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ResolveProjectID(root, true); err == nil {
		t.Fatal("expected symlinked cache directory to be rejected")
	}
}

func TestLegacyProjectIDRemainsPathDerived(t *testing.T) {
	root := t.TempDir()
	first, err := LegacyProjectID(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LegacyProjectID(root)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !validProjectID(first) {
		t.Fatalf("legacy project IDs = %q, %q", first, second)
	}
}

func TestAddProjectCacheGitIgnoreAppendsAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	ignorePath := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(ignorePath, []byte("build/"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := AddProjectCacheGitIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AddProjectCacheGitIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Changed || second.Changed {
		t.Fatalf("results = %#v, %#v", first, second)
	}
	if string(payload) != "build/\n# latexmk local project identity and dependency cache\n.latexmk-cache/\n" {
		t.Fatalf(".gitignore = %q", payload)
	}
}

func TestAddProjectCacheGitIgnoreRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "ignore")
	if err := os.WriteFile(target, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, ".gitignore")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := AddProjectCacheGitIgnore(root); err == nil {
		t.Fatal("expected symlinked .gitignore to be rejected")
	}
}

func TestInspectProjectCacheGitIgnoreUsesEffectiveRules(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	root := t.TempDir()
	cmd := exec.Command(git, "init", "-q", root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	status, err := InspectProjectCacheGitIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.InWorkTree || status.Ignored {
		t.Fatalf("initial status = %#v", status)
	}
	result, err := AddProjectCacheGitIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatal("ignore rule was not added")
	}
	status, err = InspectProjectCacheGitIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.InWorkTree || !status.Ignored {
		t.Fatalf("updated status = %#v", status)
	}
	payload, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil || strings.Count(string(payload), ".latexmk-cache/") != 1 {
		t.Fatalf(".gitignore = %q, %v", payload, err)
	}
}

func TestInspectProjectCacheGitIgnoreChecksEveryCacheEntry(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	root := t.TempDir()
	if output, err := exec.Command(git, "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	cacheDir := filepath.Join(root, ".latexmk-cache")
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"project-id", "dependencies.json", "future-cache-file"} {
		if err := os.WriteFile(filepath.Join(cacheDir, name), []byte("cache\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ignore := ".latexmk-cache/*\n!.latexmk-cache/dependencies.json\n!.latexmk-cache/future-cache-file\n"
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(ignore), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := InspectProjectCacheGitIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.InWorkTree || status.Ignored {
		t.Fatalf("partially ignored cache status = %#v", status)
	}

	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".latexmk-cache/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err = InspectProjectCacheGitIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.InWorkTree || !status.Ignored {
		t.Fatalf("fully ignored cache status = %#v", status)
	}
}

func TestAddProjectCacheGitIgnoreOverridesLaterNegation(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	root := t.TempDir()
	if output, err := exec.Command(git, "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	cacheDir := filepath.Join(root, ".latexmk-cache")
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "project-id"), []byte("project-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ignore := ".latexmk-cache/*\n!.latexmk-cache/project-id\n"
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(ignore), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := InspectProjectCacheGitIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ignored {
		t.Fatal("negated project ID unexpectedly reported as ignored")
	}
	result, err := AddProjectCacheGitIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatal("effective non-ignore rule did not cause an append")
	}
	status, err = InspectProjectCacheGitIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Ignored {
		t.Fatal("appended rule did not restore effective ignore policy")
	}
}
