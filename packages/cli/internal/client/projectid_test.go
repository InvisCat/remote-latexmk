package client

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectIDPersistsDistinctIdentity(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	first, err := ResolveProjectID(firstRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ResolveProjectID(firstRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveProjectID(secondRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatalf("project ID changed: %q != %q", first, again)
	}
	if first == second {
		t.Fatalf("separate projects share ID %q", first)
	}
	info, err := os.Stat(filepath.Join(firstRoot, filepath.FromSlash(projectIDRelativePath)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("project ID permissions are too broad: %o", info.Mode().Perm())
	}
}

func TestResolveProjectIDDoesNotCreateDuringLookup(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveProjectID(root, false); !errors.Is(err, ErrProjectIDNotFound) {
		t.Fatalf("lookup error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".latexmk-cache")); !os.IsNotExist(err) {
		t.Fatalf("lookup created cache directory: %v", err)
	}
}

func TestResolveProjectIDRejectsSymlinkedCache(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, ".latexmk-cache")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ResolveProjectID(root, true); err == nil {
		t.Fatal("expected symlinked cache directory to be rejected")
	}
}

func TestLegacyProjectIDRemainsPathDerived(t *testing.T) {
	root := t.TempDir()
	first, err := LegacyProjectID(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LegacyProjectID(root)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !validProjectID(first) {
		t.Fatalf("legacy project IDs = %q, %q", first, second)
	}
}
