package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInvalidLimitFailsFast(t *testing.T) {
	t.Setenv("LATEXMK_MAX_FILES", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid environment variable error")
	}
}

func TestTokenMustBeLong(t *testing.T) {
	t.Setenv("LATEXMK_AUTH_MODE", "token")
	t.Setenv("LATEXMK_API_TOKEN", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected short token error")
	}
}

func TestTokenCanBeReadFromFile(t *testing.T) {
	t.Setenv("LATEXMK_AUTH_MODE", "token")
	t.Setenv("LATEXMK_API_TOKEN", "")
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("0123456789abcdef0123456789abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LATEXMK_API_TOKEN_FILE", path)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIToken != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected token %q", cfg.APIToken)
	}
}

func TestTokenEnvironmentAndFileAreMutuallyExclusive(t *testing.T) {
	t.Setenv("LATEXMK_API_TOKEN", "0123456789abcdef0123456789abcdef")
	t.Setenv("LATEXMK_API_TOKEN_FILE", filepath.Join(t.TempDir(), "token"))
	if _, err := Load(); err == nil {
		t.Fatal("expected conflicting token settings to fail")
	}
}

func TestValidOriginRejectsWildcardsAndPaths(t *testing.T) {
	for _, origin := range []string{"*", "https://console.example.edu/path", "ftp://console.example.edu", "https://user:pass@console.example.edu"} {
		if validOrigin(origin) {
			t.Fatalf("expected invalid origin %q", origin)
		}
	}
	if !validOrigin("https://console.example.edu:8443") {
		t.Fatal("expected exact HTTPS origin to be valid")
	}
}

func TestToolchainPathRequiresAbsoluteDirectories(t *testing.T) {
	t.Setenv("LATEXMK_AUTH_MODE", "none")
	t.Setenv("LATEXMK_TOOLCHAIN_PATH", "relative/bin:/usr/bin")
	if _, err := Load(); err == nil {
		t.Fatal("expected relative toolchain path to fail")
	}
}
