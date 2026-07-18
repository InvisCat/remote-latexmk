// Command release-builder creates deterministic client release archives.
package main

import (
	"archive/tar"
	"archive/zip"
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
	goos      string
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
	flag.StringVar(&opts.goos, "goos", "", "target operating system")
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
	if !supportedTarget(opts.goos, opts.goarch) {
		return fmt.Errorf("unsupported target %s/%s", opts.goos, opts.goarch)
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
	if _, err := os.Stat(filepath.Join(root, "packages", "cli", "go.mod")); err != nil {
		return errors.New("repo root does not contain packages/cli/go.mod")
	}
	outDir, err := filepath.Abs(opts.outDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "latexmk-release-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	binaryName := "latexmk"
	if opts.goos == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(tempDir, binaryName)
	ldflags := fmt.Sprintf("-s -w -X main.version=%s -X main.commit=%s -X main.buildDate=%s", opts.version, opts.commit, opts.buildDate)
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", ldflags, "-o", binaryPath, "./cmd/latexmk")
	cmd.Dir = filepath.Join(root, "packages", "cli")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+opts.goos, "GOARCH="+opts.goarch)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build client: %w", err)
	}
	prefix := fmt.Sprintf("latexmk_%s_%s_%s", opts.version, opts.goos, opts.goarch)
	files := []archiveFile{
		{name: binaryName, path: binaryPath, mode: 0o755},
		{name: "LICENSE", path: filepath.Join(root, "LICENSE"), mode: 0o644},
		{name: "README.md", path: filepath.Join(root, "README.md"), mode: 0o644},
	}
	extension := ".tar.gz"
	if opts.goos == "windows" {
		extension = ".zip"
	}
	target := filepath.Join(outDir, prefix+extension)
	if err := writeArchive(target, prefix, files, timestamp.UTC(), opts.goos == "windows"); err != nil {
		return err
	}
	fmt.Println(target)
	return nil
}

func supportedTarget(goos, goarch string) bool {
	if goarch != "amd64" && goarch != "arm64" {
		return false
	}
	return goos == "linux" || goos == "darwin" || goos == "windows"
}

func writeArchive(target, prefix string, files []archiveFile, timestamp time.Time, windows bool) error {
	temp, err := os.CreateTemp(filepath.Dir(target), ".release-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	if windows {
		err = writeZip(temp, prefix, files, timestamp)
	} else {
		err = writeTarGz(temp, prefix, files, timestamp)
	}
	if err != nil {
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

func writeTarGz(dst io.Writer, prefix string, files []archiveFile, timestamp time.Time) error {
	gz, err := gzip.NewWriterLevel(dst, gzip.BestCompression)
	if err != nil {
		return err
	}
	gz.Header.ModTime = timestamp
	gz.Header.OS = 255
	tw := tar.NewWriter(gz)
	for _, file := range files {
		if err := addTarFile(tw, prefix, file, timestamp); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return err
	}
	return gz.Close()
}

func addTarFile(tw *tar.Writer, prefix string, file archiveFile, timestamp time.Time) error {
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
	return copyFile(tw, file.path)
}

func writeZip(dst io.Writer, prefix string, files []archiveFile, timestamp time.Time) error {
	zw := zip.NewWriter(dst)
	for _, file := range files {
		info, err := os.Stat(file.path)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if !info.Mode().IsRegular() {
			_ = zw.Close()
			return fmt.Errorf("release input %s is not a regular file", file.path)
		}
		header := &zip.FileHeader{Name: filepath.ToSlash(filepath.Join(prefix, file.name)), Method: zip.Deflate}
		header.SetMode(file.mode)
		header.SetModTime(timestamp)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if err := copyFile(writer, file.path); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}

func copyFile(dst io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, f)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
