package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/billstark001/latexmk/packages/server/internal/api"
	"github.com/billstark001/latexmk/packages/server/internal/auth"
	"github.com/billstark001/latexmk/packages/server/internal/compile"
	"github.com/billstark001/latexmk/packages/server/internal/config"
	"github.com/billstark001/latexmk/packages/server/internal/jobs"
	"github.com/billstark001/latexmk/packages/server/internal/project"
)

func TestProjectCleanupRoutesPreviewAndDelete(t *testing.T) {
	cfg := config.Config{
		AuthMode: "none", StateDir: t.TempDir(), Engines: []string{"xelatex"},
		MaxFiles: 10, MaxExpandedBytes: 1024, MaxStateBytes: 4096,
		MaxQueuedJobs: 2, MaxConcurrentCompiles: 1,
	}
	projects, err := project.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := compile.NewRunner(cfg)
	queue := jobs.New(cfg, api.Metadata{}, runner, projects, nil, logger)
	server := New(cfg, api.Metadata{}, runner, auth.New(cfg, nil), nil, projects, queue, logger)
	healthRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK || !strings.Contains(healthResponse.Body.String(), `"service":"remote-latexmk"`) {
		t.Fatalf("health response = %d %s", healthResponse.Code, healthResponse.Body.String())
	}

	for _, test := range []struct {
		method string
		dryRun bool
	}{
		{method: http.MethodGet, dryRun: true},
		{method: http.MethodDelete, dryRun: false},
	} {
		req := httptest.NewRequest(test.method, "/v1/projects/project-test/cleanup?scope=project", nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("%s cleanup status = %d, body=%s", test.method, response.Code, response.Body.String())
		}
		var report api.CleanupReport
		if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if report.ProjectID != "project-test" || report.Scope != "project" || report.DryRun != test.dryRun {
			t.Fatalf("%s cleanup report = %#v", test.method, report)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/invalid%20id/cleanup?scope=project", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid ID status = %d, body=%s", response.Code, response.Body.String())
	}
}
