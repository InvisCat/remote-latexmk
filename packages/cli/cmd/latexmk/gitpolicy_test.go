package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	projectarchive "github.com/billstark001/latexmk/packages/cli/internal/archive"
)

func TestEffectiveGitExcludesFileResolvesRelativeAndHomePaths(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := t.TempDir()
	if output, err := exec.Command(git, "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}

	setCoreExcludesFile(t, git, repo, filepath.Join("policy", "global-ignore"))
	path, ok := effectiveGitExcludesFile(repo)
	if want := filepath.Join(repo, "policy", "global-ignore"); !ok || path != want {
		t.Fatalf("relative core.excludesFile = %q, %t; want %q", path, ok, want)
	}

	setCoreExcludesFile(t, git, repo, "~/global-ignore")
	path, ok = effectiveGitExcludesFile(repo)
	if want := filepath.Join(home, "global-ignore"); !ok || path != want {
		t.Fatalf("home core.excludesFile = %q, %t; want %q", path, ok, want)
	}
}

func TestEffectiveGitExcludesFileUsesMissingDefaultPath(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	home := t.TempDir()
	configHome := filepath.Join(home, "custom-config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := t.TempDir()
	if output, err := exec.Command(git, "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}

	path, ok := effectiveGitExcludesFile(repo)
	if want := filepath.Join(configHome, "git", "ignore"); !ok || path != want {
		t.Fatalf("default excludes path = %q, %t; want %q", path, ok, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("test expected missing path, stat error = %v", err)
	}
}

func TestEffectiveGitExcludesFileToleratesGitFailure(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	if path, ok := effectiveGitExcludesFile(repo); ok || path != "" {
		t.Fatalf("Git failure returned %q, %t", path, ok)
	}
}

func TestWatchTargetsIncludesConfiguredCoreExcludesFile(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := t.TempDir()
	if output, err := exec.Command(git, "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	project := filepath.Join(repo, "paper")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(project, "main.tex")
	if err := os.WriteFile(mainPath, []byte("paper\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setCoreExcludesFile(t, git, repo, filepath.Join("policy", "global-ignore"))

	targets := watchTargets(compileOptions{projectRoot: project, gitIgnore: true}, []projectarchive.File{{Path: "main.tex", Source: mainPath}})
	want := filepath.Join(repo, "policy", "global-ignore")
	for _, target := range targets {
		if target.Path == want {
			return
		}
	}
	t.Fatalf("watch targets do not include core.excludesFile %s: %#v", want, targets)
}

func TestDoctorRequiresTheWholeProjectCacheToBeIgnored(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	repo := t.TempDir()
	if output, err := exec.Command(git, "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	cacheDir := filepath.Join(repo, ".latexmk-cache")
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "project-id"), []byte("project-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "dependencies.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	partial := ".latexmk-cache/*\n!.latexmk-cache/dependencies.json\n"
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(partial), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stdout, stderr := captureCommandOutput(t, func() int {
		reportDoctorProjectCache(repo, "", false)
		return 0
	})
	if strings.Contains(stdout, "configured") || !strings.Contains(stderr, "latexmk cache ignore") {
		t.Fatalf("partial cache doctor output: stdout=%q stderr=%q", stdout, stderr)
	}

	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".latexmk-cache/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stdout, stderr = captureCommandOutput(t, func() int {
		reportDoctorProjectCache(repo, "", false)
		return 0
	})
	if !strings.Contains(stdout, "configured") || stderr != "" {
		t.Fatalf("full cache doctor output: stdout=%q stderr=%q", stdout, stderr)
	}
}

func setCoreExcludesFile(t *testing.T, git, repo, value string) {
	t.Helper()
	cmd := exec.Command(git, "-C", repo, "config", "core.excludesFile", value)
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set core.excludesFile: %v: %s", err, output)
	}
}
