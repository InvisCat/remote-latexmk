package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/billstark001/latexmk/packages/cli/internal/config"
)

func TestSetupPreviewsThenWritesUserConfigWithoutTokenLeak(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	t.Setenv("LATEXMK_CA_FILE", "")
	token := "secret-token-must-not-appear"
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolvedTokenPath, err := filepath.EvalSymlinks(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"latexmk", "setup", "--server", "https://latex.example", "--token-file", tokenPath, "--json"}
	code, stdout, stderr := captureCommandOutput(t, func() int { return run(args) })
	if code != 0 || stderr != "" {
		t.Fatalf("preview code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if strings.Contains(stdout, token) {
		t.Fatalf("preview leaked token: %s", stdout)
	}
	var preview agentJSONEnvelope
	if err := json.Unmarshal([]byte(stdout), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Command != "setup.preview" {
		t.Fatalf("preview command = %q", preview.Command)
	}
	configPath, err := config.UserConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("preview wrote config: %v", err)
	}

	applyArgs := append(append([]string{}, args...), "--yes")
	code, stdout, stderr = captureCommandOutput(t, func() int { return run(applyArgs) })
	if code != 0 || stderr != "" {
		t.Fatalf("apply code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if strings.Contains(stdout, token) {
		t.Fatalf("apply leaked token: %s", stdout)
	}
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), token) || !strings.Contains(string(contents), `"tokenFile"`) {
		t.Fatalf("unsafe user config: %s", contents)
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o", info.Mode().Perm())
	}
	loaded, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server != "https://latex.example" || loaded.Token != token || loaded.TokenFile != resolvedTokenPath {
		t.Fatalf("loaded config = %#v", loaded)
	}
}

func TestSetupRejectsRawOrBroadlyReadableToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	code, _, stderr := captureCommandOutput(t, func() int {
		return run([]string{"latexmk", "setup", "--server", "https://latex.example", "--token", "secret"})
	})
	if code != 2 || !strings.Contains(stderr, "raw tokens are not accepted") {
		t.Fatalf("raw token code=%d stderr=%q", code, stderr)
	}
	if runtime.GOOS == "windows" {
		return
	}
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = captureCommandOutput(t, func() int {
		return run([]string{"latexmk", "setup", "--server", "https://latex.example", "--token-file", tokenPath})
	})
	if code != 2 || !strings.Contains(stderr, "chmod 600") {
		t.Fatalf("broad token code=%d stderr=%q", code, stderr)
	}
}

func TestSetupRejectsDryRunWithApply(t *testing.T) {
	code, _, stderr := captureCommandOutput(t, func() int {
		return run([]string{"latexmk", "setup", "--dry-run", "--yes"})
	})
	if code != 2 || !strings.Contains(stderr, "--dry-run and --yes cannot be combined") {
		t.Fatalf("dry-run apply code=%d stderr=%q", code, stderr)
	}
}
