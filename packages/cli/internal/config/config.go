package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const FileName = ".latexmk.json"
const UserFileName = "config.json"

const (
	userConfigDir       = "remote-latexmk"
	legacyUserConfigDir = "latexmk"
	managedTokenPrefix  = "token-"
)

const maxTokenFileSize = 64 << 10

type FileConfig struct {
	Server             string   `json:"server"`
	Token              string   `json:"token,omitempty"`
	TokenFile          string   `json:"tokenFile,omitempty"`
	TokenFileManaged   bool     `json:"tokenFileManaged,omitempty"`
	ProjectRoot        string   `json:"projectRoot,omitempty"`
	ProjectID          string   `json:"projectId,omitempty"`
	RootMode           string   `json:"rootMode,omitempty"`
	UploadMode         string   `json:"uploadMode,omitempty"`
	ManifestFile       string   `json:"manifestFile,omitempty"`
	IncludeFiles       []string `json:"includeFiles,omitempty"`
	RespectGitIgnore   *bool    `json:"respectGitignore,omitempty"`
	Engine             string   `json:"engine,omitempty"`
	Timeout            string   `json:"timeout,omitempty"`
	Exclude            []string `json:"exclude,omitempty"`
	CAFile             string   `json:"caFile,omitempty"`
	InsecureSkipVerify bool     `json:"insecureSkipVerify,omitempty"`
}

type Resolved struct {
	Server             string
	Token              string
	TokenFile          string
	ProjectRoot        string
	ProjectID          string
	RootMode           string
	UploadMode         string
	ManifestFile       string
	IncludeFiles       []string
	RespectGitIgnore   bool
	Engine             string
	Timeout            time.Duration
	Exclude            []string
	CAFile             string
	InsecureSkipVerify bool
	ConfigPath         string
	UserConfigPath     string
}

// DefaultExcludes returns files that should not be uploaded without an
// explicit configuration override.
func DefaultExcludes() []string {
	return []string{
		".git",
		".gitignore",
		"node_modules",
		".latexmk-cache",
		"*.aux",
		"*.fdb_latexmk",
		"*.fls",
		"*.log",
		"*.synctex.gz",
		"*.xdv",
	}
}

// DefaultDeny returns local configuration and credential patterns that remain
// excluded even when a project replaces the ordinary exclude list.
func DefaultDeny() []string {
	return []string{
		FileName,
		".latexmk-cache",
		".latexmkignore",
		".latexmk-files",
		".env",
		".env.*",
		"*.key",
		"*.pem",
		"*.p12",
		"*.pfx",
		"id_rsa",
		"id_ed25519",
	}
}

func Load(start string) (Resolved, error) {
	return load(start, "", true)
}

// LoadBounded loads project configuration without walking above boundary.
// It is used when an MCP host supplies the workspace security boundary.
func LoadBounded(start, boundary string) (Resolved, error) {
	return load(start, boundary, true)
}

// LoadLocalPolicy loads project selection policy for commands that never
// contact the server. It does not read token files or expose token values.
func LoadLocalPolicy(start string) (Resolved, error) {
	return load(start, "", false)
}

// LoadLocalPolicyBounded loads non-secret project policy without walking
// above boundary. It is used when a command receives an explicit project root.
func LoadLocalPolicyBounded(start, boundary string) (Resolved, error) {
	return load(start, boundary, false)
}

