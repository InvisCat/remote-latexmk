package jobs

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/billstark001/latexmk/packages/server/internal/api"
	"github.com/billstark001/latexmk/packages/server/internal/compile"
	"github.com/billstark001/latexmk/packages/server/internal/config"
	"github.com/billstark001/latexmk/packages/server/internal/project"
)

// This test models the exact stale-record interleaving used by the old worker:
// load queued, lose the race to cancellation, then try to save running.
func TestPOCCancelledJobCannotBeResurrectedByStaleWorker(t *testing.T) {
	cfg := config.Config{StateDir: t.TempDir(), MaxStateBytes: 4096, MaxConcurrentCompiles: 1, MaxQueuedJobs: 1}
	projects, err := project.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager := New(cfg, api.Metadata{}, compile.NewRunner(cfg), projects, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Now().UTC()
	rec := record{OwnerID: "member", Job: api.Job{ID: "job_poc_cancel", Status: "queued", CreatedAt: now}}
	if err := manager.save(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	staleWorkerCopy, err := manager.load(context.Background(), rec.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := manager.Cancel(context.Background(), "member", rec.Job.ID)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("cancel = %#v, %v", cancelled, err)
	}
	started := time.Now().UTC()
	staleWorkerCopy.Job.Status = "running"
	staleWorkerCopy.Job.StartedAt = &started
	_ = manager.save(context.Background(), staleWorkerCopy)

	got, err := manager.Get(context.Background(), "member", rec.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "cancelled" {
		t.Fatalf("cancelled job was resurrected as %q by a stale worker", got.Status)
	}
}

func TestPOCArchiveFailureCannotReportSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX test compiler")
	}
	toolchain := t.TempDir()
	if err := os.WriteFile(filepath.Join(toolchain, "latexmk"), []byte("#!/bin/sh\nprintf '%%PDF-test' > main.pdf\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", toolchain)
	cfg := config.Config{
		StateDir: t.TempDir(), Engines: []string{"xelatex"}, ToolchainPath: toolchain + string(os.PathListSeparator) + "/usr/bin:/bin",
		CompileTimeout: 5 * time.Second, ShutdownTimeout: time.Second, MaxFiles: 10,
		MaxUploadBytes: 1024, MaxExpandedBytes: 1024, MaxArtifactBytes: 1024,
		MaxConcurrentCompiles: 1, MaxQueuedJobs: 2, MaxLogBytes: 1024, MaxStateBytes: 1024,
	}
	projects, err := project.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := api.CompileRequest{ProtocolVersion: api.ProtocolVersion, Entry: "main.tex", Engine: "xelatex", Interaction: "nonstopmode"}
	snapshot := commitTestSnapshot(t, projects, request, []byte("paper source"))
	manager := New(cfg, api.Metadata{}, compile.NewRunner(cfg), projects, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	job, err := manager.Enqueue(context.Background(), "member", snapshot, request)
	if err != nil {
		t.Fatal(err)
	}
	manager.run(context.Background(), 1, job.ID)
	got, err := manager.Get(context.Background(), "member", job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || !strings.Contains(got.Error, "could not package compile result") {
		t.Fatalf("job = %#v, want result packaging failure", got)
	}
	if got.Result == nil || got.Result.Success || got.Result.ExitCode != 0 {
		t.Fatalf("compile result = %#v, want failed task with compiler exit code 0", got.Result)
	}
}
