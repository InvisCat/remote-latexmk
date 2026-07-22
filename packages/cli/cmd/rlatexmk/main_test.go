package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	projectarchive "github.com/billstark001/latexmk/packages/cli/internal/archive"
	"github.com/billstark001/latexmk/packages/cli/internal/client"
	"github.com/billstark001/latexmk/packages/cli/internal/config"
	"github.com/billstark001/latexmk/packages/cli/internal/dependency"
)

func captureCommandOutput(t *testing.T, fn func() int) (int, string, string) {
	t.Helper()
	oldStdout, oldStderr := os.Stdout, os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = stdoutW, stderrW
	defer func() {
		os.Stdout, os.Stderr = oldStdout, oldStderr
	}()
	code := fn()
	_ = stdoutW.Close()
	_ = stderrW.Close()
	stdout, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatal(err)
	}
	_ = stdoutR.Close()
	_ = stderrR.Close()
	return code, string(stdout), string(stderr)
}

func TestVersionIdentifiesRemoteLatexmkClient(t *testing.T) {
	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "version"})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("version result: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "rlatexmk (remote-latexmk client)") {
		t.Fatalf("version output does not identify remote-latexmk: %q", stdout)
	}
}

func TestEntriesJSONDiscoversEntryWithoutServerAccess(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte("\\documentclass{article}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapter.tex"), []byte("chapter"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "entries", "--json", "--project-root", root})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("entries result: code=%d stderr=%q", code, stderr)
	}
	var result dependency.EntryDiscovery
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode entries %q: %v", stdout, err)
	}
	if !result.Unambiguous || result.Selected != "main.tex" || result.CandidateCount != 1 {
		t.Fatalf("entries = %#v", result)
	}
}

func TestEntriesLoadsPolicyFromExplicitProjectRoot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte("\\documentclass{article}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "excluded.tex"), []byte("\\documentclass{book}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.FileName), []byte(`{"exclude":["excluded.tex"],"projectRoot":".."}`), 0o600); err != nil {
		t.Fatal(err)
	}
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(outside); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDirectory) })

	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "entries", "--json", "--project-root", root})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("entries result: code=%d stderr=%q", code, stderr)
	}
	var result dependency.EntryDiscovery
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode entries %q: %v", stdout, err)
	}
	if !result.Unambiguous || result.Selected != "main.tex" || result.CandidateCount != 1 {
		t.Fatalf("entries = %#v", result)
	}
}

func TestEntriesExplicitProjectRootDoesNotLoadParentProjectPolicy(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	parent := t.TempDir()
	root := filepath.Join(parent, "paper")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte("\\documentclass{article}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, config.FileName), []byte(`{"exclude":["main.tex"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "entries", "--json", "--project-root", root})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("entries result: code=%d stderr=%q", code, stderr)
	}
	var result dependency.EntryDiscovery
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode entries %q: %v", stdout, err)
	}
	if !result.Unambiguous || result.Selected != "main.tex" {
		t.Fatalf("parent policy escaped explicit root: %#v", result)
	}
}

func TestEntriesDoesNotReadProjectTokenFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte("\\documentclass{article}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.FileName), []byte(`{"tokenFile":"missing-token"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDirectory) })

	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "entries", "--json"})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("entries result: code=%d stderr=%q", code, stderr)
	}
	var result dependency.EntryDiscovery
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode entries %q: %v", stdout, err)
	}
	if !result.Unambiguous || result.Selected != "main.tex" {
		t.Fatalf("entries = %#v", result)
	}
}

