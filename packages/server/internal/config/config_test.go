package config

import "testing"

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
