package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/billstark001/latexmk/packages/cli/internal/client"
	"github.com/billstark001/latexmk/packages/cli/internal/config"
	"github.com/billstark001/latexmk/packages/cli/internal/serverurl"
	"golang.org/x/term"
)

type authLoginOptions struct {
	server string
}

var readLoginToken = readLoginTokenFromTerminal

const authLoginTimeout = 15 * time.Second

func runAuth(args []string) int {
	if len(args) == 0 {
		authUsage()
		return 2
	}
	switch args[0] {
	case "help", "--help", "-h":
		authUsage()
		return 0
	case "login":
		return runAuthLogin(args[1:])
	default:
		return failAgentArguments("auth", false, fmt.Errorf("unknown auth command %q", args[0]))
	}
}

func runAuthLogin(args []string) int {
	opts, help, err := parseAuthLoginArgs(args)
	if err != nil {
		return failAgentArguments("auth.login", false, err)
	}
	if help {
		authUsage()
		return 0
	}
	current, existingPath, err := config.ReadUserFile()
	if err != nil {
		return failAgent("auth.login", false, err)
	}
	server := opts.server
	if server == "" {
		server = current.Server
	}
	server, err = serverurl.Normalize(server)
	if err != nil {
		return failAgentArguments("auth.login", false, fmt.Errorf("--server: %w", err))
	}
	if current.CAFile != "" && !filepath.IsAbs(current.CAFile) && existingPath != "" {
		current.CAFile = filepath.Join(filepath.Dir(existingPath), current.CAFile)
	}
	oldTokenPath := current.TokenFile
	oldTokenManaged := current.TokenFileManaged
	if oldTokenPath != "" && !filepath.IsAbs(oldTokenPath) && existingPath != "" {
		oldTokenPath = filepath.Join(filepath.Dir(existingPath), oldTokenPath)
	}
	fmt.Printf("server:      %s\n", server)
	remote, err := client.New(
		server,
		"",
		authLoginTimeout,
		current.InsecureSkipVerify,
		current.CAFile,
	)
	if err != nil {
		return failAgentArguments("auth.login", false, err)
	}
	preflightCtx, preflightCancel := context.WithTimeout(context.Background(), authLoginTimeout)
	metadata, err := verifyRemoteService(preflightCtx, remote)
	preflightCancel()
	if err != nil {
		return failAgent("auth.login", false, err)
	}

	token, err := readLoginToken("remote-latexmk API token: ")
	if err != nil {
		return failAgent("auth.login", false, err)
	}
	token, err = config.NormalizeUserToken(token)
	if err != nil {
		return failAgentArguments("auth.login", false, err)
	}
	remote.Token = token
	authCtx, authCancel := context.WithTimeout(context.Background(), authLoginTimeout)
	err = verifyRemoteAuthentication(authCtx, remote)
	authCancel()
	if err != nil {
		return failAgent("auth.login", false, err)
	}
	tokenPath, err := config.WriteUserToken(token)
	if err != nil {
		return failAgentArguments("auth.login", false, err)
	}
	current.Server = server
	current.Token = ""
	current.TokenFile = tokenPath
	current.TokenFileManaged = true
	configPath, err := config.WriteUser(current)
	if err != nil {
		if cleanupErr := config.RemoveManagedUserToken(tokenPath); cleanupErr != nil {
			err = fmt.Errorf("%w; cleanup staged token: %v", err, cleanupErr)
		}
		return failAgent("auth.login", false, err)
	}
	if oldTokenManaged && oldTokenPath != "" && filepath.Clean(oldTokenPath) != filepath.Clean(tokenPath) {
		if cleanupErr := config.RemoveManagedUserToken(oldTokenPath); cleanupErr != nil {
			fmt.Fprintln(os.Stderr, "latexmk: warning:", cleanupErr)
		}
	}

	fmt.Printf("verified:    %s %s (protocol v%d)\n", metadata.Service, metadata.Version, metadata.ProtocolVersion)
	fmt.Printf("credentials saved for %s\ntoken file: %s\nconfig:     %s\n", server, tokenPath, configPath)
	return 0
}

func parseAuthLoginArgs(args []string) (authLoginOptions, bool, error) {
	opts := authLoginOptions{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help" || a == "-h":
			return opts, true, nil
		case a == "--server" || strings.HasPrefix(a, "--server="):
			if strings.Contains(a, "=") {
				opts.server = strings.SplitN(a, "=", 2)[1]
			} else {
				if i+1 >= len(args) {
					return opts, false, errors.New("--server requires a value")
				}
				i++
				opts.server = args[i]
			}
		case a == "--token" || strings.HasPrefix(a, "--token="):
			return opts, false, errors.New("do not pass a token as an argument; paste it at the hidden prompt")
		default:
			return opts, false, fmt.Errorf("unknown auth login option %q", a)
		}
	}
	return opts, false, nil
}

func readLoginTokenFromTerminal(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("auth login requires an interactive terminal; use setup --token-file for automation")
	}
	fmt.Fprint(os.Stderr, prompt)
	value, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}
	return string(value), nil
}

func authUsage() {
	fmt.Print(`Usage:
  remote-latexmk auth login --server HOST_OR_URL

A bare host or an HTTP URL without a port uses http://HOST:8080. Explicit
HTTPS uses its standard port unless a port is provided. The command verifies
the service before reading the remote-latexmk API token from a hidden terminal
prompt, then stores the verified login in the client user's private config.
The token is not placed in the command line, shell history, paper directory,
or user config JSON.
`)
}