func TestNormalizeCompilePathsResolvesProjectRootSymlink(t *testing.T) {
	physicalRoot := t.TempDir()
	entry := filepath.Join(physicalRoot, "main.tex")
	if err := os.WriteFile(entry, []byte("\\documentclass{article}"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(physicalRoot, alias); err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(physicalRoot)
	if err != nil {
		t.Fatal(err)
	}
	opts := compileOptions{projectRoot: alias, entry: "main.tex"}
	if err := normalizeCompilePaths(&opts, physicalRoot); err != nil {
		t.Fatal(err)
	}
	if opts.projectRoot != resolvedRoot {
		t.Fatalf("project root = %q, want %q", opts.projectRoot, resolvedRoot)
	}
	if opts.entry != "main.tex" {
		t.Fatalf("entry = %q, want main.tex", opts.entry)
	}
}

func TestNormalizeCompilePathsDefaultsToEntryDirectory(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "paper")
	entry := filepath.Join(project, "main.tex")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("\\documentclass{article}"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := compileOptions{rootMode: "entry", entry: filepath.Join("paper", "main.tex")}
	if err := normalizeCompilePaths(&opts, parent); err != nil {
		t.Fatal(err)
	}
	resolvedProject, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	if opts.projectRoot != resolvedProject {
		t.Fatalf("project root = %q, want %q", opts.projectRoot, resolvedProject)
	}
	if opts.entry != "main.tex" {
		t.Fatalf("entry = %q, want main.tex", opts.entry)
	}
}

func TestNormalizeCompilePathsUsesGitRootOnlyWhenRequested(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(repo, "papers", "demo")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(project, "main.tex")
	if err := os.WriteFile(entry, []byte("\\documentclass{article}"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := compileOptions{rootMode: "git", entry: "main.tex"}
	if err := normalizeCompilePaths(&opts, project); err != nil {
		t.Fatal(err)
	}
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if opts.projectRoot != resolvedRepo {
		t.Fatalf("project root = %q, want %q", opts.projectRoot, resolvedRepo)
	}
	if opts.entry != "papers/demo/main.tex" {
		t.Fatalf("entry = %q, want papers/demo/main.tex", opts.entry)
	}
}

func TestParseCompileArgsRejectsUnknownRootMode(t *testing.T) {
	opts := compileOptions{timeout: time.Minute}
	if err := parseCompileArgs([]string{"--root-mode", "parent", "main.tex"}, &opts); err == nil {
		t.Fatal("expected invalid root mode error")
	}
}

func TestParseCompileArgsReadsTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := compileOptions{timeout: time.Minute}
	if err := parseCompileArgs([]string{"--token-file", path, "main.tex"}, &opts); err != nil {
		t.Fatal(err)
	}
	if opts.token != "file-token" {
		t.Fatalf("token = %q, want file-token", opts.token)
	}
}

func TestParseCompileArgsUploadMode(t *testing.T) {
	opts := compileOptions{timeout: time.Minute, uploadMode: "auto"}
	if err := parseCompileArgs([]string{"--upload-mode", "all", "main.tex"}, &opts); err != nil {
		t.Fatal(err)
	}
	if opts.uploadMode != "all" {
		t.Fatalf("upload mode = %q, want all", opts.uploadMode)
	}
	if err := parseCompileArgs([]string{"--upload-mode", "legacy", "main.tex"}, &opts); err == nil {
		t.Fatal("expected unsupported upload mode to fail")
	}
}

func TestParseCompileArgsManifestFiles(t *testing.T) {
	opts := compileOptions{timeout: time.Minute, uploadMode: "auto"}
	args := []string{"--upload-mode", "manifest", "--manifest", ".latexmk-files", "--include-file", "chapter.tex", "--include-file=figure.pdf", "main.tex"}
	if err := parseCompileArgs(args, &opts); err != nil {
		t.Fatal(err)
	}
	if opts.uploadMode != "manifest" || opts.manifestFile != ".latexmk-files" {
		t.Fatalf("manifest options = %#v", opts)
	}
	if len(opts.includeFiles) != 2 || opts.includeFiles[0] != "chapter.tex" || opts.includeFiles[1] != "figure.pdf" {
		t.Fatalf("include files = %#v", opts.includeFiles)
	}
}

func TestParseCompileArgsWatchOptions(t *testing.T) {
	opts := compileOptions{timeout: time.Minute, watchInterval: 500 * time.Millisecond, watchDebounce: 500 * time.Millisecond}
	if err := parseCompileArgs([]string{"--watch", "--watch-interval", "25ms", "--watch-debounce=75ms", "main.tex"}, &opts); err != nil {
		t.Fatal(err)
	}
	if !opts.watch || opts.watchInterval != 25*time.Millisecond || opts.watchDebounce != 75*time.Millisecond {
		t.Fatalf("watch options = %#v", opts)
	}
	opts.watchInterval = 0
	if err := parseCompileArgs([]string{"--watch", "main.tex"}, &opts); err == nil {
		t.Fatal("expected zero watch interval to fail")
	}
}

func TestParseCompileArgsDetach(t *testing.T) {
	opts := compileOptions{timeout: time.Minute}
	if err := parseCompileArgs([]string{"--detach", "--json", "main.tex"}, &opts); err != nil {
		t.Fatal(err)
	}
	if !opts.detach || !opts.jsonOutput || opts.entry != "main.tex" {
		t.Fatalf("detach options = %#v", opts)
	}
}

func TestCapabilityErrorUsesStableAgentCode(t *testing.T) {
	code, details, retryable, exitCode := classifyAgentError(&client.CapabilityError{Capability: "detached queued compilation"})
	if code != "unsupported_capability" || retryable || exitCode != 1 || details["capability"] != "detached queued compilation" {
		t.Fatalf("classification = %q %#v %t %d", code, details, retryable, exitCode)
	}
}

func TestParseResultCommandArgs(t *testing.T) {
	opts := resultCommandOptions{timeout: time.Minute, source: "all", tailLines: 200, maxBytes: 64 << 10}
	if err := parseResultCommandArgs("logs", []string{"job_test", "--source", "compiler", "--tail", "25", "--max-bytes", "4096", "--json"}, &opts); err != nil {
		t.Fatal(err)
	}
	if opts.jobID != "job_test" || opts.source != "compiler" || opts.tailLines != 25 || opts.maxBytes != 4096 || !opts.jsonOutput {
		t.Fatalf("log options = %#v", opts)
	}
	opts = resultCommandOptions{timeout: time.Minute}
	if err := parseResultCommandArgs("artifacts.get", []string{"job_test", strings.Repeat("a", 32), "--out-dir", "build"}, &opts); err != nil {
		t.Fatal(err)
	}
	if opts.jobID != "job_test" || opts.artifactID != strings.Repeat("a", 32) || opts.outDir != "build" {
		t.Fatalf("artifact options = %#v", opts)
	}
	opts = resultCommandOptions{timeout: time.Minute}
	if err := parseResultCommandArgs("diagnostics", []string{"job_test", "--json"}, &opts); err != nil {
		t.Fatal(err)
	}
	if opts.jobID != "job_test" || !opts.jsonOutput {
		t.Fatalf("diagnostic options = %#v", opts)
	}
	if err := parseResultCommandArgs("diagnostics", []string{"job_test", "--tail", "20"}, &opts); err == nil {
		t.Fatal("expected diagnostics to reject log-only options")
	}
}

func TestResultStateErrorUsesRetryableAgentCode(t *testing.T) {
	code, details, retryable, exitCode := classifyAgentError(&client.ResultStateError{Status: "running"})
	if code != "result_not_ready" || !retryable || exitCode != 1 || details["status"] != "running" {
		t.Fatalf("classification = %q %#v %t %d", code, details, retryable, exitCode)
	}
}

func TestSelectedFilesChangedIgnoresNewRecorderDependencies(t *testing.T) {
	before := []projectarchive.File{{Path: "main.tex", SHA256: "main-v1"}}
	after := []projectarchive.File{{Path: "main.tex", SHA256: "main-v1"}, {Path: "dynamic.tex", SHA256: "dynamic-v1"}}
	if selectedFilesChanged(before, after) {
		t.Fatal("a newly discovered dependency should not imply an edit during compilation")
	}
	after[0].SHA256 = "main-v2"
	if !selectedFilesChanged(before, after) {
		t.Fatal("an existing selected file change was not detected")
	}
}

func TestWatchTargetsOnlyAddsSelectedFilesAndPolicyControls(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git", "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(repo, "paper")
	if err := os.MkdirAll(filepath.Join(project, "sections"), 0o700); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(project, "main.tex")
	bodyPath := filepath.Join(project, "sections", "body.tex")
	targets := watchTargets(compileOptions{projectRoot: project, gitIgnore: true, manifestFile: ".latexmk-files"}, []projectarchive.File{
		{Path: "main.tex", Source: mainPath},
		{Path: "sections/body.tex", Source: bodyPath},
	})
	paths := make(map[string]bool)
	for _, target := range targets {
		paths[target.Path] = true
	}
	for _, required := range []string{
		mainPath,
		bodyPath,
		filepath.Join(project, ".latexmk-files"),
		filepath.Join(project, ".gitignore"),
		filepath.Join(project, "sections", ".gitignore"),
		filepath.Join(repo, ".gitignore"),
		filepath.Join(repo, ".git", "info", "exclude"),
	} {
		if !paths[required] {
			t.Errorf("missing watch target %s", required)
		}
	}
	if paths[filepath.Join(project, "unrelated-secret.txt")] {
		t.Fatal("watcher included an unrelated project file")
	}
}

func TestParseRemoteCleanArgsRequiresPreviewPlan(t *testing.T) {
	validPlanID := strings.Repeat("a", 32)
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "direct scope apply", args: []string{"--scope", "project", "--yes"}, want: "--plan-id"},
		{name: "plan without confirmation", args: []string{"--plan-id", validPlanID}, want: "requires --yes"},
		{name: "scope with plan", args: []string{"--scope", "project", "--plan-id", validPlanID, "--yes"}, want: "do not pass --scope"},
		{name: "invalid plan", args: []string{"--plan-id", "invalid", "--yes"}, want: "valid --plan-id"},
		{name: "dry run apply", args: []string{"--plan-id", validPlanID, "--yes", "--dry-run"}, want: "cannot be used together"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := parseRemoteCleanArgs(test.args, &remoteCleanOptions{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
	if err := parseRemoteCleanArgs([]string{"--scope", "results"}, &remoteCleanOptions{}); err != nil {
		t.Fatalf("preview arguments rejected: %v", err)
	}
	if err := parseRemoteCleanArgs([]string{"--plan-id", validPlanID, "--yes"}, &remoteCleanOptions{}); err != nil {
		t.Fatalf("apply arguments rejected: %v", err)
	}
}

func TestRemoteCleanPlanBindsPreviewAndConsumesOnSuccess(t *testing.T) {
	useTestCleanupPlansDir(t)
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	digest := strings.Repeat("a", 64)
	methods := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/meta":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"protocolVersion": 2,
				"capabilities":    map[string]any{"remoteCleanup": true},
			})
		case "/v1/projects/project-test/cleanup":
			methods = append(methods, r.Method)
			if r.Method == http.MethodDelete && r.URL.Query().Get("expectedDigest") != digest {
				http.Error(w, "missing digest", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"projectId": "project-test", "scope": "project", "dryRun": r.Method == http.MethodGet,
				"planDigest":      digest,
				"snapshotPresent": true, "snapshotFiles": 1, "snapshotBytes": 5,
				"jobs": 1, "results": 1, "resultBytes": 3, "reclaimedBytes": 8,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	base := []string{"rlatexmk", "remote", "clean", "--project-root", root, "--project-id", "project-test", "--server", server.URL, "--token", "secret-token", "--json"}
	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run(append(append([]string{}, base...), "--scope", "project"))
	})
	if code != 0 || stderr != "" {
		t.Fatalf("preview code=%d stderr=%q", code, stderr)
	}
	var preview remoteCleanupOutput
	if err := json.Unmarshal([]byte(stdout), &preview); err != nil {
		t.Fatalf("decode preview %q: %v", stdout, err)
	}
	if !cleanupPlanIDPattern.MatchString(preview.PlanID) || preview.ExpiresAt == nil || preview.Report.PlanDigest != digest {
		t.Fatalf("preview = %#v", preview)
	}
	plansDir, err := cleanupPlansDir()
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(plansDir, preview.PlanID+".json")
	payload, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "secret-token") || strings.Contains(strings.ToLower(string(payload)), `"token"`) {
		t.Fatalf("cleanup plan persisted authentication material: %s", payload)
	}
	for _, expected := range []string{server.URL, "project-test", "project", digest} {
		if !strings.Contains(string(payload), expected) {
			t.Fatalf("cleanup plan omitted %q: %s", expected, payload)
		}
	}

	code, _, stderr = captureCommandOutput(t, func() int {
		return run(append(append([]string{}, base...), "--plan-id", preview.PlanID, "--yes"))
	})
	if code != 0 || stderr != "" {
		t.Fatalf("apply code=%d stderr=%q", code, stderr)
	}
	if len(methods) != 2 || methods[0] != http.MethodGet || methods[1] != http.MethodDelete {
		t.Fatalf("cleanup methods = %#v", methods)
	}
	if _, err := os.Stat(planPath); !os.IsNotExist(err) {
		t.Fatalf("successful plan was not consumed: %v", err)
	}
	code, _, _ = captureCommandOutput(t, func() int {
		return run(append(append([]string{}, base...), "--plan-id", preview.PlanID, "--yes"))
	})
	if code == 0 || len(methods) != 2 {
		t.Fatalf("consumed plan was reused: code=%d methods=%#v", code, methods)
	}
}

func TestRemoteCleanPlanRejectsServerStateDrift(t *testing.T) {
	useTestCleanupPlansDir(t)
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	digest := strings.Repeat("b", 64)
	deleteCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/meta":
			_ = json.NewEncoder(w).Encode(map[string]any{"protocolVersion": 2, "capabilities": map[string]any{"remoteCleanup": true}})
		case "/v1/projects/project-test/cleanup":
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"projectId": "project-test", "scope": "results", "dryRun": true, "planDigest": digest,
					"results": 1, "resultBytes": 4,
				})
				return
			}
			deleteCalls++
			if r.URL.Query().Get("expectedDigest") != digest {
				http.Error(w, "wrong expected digest", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "cleanup targets changed since preview; create a new plan"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	base := []string{"rlatexmk", "remote", "clean", "--project-root", root, "--project-id", "project-test", "--server", server.URL, "--json"}
	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run(append(append([]string{}, base...), "--scope", "results"))
	})
	if code != 0 || stderr != "" {
		t.Fatalf("preview code=%d stderr=%q", code, stderr)
	}
	var preview remoteCleanupOutput
	if err := json.Unmarshal([]byte(stdout), &preview); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = captureCommandOutput(t, func() int {
		return run(append(append([]string{}, base...), "--plan-id", preview.PlanID, "--yes"))
	})
	if code == 0 || !strings.Contains(stderr, "changed since preview") || deleteCalls != 1 {
		t.Fatalf("drift result code=%d stderr=%q deleteCalls=%d", code, stderr, deleteCalls)
	}
}

