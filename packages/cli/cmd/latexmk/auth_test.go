package main

import (
	"net/http"
	"net/http/httptest"
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/v1/meta":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"service":"remote-latexmk","version":"test","protocolVersion":2}`))
		case "/v1/jobs":
			if r.Header.Get("Authorization") != "Bearer "+secret {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jobs":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	prompt := ""
	previousReader := readLoginToken
	readLoginToken = func(value string) (string, error) {
		prompt = value
		return secret, nil
	}
	t.Cleanup(func() { readLoginToken = previousReader })

	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"remote-latexmk", "auth", "login", "--server", server.URL})
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
	if userConfig.Server != server.URL || userConfig.Token != "" {
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
	if !strings.Contains(stdout, "verified:    remote-latexmk test (protocol v2)") {
		t.Fatalf("login did not report verification: %s", stdout)
	}
}

func TestAuthLoginDoesNotReplaceCredentialsWhenVerificationFails(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	oldTokenPath, err := config.WriteUserToken("old-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.WriteUser(config.FileConfig{
		Server:    "https://old.example",
		TokenFile: oldTokenPath,
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/v1/meta":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"service":"remote-latexmk","version":"test","protocolVersion":2}`))
		case "/v1/jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	previousReader := readLoginToken
	readLoginToken = func(string) (string, error) { return "wrong-secret", nil }
	t.Cleanup(func() { readLoginToken = previousReader })

	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"remote-latexmk", "auth", "login", "--server", server.URL})
	})
	if code == 0 || stdout != "" || !strings.Contains(stderr, "API token verification failed") {
		t.Fatalf("login code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if strings.Contains(stderr, "wrong-secret") {
		t.Fatalf("login printed the rejected token: %s", stderr)
	}
	userConfig, _, err := config.ReadUserFile()
	if err != nil {
		t.Fatal(err)
	}
	if userConfig.Server != "https://old.example" || userConfig.TokenFile != oldTokenPath {
		t.Fatalf("failed login replaced config: %#v", userConfig)
	}
	contents, err := os.ReadFile(oldTokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "old-secret\n" {
		t.Fatal("failed login replaced the stored token")
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
