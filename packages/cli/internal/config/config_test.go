package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func isolateUserConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_CA_FILE", "")
	t.Setenv("LATEXMK_UPLOAD_MODE", "")
	t.Setenv("LATEXMK_MANIFEST_FILE", "")
	return dir
}

func TestLoadDefaultsToAutomaticDependencySelection(t *testing.T) {
	isolateUserConfig(t)
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UploadMode != "auto" {
		t.Fatalf("upload mode = %q, want auto", cfg.UploadMode)
	}
}

func TestLoadLocalPolicyDoesNotReadTokenFiles(t *testing.T) {
	isolateUserConfig(t)
	missing := filepath.Join(t.TempDir(), "missing-token")
	if _, err := WriteUser(FileConfig{Server: "https://latex.example", TokenFile: missing}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(`{"tokenFile":"also-missing","uploadMode":"all"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := LoadLocalPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Token != "" || policy.TokenFile != "" || policy.UploadMode != "all" {
		t.Fatalf("local policy = %#v", policy)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("normal config load unexpectedly ignored the missing token file")
	}
}

func TestLoadRejectsUnknownUploadMode(t *testing.T) {
	isolateUserConfig(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(`{"uploadMode":"legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("expected invalid uploadMode to fail")
	}
}

func TestLoadManifestConfiguration(t *testing.T) {
	isolateUserConfig(t)
	root := t.TempDir()
	configJSON := `{"uploadMode":"manifest","manifestFile":".latexmk-files","includeFiles":["chapter.tex"]}`
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(configJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UploadMode != "manifest" || cfg.ManifestFile != ".latexmk-files" || len(cfg.IncludeFiles) != 1 || cfg.IncludeFiles[0] != "chapter.tex" {
		t.Fatalf("manifest config = %#v", cfg)
	}
}

func TestLoadDefaultsToEntryRootModeWithoutExpandingToGitRoot(t *testing.T) {
	isolateUserConfig(t)
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repo, "papers", "demo")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LATEXMK_ROOT_MODE", "")
	cfg, err := Load(nested)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RootMode != "entry" {
		t.Fatalf("root mode = %q, want entry", cfg.RootMode)
	}
	if cfg.ProjectRoot != "" {
		t.Fatalf("project root = %q, want empty", cfg.ProjectRoot)
	}
}

func TestFindGitRoot(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := FindGitRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	if root != repo {
		t.Fatalf("root = %q, want %q", root, repo)
	}
}

func TestDefaultDenyIncludesSensitiveLocalConfiguration(t *testing.T) {
	want := map[string]bool{".latexmk.json": false, ".latexmk-cache": false, ".latexmk-files": false, ".env": false, "*.key": false, "*.pem": false}
	for _, pattern := range DefaultDeny() {
		if _, ok := want[pattern]; ok {
			want[pattern] = true
		}
	}
	for pattern, found := range want {
		if !found {
			t.Errorf("default excludes missing %q", pattern)
		}
	}
}

func TestLoadKeepsDefaultDenyWhenProjectReplacesExcludes(t *testing.T) {
	isolateUserConfig(t)
	root := t.TempDir()
	configJSON := `{"server":"http://127.0.0.1:8080","exclude":["custom.tmp"]}`
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(configJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"custom.tmp": false, FileName: false, ".env": false, "*.key": false}
	for _, pattern := range cfg.Exclude {
		if _, ok := want[pattern]; ok {
			want[pattern] = true
		}
	}
	for pattern, found := range want {
		if !found {
			t.Errorf("resolved excludes missing %q", pattern)
		}
	}
}

func TestLoadRespectsExplicitGitIgnoreSetting(t *testing.T) {
	isolateUserConfig(t)
	root := t.TempDir()
	configJSON := `{"server":"http://127.0.0.1:8080","respectGitignore":false}`
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(configJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RespectGitIgnore {
		t.Fatal("respectGitignore = true, want false")
	}
}

func TestLoadTokenPrecedence(t *testing.T) {
	configHome := isolateUserConfig(t)
	root := t.TempDir()
	userDir := filepath.Join(configHome, "latexmk")
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, UserFileName), []byte(`{"token":"user-token","server":"https://user.example"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(`{"token":"project-token","server":"https://project.example"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "user-token" {
		t.Fatalf("token = %q, want user-token", cfg.Token)
	}
	if cfg.Server != "https://user.example" {
		t.Fatalf("server = %q, want the endpoint bound to user credentials", cfg.Server)
	}

	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LATEXMK_TOKEN_FILE", tokenFile)
	cfg, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "file-token" {
		t.Fatalf("token = %q, want file-token", cfg.Token)
	}

	t.Setenv("LATEXMK_TOKEN", "environment-token")
	cfg, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "environment-token" {
		t.Fatalf("token = %q, want environment-token", cfg.Token)
	}
}

func TestReadTokenFileRejectsMultipleLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("first\nsecond\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTokenFile(path); err == nil {
		t.Fatal("expected multiple token lines to be rejected")
	}
}

func TestLoadPrefersPrimaryUserConfigAndReadsTokenFile(t *testing.T) {
	configHome := isolateUserConfig(t)
	primaryDir := filepath.Join(configHome, userConfigDir)
	legacyDir := filepath.Join(configHome, legacyUserConfigDir)
	if err := os.MkdirAll(primaryDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(primaryDir, "token")
	if err := os.WriteFile(tokenPath, []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(primaryDir, UserFileName), []byte(`{"server":"https://primary.example","tokenFile":"token"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, UserFileName), []byte(`{"server":"https://legacy.example","token":"legacy-token"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != "https://primary.example" || cfg.Token != "file-token" || cfg.TokenFile != tokenPath {
		t.Fatalf("resolved primary config = %#v", cfg)
	}
}

func TestLoadBoundedDoesNotReadParentProjectConfig(t *testing.T) {
	isolateUserConfig(t)
	parent := t.TempDir()
	workspace := filepath.Join(parent, "paper")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, FileName), []byte(`{"server":"https://parent.example"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	unbounded, err := Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if unbounded.Server != "https://parent.example" {
		t.Fatalf("unbounded server = %q", unbounded.Server)
	}
	bounded, err := LoadBounded(workspace, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if bounded.Server != "http://127.0.0.1:8080" || bounded.ConfigPath != "" {
		t.Fatalf("bounded config = %#v", bounded)
	}
}

func TestLoadBoundedDoesNotLetProjectRedirectUserCredentials(t *testing.T) {
	configHome := isolateUserConfig(t)
	userDir := filepath.Join(configHome, userConfigDir)
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(userDir, "token")
	if err := os.WriteFile(tokenPath, []byte("user-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	userConfig := `{"server":"https://private.example","tokenFile":"token","caFile":"private-ca.pem"}`
	if err := os.WriteFile(filepath.Join(userDir, UserFileName), []byte(userConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	projectConfig := `{"server":"https://attacker.example","token":"attacker-token","tokenFile":"../../secret","caFile":"attacker-ca.pem","insecureSkipVerify":true,"engine":"pdflatex"}`
	if err := os.WriteFile(filepath.Join(workspace, FileName), []byte(projectConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadBounded(workspace, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != "https://private.example" || cfg.Token != "user-token" || cfg.TokenFile != tokenPath {
		t.Fatalf("Agent connection config was redirected: %#v", cfg)
	}
	if cfg.CAFile != filepath.Join(userDir, "private-ca.pem") || cfg.InsecureSkipVerify {
		t.Fatalf("Agent TLS config was redirected: %#v", cfg)
	}
	if cfg.Engine != "pdflatex" {
		t.Fatalf("safe project setting was ignored: engine=%q", cfg.Engine)
	}
}

func TestLoadBoundedRejectsSymlinkedProjectConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require privileges on Windows")
	}
	isolateUserConfig(t)
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"engine":"pdflatex"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, FileName)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBounded(workspace, workspace); err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("symlinked project config error = %v", err)
	}
}

func TestWriteUserUsesPrimaryPathAndPrivatePermissions(t *testing.T) {
	configHome := isolateUserConfig(t)
	path, err := WriteUser(FileConfig{Server: "https://latex.example", TokenFile: "/secure/token"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(configHome, userConfigDir, UserFileName)
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("config directory mode = %o", dirInfo.Mode().Perm())
	}
}

func TestWriteUserTokenCreatesUniqueManagedFiles(t *testing.T) {
	configHome := isolateUserConfig(t)
	first, err := WriteUserToken("first-token")
	if err != nil {
		t.Fatal(err)
	}
	second, err := WriteUserToken("second-token")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("token paths are not unique: %q", first)
	}
	for _, path := range []string{first, second} {
		if filepath.Dir(path) != filepath.Join(configHome, userConfigDir) || !managedTokenFileName(filepath.Base(path)) {
			t.Fatalf("unmanaged token path %q", path)
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("token mode = %o", info.Mode().Perm())
			}
		}
	}
	if err := RemoveManagedUserToken(first); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("managed token still exists: %v", err)
	}
	contents, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "second-token\n" {
		t.Fatalf("second token = %q", contents)
	}
}

func TestRemoveManagedUserTokenLeavesExternalFiles(t *testing.T) {
	configHome := isolateUserConfig(t)
	external := filepath.Join(t.TempDir(), "token-external")
	if err := os.WriteFile(external, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveManagedUserToken(external); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatalf("external token was removed: %v", err)
	}
	custom := filepath.Join(configHome, userConfigDir, "token-custom")
	if err := os.MkdirAll(filepath.Dir(custom), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(custom, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveManagedUserToken(custom); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Fatalf("custom token was removed: %v", err)
	}
}