func TestJobsListJSONUsesVersionedEnvelope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/jobs" || r.URL.Query().Get("limit") != "2" || r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jobs": []map[string]any{
			{"id": "job_old", "projectId": "project", "status": "succeeded", "createdAt": older},
			{"id": "job_new", "projectId": "project", "status": "queued", "createdAt": newer},
		}})
	}))
	defer server.Close()
	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "jobs", "list", "--server", server.URL, "--token", "secret-token", "--limit", "2", "--json"})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "secret-token") {
		t.Fatal("JSON output exposed the bearer token")
	}
	var envelope struct {
		SchemaVersion int    `json:"schemaVersion"`
		OK            bool   `json:"ok"`
		Command       string `json:"command"`
		Data          struct {
			Count int `json:"count"`
			Limit int `json:"limit"`
			Jobs  []struct {
				ID string `json:"id"`
			} `json:"jobs"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != 1 || !envelope.OK || envelope.Command != "jobs.list" || envelope.Data.Count != 2 || envelope.Data.Limit != 2 || envelope.Data.Jobs[0].ID != "job_new" {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestJobsJSONErrorIsStableAndDoesNotExposeToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"job not found"}`))
	}))
	defer server.Close()
	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "jobs", "show", "job_missing", "--server", server.URL, "--token", "secret-token", "--json"})
	})
	if code != 1 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "secret-token") {
		t.Fatal("JSON error exposed the bearer token")
	}
	var envelope agentJSONEnvelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Command != "jobs.show" || envelope.Error == nil || envelope.Error.Code != "not_found" || envelope.Error.Retryable || envelope.Error.Details["httpStatus"] != float64(http.StatusNotFound) {
		t.Fatalf("error envelope = %#v", envelope)
	}
}

func TestJobsInvalidArgumentsUseJSONError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "jobs", "list", "--limit", "0", "--json"})
	})
	if code != 2 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var envelope agentJSONEnvelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error == nil || envelope.Error.Code != "invalid_arguments" || envelope.Error.Retryable {
		t.Fatalf("error envelope = %#v", envelope)
	}
}
