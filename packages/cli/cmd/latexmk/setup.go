package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/billstark001/latexmk/packages/cli/internal/config"
	"github.com/billstark001/latexmk/packages/cli/internal/serverurl"
)

type setupOptions struct {
	server      string
	tokenFile   string
	caFile      string
	caFileSet   bool
	clearCAFile bool
	yes         bool
	dryRun      bool
	jsonOutput  bool
}

type setupResult struct {
	ConfigPath   string `json:"configPath"`
	MigratedFrom string `json:"migratedFrom,omitempty"`
	Server       string `json:"server"`
	TokenFile    string `json:"tokenFile"`
	CAFile       string `json:"caFile,omitempty"`
	Applied      bool   `json:"applied"`
}

func runSetup(args []string) int {
	opts, err := parseSetupArgs(args)
	if err != nil {
		return failAgentArguments("setup", hasJSONFlag(args), err)
	}
	current, existingPath, err := config.ReadUserFile()
	if err != nil {
		return failAgent("setup", opts.jsonOutput, err)
	}
	targetPath, err := config.UserConfigPath()
	if err != nil {
		return failAgent("setup", opts.jsonOutput, err)
	}

	server := opts.server
	if server == "" {
		server = current.Server
	}
	server, err = serverurl.Normalize(server)
	if err != nil {
		return failAgentArguments("setup", opts.jsonOutput, fmt.Errorf("--server: %w", err))
	}

	tokenFile := opts.tokenFile
	tokenFileExplicit := tokenFile != ""
	if tokenFile == "" {
		tokenFile = current.TokenFile
		if tokenFile != "" && !filepath.IsAbs(tokenFile) && existingPath != "" {
			tokenFile = filepath.Join(filepath.Dir(existingPath), tokenFile)
		}
	}
	if tokenFile == "" {
		if current.Token != "" {
			return failAgentArguments("setup", opts.jsonOutput, errors.New("the existing user config contains an embedded token; pass --token-file to migrate it"))
		}
		return failAgentArguments("setup", opts.jsonOutput, errors.New("--token-file is required for the first setup"))
	}
	tokenFile, err = validateSetupTokenFile(tokenFile)
	if err != nil {
		return failAgentArguments("setup", opts.jsonOutput, err)
	}

	caFile := current.CAFile
	if opts.clearCAFile {
		caFile = ""
	} else if opts.caFileSet {
		caFile = opts.caFile
	}
	if caFile != "" {
		if !filepath.IsAbs(caFile) && existingPath != "" && !opts.caFileSet {
			caFile = filepath.Join(filepath.Dir(existingPath), caFile)
		}
		caFile, err = validateSetupRegularFile("CA file", caFile)
		if err != nil {
			return failAgentArguments("setup", opts.jsonOutput, err)
		}
	}

	result := setupResult{
		ConfigPath: targetPath,
		Server:     server,
		TokenFile:  tokenFile,
		CAFile:     caFile,
		Applied:    opts.yes,
	}
	if existingPath != "" && existingPath != targetPath {
		result.MigratedFrom = existingPath
	}
	command := "setup.preview"
	if opts.yes {
		current.Server = server
		current.Token = ""
		current.TokenFile = tokenFile
		if tokenFileExplicit {
			current.TokenFileManaged = false
		}
		current.CAFile = caFile
		writtenPath, writeErr := config.WriteUser(current)
		if writeErr != nil {
			return failAgent("setup.apply", opts.jsonOutput, writeErr)
		}
		result.ConfigPath = writtenPath
		command = "setup.apply"
	}
	if opts.jsonOutput {
		if err := writeAgentJSON(command, result); err != nil {
			return fail(err)
		}
		return 0
	}
	fmt.Printf("config:     %s\nserver:     %s\ntoken file: %s\n", result.ConfigPath, result.Server, result.TokenFile)
	if result.CAFile != "" {
		fmt.Printf("CA file:    %s\n", result.CAFile)
	}
	if result.MigratedFrom != "" {
		fmt.Printf("migrate:    %s\n", result.MigratedFrom)
	}
	if opts.yes {
		fmt.Println("setup applied; run 'latexmk doctor' to verify the connection")
	} else {
		fmt.Println("preview only; repeat the command with --yes after confirming these paths")
	}
	return 0
}

func parseSetupArgs(args []string) (setupOptions, error) {
	opts := setupOptions{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		value := func(name string) (string, error) {
			if strings.Contains(a, "=") {
				return strings.SplitN(a, "=", 2)[1], nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", name)
			}
			i++
			return args[i], nil
		}
		var err error
		switch {
		case a == "--server" || strings.HasPrefix(a, "--server="):
			opts.server, err = value("--server")
		case a == "--token-file" || strings.HasPrefix(a, "--token-file="):
			opts.tokenFile, err = value("--token-file")
		case a == "--ca-file" || strings.HasPrefix(a, "--ca-file="):
			opts.caFile, err = value("--ca-file")
			opts.caFileSet = err == nil
		case a == "--clear-ca-file":
			opts.clearCAFile = true
		case a == "--yes":
			opts.yes = true
		case a == "--dry-run":
			opts.dryRun = true
		case a == "--json":
			opts.jsonOutput = true
		case a == "--token" || strings.HasPrefix(a, "--token="):
			return setupOptions{}, errors.New("raw tokens are not accepted; use --token-file")
		default:
			return setupOptions{}, fmt.Errorf("unknown setup option %q", a)
		}
		if err != nil {
			return setupOptions{}, err
		}
	}
	if opts.clearCAFile && opts.caFileSet {
		return setupOptions{}, errors.New("--ca-file and --clear-ca-file cannot be combined")
	}
	if opts.dryRun && opts.yes {
		return setupOptions{}, errors.New("--dry-run and --yes cannot be combined")
	}
	return opts, nil
}

func validateSetupTokenFile(path string) (string, error) {
	resolved, err := validateSetupRegularFile("token file", path)
	if err != nil {
		return "", err
	}
	if _, err := config.ReadTokenFile(resolved); err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(resolved)
		if err != nil {
			return "", err
		}
		if info.Mode().Perm()&0o077 != 0 {
			return "", errors.New("token file is readable by group or other users; use chmod 600")
		}
	}
	return resolved, nil
}

func validateSetupRegularFile(label, path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s must be a regular file", label)
	}
	return resolved, nil
}
