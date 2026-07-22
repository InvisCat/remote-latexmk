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
		return run([]string{"rlatexmk", "auth", "login", "--server", strings.TrimPrefix(server.URL, "http://")})
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
	if userConfig.Server != server.URL || userConfig.Token != "" || !userConfig.TokenFileManaged {
		t.Fatalf("unexpected user config: %#v", userConfig)
	}
	wantTokenDir := filepath.Join(configHome, "remote-latexmk")
	wantTokenPath := userConfig.TokenFile
	if filepath.Dir(wantTokenPath) != wantTokenDir || !strings.HasPrefix(filepath.Base(wantTokenPath), "token-") {
		t.Fatalf("token file = %q, want a managed unique file under %q", wantTokenPath, wantTokenDir)
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

func TestAuthLoginChecksServerBeforeReadingToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	called := false
	previousReader := readLoginToken
	readLoginToken = func(string) (string, error) {
		called = true
		return "must-not-be-read", nil
	}
	t.Cleanup(func() { readLoginToken = previousReader })

	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "auth", "login", "--server", strings.TrimPrefix(server.URL, "http://")})
	})
	if code == 0 || !strings.Contains(stdout, "server:      "+server.URL) || !strings.Contains(stderr, server.URL) {
		t.Fatalf("preflight code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if called {
		t.Fatal("auth login read the token before server preflight succeeded")
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
		return run([]string{"rlatexmk", "auth", "login", "--server", server.URL})
	})
	if code == 0 || !strings.Contains(stdout, "server:      "+server.URL) || !strings.Contains(stderr, "API token verification failed") {
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

func TestAuthLoginKeepsOldCredentialsWhenConfigSwitchFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require privileges on Windows")
	}
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	oldTokenPath, err := config.WriteUserToken("old-secret")
	if err != nil {
		t.Fatal(err)
	}
	configPath, err := config.WriteUser(config.FileConfig{Server: "https://old.example", TokenFile: oldTokenPath})
	if err != nil {
		t.Fatal(err)
	}
	oldConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	newSecret := "new-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/v1/meta":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"service":"remote-latexmk","version":"test","protocolVersion":2}`))
		case "/v1/jobs":
			if r.Header.Get("Authorization") != "Bearer "+newSecret {
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
	realConfig := filepath.Join(t.TempDir(), "old-config.json")
	previousReader := readLoginToken
	readLoginToken = func(string) (string, error) {
		if err := os.WriteFile(realConfig, oldConfig, 0o600); err != nil {
			return "", err
		}
		if err := os.Remove(configPath); err != nil {
			return "", err
		}
		if err := os.Symlink(realConfig, configPath); err != nil {
			return "", err
		}
		return newSecret, nil
	}
	t.Cleanup(func() { readLoginToken = previousReader })

	code, _, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "auth", "login", "--server", server.URL})
	})
	if code == 0 || !strings.Contains(stderr, "is not a regular file") {
		t.Fatalf("config switch code=%d stderr=%q", code, stderr)
	}
	userConfig, _, err := config.ReadUserFile()
	if err != nil {
		t.Fatal(err)
	}
	if userConfig.Server != "https://old.example" || userConfig.TokenFile != oldTokenPath {
		t.Fatalf("failed config switch replaced config: %#v", userConfig)
	}
	contents, err := os.ReadFile(oldTokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "old-secret\n" {
		t.Fatal("failed config switch replaced the stored token")
	}
	tokenFiles, err := filepath.Glob(filepath.Join(configHome, "remote-latexmk", "token-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tokenFiles) != 1 || tokenFiles[0] != oldTokenPath {
		t.Fatalf("staged token was not cleaned up: %v", tokenFiles)
	}
}

func TestAuthLoginRemovesOldManagedTokenAfterConfigSwitch(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	oldTokenPath, err := config.WriteUserToken("old-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.WriteUser(config.FileConfig{Server: "https://old.example", TokenFile: oldTokenPath, TokenFileManaged: true}); err != nil {
		t.Fatal(err)
	}

	newSecret := "new-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/v1/meta":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"service":"remote-latexmk","version":"test","protocolVersion":2}`))
		case "/v1/jobs":
			if r.Header.Get("Authorization") != "Bearer "+newSecret {
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
	previousReader := readLoginToken
	readLoginToken = func(string) (string, error) { return newSecret, nil }
	t.Cleanup(func() { readLoginToken = previousReader })

	code, _, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "auth", "login", "--server", server.URL})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("config switch code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(oldTokenPath); !os.IsNotExist(err) {
		t.Fatalf("old managed token still exists: %v", err)
	}
	userConfig, _, err := config.ReadUserFile()
	if err != nil {
		t.Fatal(err)
	}
	if userConfig.TokenFile == oldTokenPath || !strings.HasPrefix(filepath.Base(userConfig.TokenFile), "token-") {
		t.Fatalf("new token path = %q", userConfig.TokenFile)
	}
	contents, err := os.ReadFile(userConfig.TokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != newSecret+"\n" {
		t.Fatal("new managed token content does not match")
	}
}

func TestAuthLoginPreservesUserManagedTokenWithManagedLookingName(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("LATEXMK_SERVER", "")
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	oldTokenPath := filepath.Join(configHome, "remote-latexmk", "token-123")
	if err := os.MkdirAll(filepath.Dir(oldTokenPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldTokenPath, []byte("user-managed-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.WriteUser(config.FileConfig{
		Server:           "https://old.example",
		TokenFile:        oldTokenPath,
		TokenFileManaged: false,
	}); err != nil {
		t.Fatal(err)
	}

	newSecret := "new-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/v1/meta":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"service":"remote-latexmk","version":"test","protocolVersion":2}`))
		case "/v1/jobs":
			if r.Header.Get("Authorization") != "Bearer "+newSecret {
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
	previousReader := readLoginToken
	readLoginToken = func(string) (string, error) { return newSecret, nil }
	t.Cleanup(func() { readLoginToken = previousReader })

	code, _, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "auth", "login", "--server", server.URL})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("config switch code=%d stderr=%q", code, stderr)
	}
	contents, err := os.ReadFile(oldTokenPath)
	if err != nil {
		t.Fatalf("user-managed token was removed: %v", err)
	}
	if string(contents) != "user-managed-secret\n" {
		t.Fatalf("user-managed token changed: %q", contents)
	}
}

func TestAuthLoginRejectsTokenArgument(t *testing.T) {
	code, _, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "auth", "login", "--server", "https://latex.example", "--token", "secret"})
	})
	if code != 2 || !strings.Contains(stderr, "hidden prompt") {
		t.Fatalf("raw token code=%d stderr=%q", code, stderr)
	}
}
