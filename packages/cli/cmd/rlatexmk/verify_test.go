package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/billstark001/latexmk/packages/cli/internal/client"
)

func newAccessTestServer(t *testing.T, expectedToken string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/v1/meta":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"service":"remote-latexmk","version":"test","protocolVersion":2}`))
		case "/v1/jobs":
			if r.URL.Query().Get("limit") != "1" || r.Header.Get("Authorization") != "Bearer "+expectedToken {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jobs":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func chdirTestTemp(t *testing.T) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}

func TestDoctorVerifiesConfiguredAuthentication(t *testing.T) {
	server := newAccessTestServer(t, "secret-token")
	chdirTestTemp(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_SERVER", server.URL)
	t.Setenv("LATEXMK_TOKEN", "secret-token")
	t.Setenv("LATEXMK_TOKEN_FILE", "")

	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "doctor", "--json"})
	})
	if code != 0 || stderr != "" {
		t.Fatalf("doctor code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(stdout), &metadata); err != nil {
		t.Fatalf("doctor output is not JSON: %q: %v", stdout, err)
	}
	if metadata["service"] != "remote-latexmk" {
		t.Fatalf("doctor metadata = %#v", metadata)
	}
}

func TestDoctorRejectsInvalidConfiguredToken(t *testing.T) {
	server := newAccessTestServer(t, "secret-token")
	chdirTestTemp(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_SERVER", server.URL)
	t.Setenv("LATEXMK_TOKEN", "wrong-token")
	t.Setenv("LATEXMK_TOKEN_FILE", "")

	code, stdout, stderr := captureCommandOutput(t, func() int {
		return run([]string{"rlatexmk", "doctor", "--json"})
	})
	if code == 0 || stdout != "" || !strings.Contains(stderr, "API token verification failed") {
		t.Fatalf("doctor code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if strings.Contains(stderr, "wrong-token") {
		t.Fatalf("doctor printed the rejected token: %s", stderr)
	}
}

func TestMCPServerStatusVerifiesConfiguredAuthentication(t *testing.T) {
	root := t.TempDir()
	remote := newAccessTestServer(t, "secret-token")
	c, err := client.New(remote.URL, "secret-token", time.Second, false, "")
	if err != nil {
		t.Fatal(err)
	}
	server := newStdioMCPServer(&bytes.Buffer{}, &bytes.Buffer{}, root, c, "xelatex", time.Second)
	result, err := server.callTool("server_status", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	status, ok := result.(map[string]any)
	if !ok || status["healthy"] != true || status["accessVerified"] != true {
		t.Fatalf("server status = %#v", result)
	}
}
