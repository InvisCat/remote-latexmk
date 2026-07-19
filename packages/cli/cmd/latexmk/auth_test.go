package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/billstark001/latexmk/packages/cli/internal/config"
)

func TestAuthLoginStoresTokenOutsideConfigWithoutEcho(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	secret := "secret-token-that-must-not-be-printed"
	prompt := ""
	previousReader := readLoginToken
	readLoginToken = func(value string) (string, error) {
		prompt = value
		return secret, nil
	}
	t.Cleanup(func() { readLoginToken = previousReader })

	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"remote-latexmk", "auth", "login", "--server", "https://latex.example"})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("login code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if strings.Contains(stdout, secret) {
		t.Fatalf("login printed the token: %s", stdout)
	}
	if prompt != "remote-latexmk API token: " {
		t.Fatalf("login prompt = %q", prompt)
	}

	userConfig, _, err := config.ReadUserFile()
	if err != nil {
		t.Fatal(err)
	}
	if userConfig.Server != "https://latex.example" || userConfig.Token != "" {
		t.Fatalf("unexpected user config: %#v", userConfig)
	}
	wantTokenPath := filepath.Join(configHome, "remote-latexmk", "token")
	if userConfig.TokenFile != wantTokenPath {
		t.Fatalf("token file = %q, want %q", userConfig.TokenFile, wantTokenPath)
	}
	contents, err := os.ReadFile(wantTokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != secret+"\n" {
		t.Fatal("stored token does not match hidden input")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(wantTokenPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("token mode = %o", info.Mode().Perm())
		}
	}
}

func TestAuthLoginRejectsTokenArgument(t *testing.T) {
	code, _, stderr := captureCommandOutput(t, func() int {
		return run([]string{"remote-latexmk", "auth", "login", "--server", "https://latex.example", "--token", "secret"})
	})
	if code != 2 || !strings.Contains(stderr, "hidden prompt") {
		t.Fatalf("raw token code=%d stderr=%q", code, stderr)
	}
}
