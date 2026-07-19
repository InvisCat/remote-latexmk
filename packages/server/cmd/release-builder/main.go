// Command release-builder creates deterministic native server release archives.
package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

type options struct {
	version   string
	goarch    string
	commit    string
	buildDate string
	repoRoot  string
	outDir    string
}

type archiveFile struct {
	name string
	path string
	mode os.FileMode
}

func main() {
	var opts options
	flag.StringVar(&opts.version, "version", "", "release version without v prefix")
	flag.StringVar(&opts.goarch, "goarch", "", "target architecture")
	flag.StringVar(&opts.commit, "commit", "", "source commit")
	flag.StringVar(&opts.buildDate, "build-date", "", "RFC3339 source commit time")
	flag.StringVar(&opts.repoRoot, "repo-root", "", "repository root")
	flag.StringVar(&opts.outDir, "out-dir", "dist/release", "archive output directory")
	flag.Parse()
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "release-builder:", err)
		os.Exit(2)
	}
}

func run(opts options) error {
	if !versionPattern.MatchString(opts.version) {
		return errors.New("version must be a semantic version without a v prefix")
	}
	if opts.goarch != "amd64" && opts.goarch != "arm64" {
		return fmt.Errorf("unsupported target linux/%s", opts.goarch)
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(opts.commit) {
		return errors.New("commit must be a full lowercase Git SHA-1")
	}
	timestamp, err := time.Parse(time.RFC3339, opts.buildDate)
	if err != nil {
		return fmt.Errorf("build date must be RFC3339: %w", err)
	}
	root, err := filepath.Abs(opts.repoRoot)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "packages", "server", "go.mod")); err != nil {
		return errors.New("repo root does not contain packages/server/go.mod")
	}
	outDir, err := filepath.Abs(opts.outDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "remote-latexmk-server-release-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	binaryPath := filepath.Join(tempDir, "remote-latexmk-server")
	ldflags := fmt.Sprintf("-s -w -X main.version=%s -X main.commit=%s -X main.buildDate=%s", opts.version, opts.commit, opts.buildDate)
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", ldflags, "-o", binaryPath, "./cmd/server")
	cmd.Dir = filepath.Join(root, "packages", "server")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+opts.goarch)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build server: %w", err)
	}

	prefix := fmt.Sprintf("remote-latexmk-server_%s_linux_%s", opts.version, opts.goarch)
	files := []archiveFile{
		{name: "remote-latexmk-server", path: binaryPath, mode: 0o755},
		{name: "remote-latexmkctl", path: filepath.Join(root, "scripts", "remote-latexmkctl"), mode: 0o755},
		{name: "install-server.sh", path: filepath.Join(root, "scripts", "install-server.sh"), mode: 0o755},
		{name: "LICENSE", path: filepath.Join(root, "LICENSE"), mode: 0o644},
	}
	target := filepath.Join(outDir, prefix+".tar.gz")
	if err := writeArchive(target, prefix, files, timestamp.UTC()); err != nil {
		return err
	}
	fmt.Println(target)
	return nil
}

func writeArchive(target, prefix string, files []archiveFile, timestamp time.Time) error {
	temp, err := os.CreateTemp(filepath.Dir(target), ".server-release-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	gz, err := gzip.NewWriterLevel(temp, gzip.BestCompression)
	if err != nil {
		cleanup()
		return err
	}
	gz.Header.ModTime = timestamp
	gz.Header.OS = 255
	tw := tar.NewWriter(gz)
	for _, file := range files {
		if err := addFile(tw, prefix, file, timestamp); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			cleanup()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		cleanup()
		return err
	}
	if err := gz.Close(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Chmod(tempName, 0o644); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, target); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	return nil
}

func addFile(tw *tar.Writer, prefix string, file archiveFile, timestamp time.Time) error {
	info, err := os.Stat(file.path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("release input %s is not a regular file", file.path)
	}
	header := &tar.Header{
		Name: filepath.ToSlash(filepath.Join(prefix, file.name)), Mode: int64(file.mode.Perm()), Size: info.Size(),
		ModTime: timestamp, Uid: 0, Gid: 0, Uname: "root", Gname: "root", Format: tar.FormatUSTAR,
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	f, err := os.Open(file.path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(tw, f)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
