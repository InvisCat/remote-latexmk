package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeCompilePathsResolvesProjectRootSymlink(t *testing.T) {
	physicalRoot := t.TempDir()
	entry := filepath.Join(physicalRoot, "main.tex")
	if err := os.WriteFile(entry, []byte("\\documentclass{article}"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(physicalRoot, alias); err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(physicalRoot)
	if err != nil {
		t.Fatal(err)
	}
	opts := compileOptions{projectRoot: alias, entry: "main.tex"}
	if err := normalizeCompilePaths(&opts, physicalRoot); err != nil {
		t.Fatal(err)
	}
	if opts.projectRoot != resolvedRoot {
		t.Fatalf("project root = %q, want %q", opts.projectRoot, resolvedRoot)
	}
	if opts.entry != "main.tex" {
		t.Fatalf("entry = %q, want main.tex", opts.entry)
	}
}
