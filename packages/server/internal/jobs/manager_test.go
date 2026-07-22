package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/billstark001/latexmk/packages/server/internal/api"
	"github.com/billstark001/latexmk/packages/server/internal/compile"
	"github.com/billstark001/latexmk/packages/server/internal/config"
	"github.com/billstark001/latexmk/packages/server/internal/project"
	"github.com/billstark001/latexmk/packages/server/internal/store"
)

func TestQueueAcceptsMultipleJobsAndAllowsQueuedCancellation(t *testing.T) {
	cfg := config.Config{
		StateDir: t.TempDir(), Engines: []string{"xelatex"}, MaxFiles: 10,
		MaxExpandedBytes: 1024, MaxConcurrentCompiles: 1, MaxQueuedJobs: 2,
		MaxStateBytes: 1024,
	}
	projects, err := project.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("\\documentclass{article}")
	digest := sha256.Sum256(content)
	sha := hex.EncodeToString(digest[:])
	request := api.CompileRequest{ProtocolVersion: api.ProtocolVersion, Entry: "main.tex", Engine: "xelatex", Interaction: "nonstopmode"}
	plan, err := projects.Plan("member", api.UploadPlanRequest{ProjectID: "paper", Request: request, Files: []api.ProjectFile{{Path: "main.tex", SHA256: sha, Size: int64(len(content))}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := projects.PutBlob("member", plan.UploadID, sha, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := projects.Commit(context.Background(), "member", plan.UploadID)
	if err != nil {
		t.Fatal(err)
	}
	manager := New(cfg, api.Metadata{}, compile.NewRunner(cfg), projects, nil, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	first, err := manager.Enqueue(context.Background(), "member", snapshot, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Enqueue(context.Background(), "member", snapshot, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Enqueue(context.Background(), "member", snapshot, request); err == nil {
		t.Fatal("expected bounded queue to reject a third job")
	}
	cancelled, err := manager.Cancel(context.Background(), "member", second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("cancelled job has status %q", cancelled.Status)
	}
	if got, err := manager.Get(context.Background(), "member", first.ID); err != nil || got.Status != "queued" {
		t.Fatalf("first job = %#v, %v", got, err)
	}
}

func TestQueuedTransitionCannotOverwriteCancellation(t *testing.T) {
	cfg := config.Config{MaxConcurrentCompiles: 1, MaxQueuedJobs: 2}
	manager := &Manager{cfg: cfg, jobs: make(map[string]record), logger: slog.New(slog.NewTextHandler(testWriter{t}, nil))}
	now := time.Now().UTC()
	original := record{OwnerID: "member", Job: api.Job{ID: "job_race", Status: "queued", CreatedAt: now}}
	manager.jobs[original.Job.ID] = original

	staleWorkerCopy := original
	cancelledCopy := original
	finished := time.Now().UTC()
	cancelledCopy.Job.Status = "cancelled"
	cancelledCopy.Job.Error = "cancelled by user"
	cancelledCopy.Job.FinishedAt = &finished
	changed, err := manager.transition(context.Background(), cancelledCopy, "queued")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected cancellation transition to win")
	}
	started := time.Now().UTC()
	staleWorkerCopy.Job.Status = "running"
	staleWorkerCopy.Job.StartedAt = &started
	changed, err = manager.transition(context.Background(), staleWorkerCopy, "queued")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("stale worker transition overwrote cancellation")
	}
	got, err := manager.load(context.Background(), original.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Job.Status != "cancelled" {
		t.Fatalf("job status = %q, want cancelled", got.Job.Status)
	}
}

func TestSuccessfulCompileRequiresArchivedResult(t *testing.T) {
	cfg := config.Config{StateDir: t.TempDir(), MaxStateBytes: 4096, ShutdownTimeout: time.Second}
	projects, err := project.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager := New(cfg, api.Metadata{}, compile.NewRunner(config.Config{MaxConcurrentCompiles: 1}), projects, nil, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	now := time.Now().UTC()
	rec := record{OwnerID: "member", Job: api.Job{ID: "job_archive_failed", Status: "running", CreatedAt: now}}
	if err := manager.save(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	result := &api.CompileResult{Success: true, ExitCode: 0}
	manager.finish(context.Background(), rec, result, "could not package compile result", false)
	got, err := manager.Get(context.Background(), "member", rec.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.Error != "could not package compile result" || got.Result == nil || got.Result.Success {
		t.Fatalf("job = %#v, want failed packaging status", got)
	}
}

func TestPruneTerminalJobsUsesResultRetentionCutoff(t *testing.T) {
	manager := &Manager{jobs: make(map[string]record), logger: slog.New(slog.NewTextHandler(testWriter{t}, nil))}
	now := time.Now().UTC()
	old := now.Add(-2 * time.Hour)
	recent := now.Add(-30 * time.Minute)
	manager.jobs["old"] = record{Job: api.Job{ID: "old", Status: "succeeded", FinishedAt: &old}}
	manager.jobs["recent"] = record{Job: api.Job{ID: "recent", Status: "failed", FinishedAt: &recent}}
	manager.jobs["active"] = record{Job: api.Job{ID: "active", Status: "queued", CreatedAt: old}}

	manager.pruneTerminal(context.Background(), now.Add(-time.Hour))
	if _, ok := manager.jobs["old"]; ok {
		t.Fatal("expired terminal job was retained")
	}
	if _, ok := manager.jobs["recent"]; !ok {
		t.Fatal("recent terminal job was removed")
	}
	if _, ok := manager.jobs["active"]; !ok {
		t.Fatal("active job was removed")
	}
}

func TestWaitReturnsAfterWorkerContextIsCancelled(t *testing.T) {
	cfg := config.Config{
		StateDir: t.TempDir(), Engines: []string{"xelatex"}, MaxConcurrentCompiles: 1,
		MaxQueuedJobs: 1, MaxStateBytes: 1024, ResultRetention: time.Hour,
		StateSweepInterval: time.Hour,
	}
	projects, err := project.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager := New(cfg, api.Metadata{}, compile.NewRunner(cfg), projects, nil, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	manager.Start(workerCtx)
	cancelWorkers()
	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	if err := manager.Wait(waitCtx); err != nil {
		t.Fatalf("wait for cancelled worker: %v", err)
	}
}

func TestQueuedJobKeepsSnapshotCapturedAtEnqueue(t *testing.T) {
	cfg := config.Config{
		StateDir: t.TempDir(), Engines: []string{"xelatex"}, MaxFiles: 10,
		MaxUploadBytes: 1024, MaxExpandedBytes: 1024, MaxConcurrentCompiles: 1,
		MaxQueuedJobs: 2, MaxStateBytes: 4096,
	}
	projects, err := project.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := api.CompileRequest{ProtocolVersion: api.ProtocolVersion, Entry: "main.tex", Engine: "xelatex", Interaction: "nonstopmode"}
	first := commitTestSnapshot(t, projects, request, []byte("first version"))
	manager := New(cfg, api.Metadata{}, compile.NewRunner(cfg), projects, nil, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	job, err := manager.Enqueue(context.Background(), "member", first, request)
	if err != nil {
		t.Fatal(err)
	}
	second := commitTestSnapshot(t, projects, request, []byte("second version"))
	if first.ID == second.ID {
		t.Fatal("different manifests received the same snapshot ID")
	}

	rec, err := manager.load(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Snapshot.ID != first.ID || rec.Job.SnapshotID != first.ID {
		t.Fatalf("queued snapshot = %q/%q, want %q", rec.Snapshot.ID, rec.Job.SnapshotID, first.ID)
	}
	workspace := t.TempDir()
	if err := projects.Materialize(rec.Snapshot, workspace); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "main.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "first version" {
		t.Fatalf("queued job materialized %q, want first version", content)
	}
}

func TestLegacyFinishedJobWithoutSnapshotRemainsReadable(t *testing.T) {
	row := store.CompileJob{
		ID: "job_legacy", OwnerID: "member", ProjectID: "paper", Status: "succeeded",
		Request: []byte(`{"protocolVersion":2,"entry":"main.tex","engine":"xelatex"}`),
		Result:  []byte(`{"protocolVersion":2,"requestId":"job_legacy","success":true,"exitCode":0}`),
	}
	rec, err := recordFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Job.Status != "succeeded" || rec.Job.Result == nil || !rec.Job.Result.Success {
		t.Fatalf("legacy job was not decoded: %#v", rec.Job)
	}
	row.Status = "queued"
	if _, err := recordFromRow(row); err == nil {
		t.Fatal("expected active legacy job without snapshot to be rejected")
	}
}

func TestCleanupProjectPreviewsAndDeletesTerminalState(t *testing.T) {
	cfg := config.Config{
		StateDir: t.TempDir(), Engines: []string{"xelatex"}, MaxFiles: 10,
		MaxUploadBytes: 1024, MaxExpandedBytes: 1024, MaxConcurrentCompiles: 1,
		MaxQueuedJobs: 2, MaxStateBytes: 4096,
	}
	projects, err := project.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := api.CompileRequest{ProtocolVersion: api.ProtocolVersion, Entry: "main.tex", Engine: "xelatex", Interaction: "nonstopmode"}
	snapshot := commitTestSnapshot(t, projects, request, []byte("private paper source"))
	manager := New(cfg, api.Metadata{}, compile.NewRunner(cfg), projects, nil, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	now := time.Now().UTC()
	rec := record{
		OwnerID: "member", Request: request, Snapshot: snapshot,
		Job: api.Job{ID: "job_finished", ProjectID: snapshot.ProjectID, SnapshotID: snapshot.ID, Status: "succeeded", CreatedAt: now, FinishedAt: &now},
	}
	if err := manager.save(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	resultPath, err := projects.ResultPath("member", rec.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, []byte("result archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	preview, err := manager.CleanupProject(context.Background(), "member", snapshot.ProjectID, "project", true)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.DryRun || !preview.SnapshotPresent || preview.SnapshotFiles != 1 || preview.Jobs != 1 || preview.Results != 1 {
		t.Fatalf("cleanup preview = %#v", preview)
	}
	if _, err := os.Stat(resultPath); err != nil {
		t.Fatalf("preview changed result: %v", err)
	}

	results, err := manager.CleanupProject(context.Background(), "member", snapshot.ProjectID, "results", false)
	if err != nil {
		t.Fatal(err)
	}
	if results.ReclaimedBytes != int64(len("result archive")) {
		t.Fatalf("result cleanup = %#v", results)
	}
	if _, err := manager.Get(context.Background(), "member", rec.Job.ID); err != nil {
		t.Fatalf("result cleanup removed job metadata: %v", err)
	}
	if _, err := os.Stat(resultPath); !os.IsNotExist(err) {
		t.Fatalf("result cleanup kept archive: %v", err)
	}

	cleaned, err := manager.CleanupProject(context.Background(), "member", snapshot.ProjectID, "project", false)
	if err != nil {
		t.Fatal(err)
	}
	if cleaned.Jobs != 1 || !cleaned.SnapshotPresent || cleaned.ReclaimedBytes < int64(len("private paper source")) {
		t.Fatalf("project cleanup = %#v", cleaned)
	}
	if _, err := manager.Get(context.Background(), "member", rec.Job.ID); err == nil {
		t.Fatal("project cleanup kept terminal job metadata")
	}
	if _, err := projects.Snapshot(context.Background(), "member", snapshot.ProjectID); err == nil {
		t.Fatal("project cleanup kept current snapshot")
	}
	if err := projects.Materialize(snapshot, t.TempDir()); err == nil {
		t.Fatal("project cleanup kept unreferenced source blob")
	}
}

func TestCleanupProjectRejectsActiveJobs(t *testing.T) {
	cfg := config.Config{
		StateDir: t.TempDir(), Engines: []string{"xelatex"}, MaxFiles: 10,
		MaxUploadBytes: 1024, MaxExpandedBytes: 1024, MaxConcurrentCompiles: 1,
		MaxQueuedJobs: 2, MaxStateBytes: 4096,
	}
	projects, err := project.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := api.CompileRequest{ProtocolVersion: api.ProtocolVersion, Entry: "main.tex", Engine: "xelatex", Interaction: "nonstopmode"}
	snapshot := commitTestSnapshot(t, projects, request, []byte("active source"))
	manager := New(cfg, api.Metadata{}, compile.NewRunner(cfg), projects, nil, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	job, err := manager.Enqueue(context.Background(), "member", snapshot, request)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := manager.CleanupProject(context.Background(), "member", snapshot.ProjectID, "project", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.ActiveJobs) != 1 || preview.ActiveJobs[0] != job.ID {
		t.Fatalf("active cleanup preview = %#v", preview)
	}
	if _, err := manager.CleanupProject(context.Background(), "member", snapshot.ProjectID, "project", false); err == nil {
		t.Fatal("expected active project cleanup to be rejected")
	}
	if _, err := projects.Snapshot(context.Background(), "member", snapshot.ProjectID); err != nil {
		t.Fatalf("rejected cleanup changed snapshot: %v", err)
	}
}

func TestCleanupProjectWithPlanRejectsDriftBeforeDeleting(t *testing.T) {
	cfg := config.Config{
		StateDir: t.TempDir(), Engines: []string{"xelatex"}, MaxFiles: 10,
		MaxUploadBytes: 1024, MaxExpandedBytes: 1024, MaxConcurrentCompiles: 1,
		MaxQueuedJobs: 2, MaxStateBytes: 4096,
	}
	projects, err := project.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := api.CompileRequest{ProtocolVersion: api.ProtocolVersion, Entry: "main.tex", Engine: "xelatex", Interaction: "nonstopmode"}
	snapshot := commitTestSnapshot(t, projects, request, []byte("private source"))
	manager := New(cfg, api.Metadata{}, compile.NewRunner(cfg), projects, nil, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	now := time.Now().UTC()
	first := record{OwnerID: "member", Request: request, Snapshot: snapshot, Job: api.Job{ID: "job_first", ProjectID: snapshot.ProjectID, SnapshotID: snapshot.ID, Status: "succeeded", CreatedAt: now, FinishedAt: &now}}
	if err := manager.save(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	firstPath, err := projects.ResultPath("member", first.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(firstPath, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := manager.CleanupProject(context.Background(), "member", snapshot.ProjectID, "results", true)
	if err != nil || preview.PlanDigest == "" {
		t.Fatalf("preview = %#v, error = %v", preview, err)
	}

	second := first
	second.Job.ID = "job_second"
	if err := manager.save(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	secondPath, err := projects.ResultPath("member", second.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CleanupProjectWithPlan(context.Background(), "member", snapshot.ProjectID, "results", preview.PlanDigest); err == nil || !strings.Contains(err.Error(), "changed since preview") {
		t.Fatalf("drift error = %v", err)
	}
	for _, path := range []string{firstPath, secondPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("rejected plan deleted %s: %v", path, err)
		}
	}
}

func commitTestSnapshot(t *testing.T, projects *project.Manager, request api.CompileRequest, content []byte) project.Snapshot {
	t.Helper()
	digest := sha256.Sum256(content)
	sha := hex.EncodeToString(digest[:])
	plan, err := projects.Plan("member", api.UploadPlanRequest{
		ProjectID: "paper",
		Request:   request,
		Files:     []api.ProjectFile{{Path: "main.tex", SHA256: sha, Size: int64(len(content))}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Missing) > 0 {
		if err := projects.PutBlob("member", plan.UploadID, sha, bytes.NewReader(content)); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, _, err := projects.Commit(context.Background(), "member", plan.UploadID)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { return len(p), nil }