func load(start, boundary string, resolveCredentials bool) (Resolved, error) {
	respectGitIgnore := true
	cfg := FileConfig{
		Server:           "http://127.0.0.1:8080",
		RootMode:         "entry",
		UploadMode:       "auto",
		RespectGitIgnore: &respectGitIgnore,
		Engine:           "xelatex",
		Timeout:          "3m",
		Exclude:          DefaultExcludes(),
	}
	userPath, err := findUserConfig()
	if err != nil {
		return Resolved{}, err
	}
	if userPath != "" {
		if err := mergeFile(userPath, &cfg); err != nil {
			return Resolved{}, err
		}
	}
	userServer := cfg.Server
	userToken := cfg.Token
	userTokenFile := cfg.TokenFile
	if userTokenFile != "" && !filepath.IsAbs(userTokenFile) {
		userTokenFile = filepath.Join(filepath.Dir(userPath), userTokenFile)
	}
	userCAFile := cfg.CAFile
	if userCAFile != "" && !filepath.IsAbs(userCAFile) {
		userCAFile = filepath.Join(filepath.Dir(userPath), userCAFile)
	}
	userInsecureSkipVerify := cfg.InsecureSkipVerify
	userHasCredentials := userToken != "" || userTokenFile != ""

	path, err := findConfig(start, boundary)
	if err != nil {
		return Resolved{}, err
	}
	if path != "" {
		if err := mergeFile(path, &cfg); err != nil {
			return Resolved{}, err
		}
	}
	if boundary != "" {
		// An Agent workspace may control project configuration, but it must not
		// redirect user credentials or weaken the user's TLS settings.
		cfg.Server = userServer
		cfg.Token = userToken
		cfg.TokenFile = userTokenFile
		cfg.CAFile = userCAFile
		cfg.InsecureSkipVerify = userInsecureSkipVerify
	} else if userHasCredentials {
		cfg.Server = userServer
		cfg.Token = userToken
		cfg.TokenFile = userTokenFile
		cfg.CAFile = userCAFile
		cfg.InsecureSkipVerify = userInsecureSkipVerify
	}
	cfg.Exclude = mergePatterns(cfg.Exclude, DefaultDeny())

	if v := os.Getenv("LATEXMK_SERVER"); v != "" {
		cfg.Server = v
	}
	if resolveCredentials {
		if cfg.TokenFile != "" {
			tokenFile := cfg.TokenFile
			if !filepath.IsAbs(tokenFile) {
				base := start
				if path != "" {
					base = filepath.Dir(path)
				}
				tokenFile = filepath.Join(base, tokenFile)
			}
			tokenFile, err = filepath.Abs(tokenFile)
			if err != nil {
				return Resolved{}, err
			}
			token, err := ReadTokenFile(tokenFile)
			if err != nil {
				return Resolved{}, fmt.Errorf("tokenFile: %w", err)
			}
			cfg.TokenFile = tokenFile
			cfg.Token = token
		}
		if v := os.Getenv("LATEXMK_TOKEN_FILE"); v != "" {
			absolute, err := filepath.Abs(v)
			if err != nil {
				return Resolved{}, fmt.Errorf("LATEXMK_TOKEN_FILE: %w", err)
			}
			token, err := ReadTokenFile(absolute)
			if err != nil {
				return Resolved{}, fmt.Errorf("LATEXMK_TOKEN_FILE: %w", err)
			}
			cfg.TokenFile = absolute
			cfg.Token = token
		}
		if v, ok := os.LookupEnv("LATEXMK_TOKEN"); ok && v != "" {
			cfg.Token = v
		}
	} else {
		cfg.Token = ""
		cfg.TokenFile = ""
	}
	if v := os.Getenv("LATEXMK_ENGINE"); v != "" {
		cfg.Engine = v
	}
	if v := os.Getenv("LATEXMK_PROJECT_ID"); v != "" {
		cfg.ProjectID = v
	}
	if v := os.Getenv("LATEXMK_CA_FILE"); v != "" {
		cfg.CAFile = v
	}
	if v := os.Getenv("LATEXMK_ROOT_MODE"); v != "" {
		cfg.RootMode = v
	}
	if v := os.Getenv("LATEXMK_UPLOAD_MODE"); v != "" {
		cfg.UploadMode = v
	}
	if v := os.Getenv("LATEXMK_MANIFEST_FILE"); v != "" {
		cfg.ManifestFile = v
	}
	if v := os.Getenv("LATEXMK_RESPECT_GITIGNORE"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return Resolved{}, fmt.Errorf("invalid LATEXMK_RESPECT_GITIGNORE %q: %w", v, err)
		}
		cfg.RespectGitIgnore = &parsed
	}
	if cfg.RootMode != "entry" && cfg.RootMode != "git" {
		return Resolved{}, fmt.Errorf("invalid rootMode %q; expected entry or git", cfg.RootMode)
	}
	if cfg.UploadMode != "auto" && cfg.UploadMode != "manifest" && cfg.UploadMode != "all" {
		return Resolved{}, fmt.Errorf("invalid uploadMode %q; expected auto, manifest, or all", cfg.UploadMode)
	}

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		return Resolved{}, fmt.Errorf("invalid timeout %q: %w", cfg.Timeout, err)
	}
	if timeout <= 0 {
		return Resolved{}, errors.New("timeout must be positive")
	}

	root := cfg.ProjectRoot
	if root != "" && !filepath.IsAbs(root) {
		base := start
		if path != "" {
			base = filepath.Dir(path)
		}
		root = filepath.Join(base, root)
	}
	if root != "" {
		root, err = filepath.Abs(root)
		if err != nil {
			return Resolved{}, err
		}
	}

	resolvedRoot := ""
	if root != "" {
		resolvedRoot = filepath.Clean(root)
	}
	respectGitIgnore = cfg.RespectGitIgnore == nil || *cfg.RespectGitIgnore
	return Resolved{
		Server:             cfg.Server,
		Token:              cfg.Token,
		TokenFile:          cfg.TokenFile,
		ProjectRoot:        resolvedRoot,
		ProjectID:          cfg.ProjectID,
		RootMode:           cfg.RootMode,
		UploadMode:         cfg.UploadMode,
		ManifestFile:       cfg.ManifestFile,
		IncludeFiles:       append([]string(nil), cfg.IncludeFiles...),
		RespectGitIgnore:   respectGitIgnore,
		Engine:             cfg.Engine,
		Timeout:            timeout,
		Exclude:            cfg.Exclude,
		CAFile:             cfg.CAFile,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		ConfigPath:         path,
		UserConfigPath:     userPath,
	}, nil
}

