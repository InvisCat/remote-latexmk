package dependency

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	projectarchive "github.com/billstark001/latexmk/packages/cli/internal/archive"
)

func TestDiscoverEntriesSelectsOneDocumentClassCandidate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "main.tex", "\\documentclass{article}\n\\begin{document}ok\\end{document}\n")
	writeFile(t, root, "sections/body.tex", "body")
	writeFile(t, root, "commented.tex", "% \\documentclass{book}\n")
	writeFile(t, root, "verbatim.tex", "\\begin{verbatim}\n\\documentclass{book}\n\\end{verbatim}\n")
	candidates, _, err := projectarchive.Manifest(projectarchive.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	result, err := DiscoverEntries(candidates)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "selected" || !result.Unambiguous || result.Selected != "main.tex" {
		t.Fatalf("entry discovery = %#v", result)
	}
	if result.CandidateCount != 1 || len(result.Candidates) != 1 || result.Candidates[0].Reason != "documentclass" {
		t.Fatalf("entry candidates = %#v", result.Candidates)
	}
}

func TestDiscoverEntriesReturnsSortedAmbiguousRoots(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "zeta.tex", "\\documentclass{article}\n")
	writeFile(t, root, "nested/alpha.tex", "\\documentclass{report}\n")
	writeFile(t, root, "chapter.tex", "chapter")
	candidates, _, err := projectarchive.Manifest(projectarchive.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	result, err := DiscoverEntries(candidates)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ambiguous" || result.Unambiguous || result.Selected != "" || result.CandidateCount != 2 {
		t.Fatalf("entry discovery = %#v", result)
	}
	if result.Candidates[0].Path != "nested/alpha.tex" || result.Candidates[1].Path != "zeta.tex" {
		t.Fatalf("entry candidates are not sorted: %#v", result.Candidates)
	}
}

func TestDiscoverEntriesUsesOnlyTexFileWhenNoRootMarkerExists(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "paper.tex", "plain fragment")
	candidates, _, err := projectarchive.Manifest(projectarchive.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	result, err := DiscoverEntries(candidates)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Unambiguous || result.Selected != "paper.tex" || result.Candidates[0].Reason != "only-tex-file" {
		t.Fatalf("single TeX result = %#v", result)
	}
}

func TestDiscoverEntriesFailsClosedForUninspectedTexFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "main.tex", "\\documentclass{article}\n")
	largePath := filepath.Join(root, "large.tex")
	largeFile, err := os.OpenFile(largePath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := largeFile.Truncate(maxParsedFileSize + 1); err != nil {
		_ = largeFile.Close()
		t.Fatal(err)
	}
	if err := largeFile.Close(); err != nil {
		t.Fatal(err)
	}
	candidates, _, err := projectarchive.Manifest(projectarchive.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	result, err := DiscoverEntries(candidates)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ambiguous" || result.Unambiguous || result.CandidateCount != 2 || len(result.Warnings) != 1 {
		t.Fatalf("large candidate result = %#v", result)
	}
}

func TestDiscoverEntriesBoundsAmbiguousOutput(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "fragment")
	if err := os.WriteFile(source, []byte("fragment"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := make([]projectarchive.File, 0, maxReportedEntryCandidates+1)
	for i := 0; i <= maxReportedEntryCandidates; i++ {
		files = append(files, projectarchive.File{Path: filepath.ToSlash(filepath.Join("parts", entryTestName(i))), Source: source, Size: 8})
	}
	result, err := DiscoverEntries(files)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || result.CandidateCount != maxReportedEntryCandidates+1 || len(result.Candidates) != maxReportedEntryCandidates {
		t.Fatalf("bounded entry result = %#v", result)
	}
}

func TestDiscoverEntriesBoundsWarnings(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "large.tex")
	file, err := os.OpenFile(source, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxParsedFileSize + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	files := make([]projectarchive.File, 0, 300)
	for i := 0; i < 300; i++ {
		files = append(files, projectarchive.File{Path: entryTestName(i), Source: source, Size: maxParsedFileSize + 1})
	}
	result, err := DiscoverEntries(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) > maxReportedEntryWarnings {
		t.Fatalf("warnings = %d, want at most %d", len(result.Warnings), maxReportedEntryWarnings)
	}
	if !strings.Contains(result.Warnings[len(result.Warnings)-1], "additional entry discovery warnings omitted") {
		t.Fatalf("bounded warning summary missing: %v", result.Warnings)
	}
}

func TestDiscoverEntriesRejectsContentChangedAfterManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "main.tex", "\\documentclass{book}\n")
	candidates, _, err := projectarchive.Manifest(projectarchive.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "main.tex", "\\documentclass{memo}\n")
	if _, err := DiscoverEntries(candidates); err == nil || !strings.Contains(err.Error(), "content changed") {
		t.Fatalf("changed candidate error = %v", err)
	}
}

func TestDiscoverEntriesRejectsSymlinkSwapAfterManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require privileges on Windows")
	}
	root := t.TempDir()
	writeFile(t, root, "main.tex", "\\documentclass{book}\n")
	candidates, _, err := projectarchive.Manifest(projectarchive.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.tex")
	if err := os.WriteFile(outside, []byte("\\documentclass{book}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.tex")
	if err := os.Remove(mainPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, mainPath); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverEntries(candidates); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlink candidate error = %v", err)
	}
}

func entryTestName(index int) string {
	return fmt.Sprintf("candidate-%03d.tex", index)
}
