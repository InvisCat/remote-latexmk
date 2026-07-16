package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"testing"

	"github.com/billstark001/latexmk/packages/server/internal/api"
	"github.com/billstark001/latexmk/packages/server/internal/compile"
	"github.com/billstark001/latexmk/packages/server/internal/config"
	"github.com/billstark001/latexmk/packages/server/internal/project"
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
	if _, _, err := projects.Commit(context.Background(), "member", plan.UploadID); err != nil {
		t.Fatal(err)
	}
	manager := New(cfg, api.Metadata{}, compile.NewRunner(cfg), projects, nil, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	first, err := manager.Enqueue(context.Background(), "member", "paper", request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Enqueue(context.Background(), "member", "paper", request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Enqueue(context.Background(), "member", "paper", request); err == nil {
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

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { return len(p), nil }