func mergeFile(path string, cfg *FileConfig) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, cfg); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func userConfigBase() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("find user config directory: %w", err)
		}
	}
	return base, nil
}

// UserConfigPath returns the primary path used for new user configuration.
func UserConfigPath() (string, error) {
	base, err := userConfigBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, userConfigDir, UserFileName), nil
}

// UserTokenPath returns the legacy fixed token path. New interactive logins
// use unique managed files in the same directory for transactional updates.
func UserTokenPath() (string, error) {
	base, err := userConfigBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, userConfigDir, "token"), nil
}

func findUserConfig() (string, error) {
	base, err := userConfigBase()
	if err != nil {
		return "", err
	}
	for _, directory := range []string{userConfigDir, legacyUserConfigDir} {
		path := filepath.Join(base, directory, UserFileName)
		st, statErr := os.Stat(path)
		if statErr == nil {
			if !st.Mode().IsRegular() {
				return "", fmt.Errorf("user config %s is not a regular file", path)
			}
			return path, nil
		}
		if !os.IsNotExist(statErr) {
			return "", fmt.Errorf("stat user config %s: %w", path, statErr)
		}
	}
	return "", nil
}

// ReadUserFile returns the existing primary or legacy user configuration.
func ReadUserFile() (FileConfig, string, error) {
	path, err := findUserConfig()
	if err != nil || path == "" {
		return FileConfig{}, path, err
	}
	cfg := FileConfig{}
	if err := mergeFile(path, &cfg); err != nil {
		return FileConfig{}, path, err
	}
	return cfg, path, nil
}

// ReadTokenFile reads one bearer token from a regular file. Leading and
// trailing whitespace is ignored to support Docker and Kubernetes secrets.
func ReadTokenFile(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("token file %s is not a regular file", path)
	}
	if st.Size() > maxTokenFileSize {
		return "", fmt.Errorf("token file %s exceeds %d bytes", path, maxTokenFileSize)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", fmt.Errorf("token file %s is empty", path)
	}
	if strings.ContainsAny(token, "\r\n") {
		return "", fmt.Errorf("token file %s must contain exactly one token", path)
	}
	return token, nil
}

func mergePatterns(base, required []string) []string {
	result := append([]string{}, base...)
	seen := make(map[string]struct{}, len(result))
	for _, pattern := range result {
		seen[pattern] = struct{}{}
	}
	for _, pattern := range required {
		if _, ok := seen[pattern]; ok {
			continue
		}
		result = append(result, pattern)
		seen[pattern] = struct{}{}
	}
	return result
}

