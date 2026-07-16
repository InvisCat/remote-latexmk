package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const FileName = ".latexmk.json"

type FileConfig struct {
	Server             string   `json:"server"`
	Token              string   `json:"token,omitempty"`
	ProjectRoot        string   `json:"projectRoot,omitempty"`
	Engine             string   `json:"engine,omitempty"`
	Timeout            string   `json:"timeout,omitempty"`
	Exclude            []string `json:"exclude,omitempty"`
	InsecureSkipVerify bool     `json:"insecureSkipVerify,omitempty"`
}

type Resolved struct {
	Server             string
	Token              string
	ProjectRoot        string
	Engine             string
	Timeout            time.Duration
	Exclude            []string
	InsecureSkipVerify bool
	ConfigPath         string
}

func Load(start string) (Resolved, error) {
	cfg := FileConfig{
		Server:  "http://127.0.0.1:8080",
		Engine:  "xelatex",
		Timeout: "3m",
		Exclude: []string{".git", "node_modules", ".latexmk-cache", "*.aux", "*.fdb_latexmk", "*.fls", "*.log", "*.synctex.gz", "*.xdv"},
	}
	path, err := findConfig(start)
	if err != nil {
		return Resolved{}, err
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return Resolved{}, fmt.Errorf("read %s: %w", path, err)
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return Resolved{}, fmt.Errorf("parse %s: %w", path, err)
		}
	}

	if v := os.Getenv("LATEXMK_SERVER"); v != "" {
		cfg.Server = v
	}
	if v := os.Getenv("LATEXMK_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("LATEXMK_ENGINE"); v != "" {
		cfg.Engine = v
	}

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		return Resolved{}, fmt.Errorf("invalid timeout %q: %w", cfg.Timeout, err)
	}
	if timeout <= 0 {
		return Resolved{}, errors.New("timeout must be positive")
	}

	root := cfg.ProjectRoot
	if root == "" {
		root, err = findGitRoot(start)
		if err != nil {
			return Resolved{}, err
		}
	} else if !filepath.IsAbs(root) {
		base := start
		if path != "" {
			base = filepath.Dir(path)
		}
		root = filepath.Join(base, root)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Resolved{}, err
	}

	return Resolved{
		Server:             cfg.Server,
		Token:              cfg.Token,
		ProjectRoot:        filepath.Clean(root),
		Engine:             cfg.Engine,
		Timeout:            timeout,
		Exclude:            cfg.Exclude,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		ConfigPath:         path,
	}, nil
}

func Write(path string, cfg FileConfig) error {
	if cfg.Server == "" {
		cfg.Server = "http://127.0.0.1:8080"
	}
	if cfg.Engine == "" {
		cfg.Engine = "xelatex"
	}
	if cfg.Timeout == "" {
		cfg.Timeout = "3m"
	}
	if len(cfg.Exclude) == 0 {
		cfg.Exclude = []string{".git", "node_modules", ".latexmk-cache", "*.aux", "*.fdb_latexmk", "*.fls", "*.log", "*.synctex.gz", "*.xdv"}
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func findConfig(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, FileName)
		if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func findGitRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	original := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return original, nil
		}
		dir = parent
	}
}
