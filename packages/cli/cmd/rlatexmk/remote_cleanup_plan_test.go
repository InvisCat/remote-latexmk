package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/billstark001/latexmk/packages/cli/internal/protocol"
)

func TestRemoteCleanApplyValidatesServerProjectAndTTLBeforeNetwork(t *testing.T) {
	useTestCleanupPlansDir(t)
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LATEXMK_TOKEN", "")
	t.Setenv("LATEXMK_TOKEN_FILE", "")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()
	digest := strings.Repeat("c", 64)
	report := protocol.CleanupReport{
		ProjectID: "project-a", Scope: "results", DryRun: true, PlanDigest: digest,
	}
	plan, err := createRemoteCleanupPlan(server.URL, "project-a", "results", report)
	if err != nil {
		t.Fatal(err)
	}

	base := []string{"rlatexmk", "remote", "clean", "--project-root", root, "--plan-id", plan.ID, "--yes"}
	code, _, stderr := captureCommandOutput(t, func() int {
		return run(append(append([]string{}, base...), "--project-id", "project-b", "--server", server.URL))
	})
	if code == 0 || !strings.Contains(stderr, "different project") {
		t.Fatalf("project mismatch code=%d stderr=%q", code, stderr)
	}
	code, _, stderr = captureCommandOutput(t, func() int {
		return run(append(append([]string{}, base...), "--project-id", "project-a", "--server", "http://127.0.0.1:1"))
	})
	if code == 0 || !strings.Contains(stderr, "different server") {
		t.Fatalf("server mismatch code=%d stderr=%q", code, stderr)
	}

	expiredID, err := newCleanupPlanID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	expired := remoteCleanupPlan{
		Version: cleanupPlanVersion, Kind: remoteCleanupPlanKind, ID: expiredID,
		Server: server.URL, ProjectID: "project-a", Scope: "results", PlanDigest: digest,
		CreatedAt: now.Add(-2 * cleanupPlanTTL), ExpiresAt: now.Add(-cleanupPlanTTL),
	}
	if err := saveCleanupPlanData(expired.ID, expired); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = captureCommandOutput(t, func() int {
		args := []string{"rlatexmk", "remote", "clean", "--project-root", root, "--project-id", "project-a", "--server", server.URL, "--plan-id", expired.ID, "--yes"}
		return run(args)
	})
	if code == 0 || !strings.Contains(stderr, "has expired") {
		t.Fatalf("expired plan code=%d stderr=%q", code, stderr)
	}
	if requests != 0 {
		t.Fatalf("context validation made %d unexpected request(s)", requests)
	}
}