func Write(path string, cfg FileConfig) error {
	if cfg.Server == "" {
		cfg.Server = "http://127.0.0.1:8080"
	}
	if cfg.Engine == "" {
		cfg.Engine = "xelatex"
	}
	if cfg.RootMode == "" {
		cfg.RootMode = "entry"
	}
	if cfg.UploadMode == "" {
		cfg.UploadMode = "auto"
	}
	if cfg.RespectGitIgnore == nil {
		value := true
		cfg.RespectGitIgnore = &value
	}
	if cfg.Timeout == "" {
		cfg.Timeout = "3m"
	}
	if len(cfg.Exclude) == 0 {
		cfg.Exclude = DefaultExcludes()
	}
	cfg.Exclude = mergePatterns(cfg.Exclude, DefaultDeny())
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

// WriteUser writes a minimal user configuration atomically at the primary
// remote-latexmk config path. It does not follow a symlink at the target.
func WriteUser(cfg FileConfig) (string, error) {
	path, err := UserConfigPath()
	if err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	b = append(b, '\n')
	if err := writePrivateUserFile(path, b); err != nil {
		return "", err
	}
	return path, nil
}

// WriteUserToken stores one token in a new private file outside paper
// directories. Callers can switch configuration to the returned path without
// overwriting credentials used by an existing configuration.
func WriteUserToken(token string) (string, error) {
	token, err := NormalizeUserToken(token)
	if err != nil {
		return "", err
	}
	legacyPath, err := UserTokenPath()
	if err != nil {
		return "", err
	}
	directory := filepath.Dir(legacyPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create user config directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", fmt.Errorf("protect user config directory: %w", err)
	}
	file, err := os.CreateTemp(directory, managedTokenPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("create token file: %w", err)
	}
	path := file.Name()
	installed := false
	defer func() {
		if !installed {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return "", err
	}
	if _, err := file.WriteString(token + "\n"); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	installed = true
	return path, nil
}

// RemoveManagedUserToken removes a path that the caller has recorded as being
// created by this package. The path shape is checked again here, but its name
// alone is not proof of ownership. Paths outside the private user config
// directory and non-regular files are left untouched.
func RemoveManagedUserToken(path string) error {
	if path == "" {
		return nil
	}
	legacyPath, err := UserTokenPath()
	if err != nil {
		return err
	}
	path = filepath.Clean(path)
	managedDirectory := filepath.Dir(legacyPath)
	name := filepath.Base(path)
	if filepath.Dir(path) != managedDirectory || (path != legacyPath && !managedTokenFileName(name)) {
		return nil
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect old token file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("old token file %s is not a regular file", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove old token file: %w", err)
	}
	return nil
}

func managedTokenFileName(name string) bool {
	suffix := strings.TrimPrefix(name, managedTokenPrefix)
	if suffix == name || suffix == "" {
		return false
	}
	for _, value := range suffix {
		if value < '0' || value > '9' {
			return false
		}
	}
	return true
}

// NormalizeUserToken validates one token before login attempts or storage.
func NormalizeUserToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("token must not be empty")
	}
	if strings.ContainsAny(token, "\r\n") {
		return "", errors.New("token must contain exactly one line")
	}
	if len(token) > maxTokenFileSize {
		return "", fmt.Errorf("token exceeds %d bytes", maxTokenFileSize)
	}
	return token, nil
}

func writePrivateUserFile(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create user config directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("protect user config directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("user file %s is not a regular file", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect user file %s: %w", path, err)
	}
	temporary, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary user file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("install user file: %w", err)
	}
	return nil
}

func findConfig(start, boundary string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	limit := ""
	if boundary != "" {
		limit, err = filepath.Abs(boundary)
		if err != nil {
			return "", err
		}
		rel, relErr := filepath.Rel(limit, dir)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", errors.New("configuration start is outside the workspace boundary")
		}
	}
	for {
		candidate := filepath.Join(dir, FileName)
		if st, err := os.Lstat(candidate); err == nil {
			if boundary != "" && st.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("project config %s must not be a symlink in Agent workspace mode", candidate)
			}
			if st.Mode().IsRegular() {
				return candidate, nil
			}
		}
		if limit != "" && dir == limit {
			return "", nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// FindGitRoot returns the nearest Git work tree root above start.
func FindGitRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no Git root found from %s", start)
		}
		dir = parent
	}
}
