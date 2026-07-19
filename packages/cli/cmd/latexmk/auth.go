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
	"github.com/billstark001/latexmk/packages/cli/internal/protocol"
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
	if err := validateSetupServer(server); err != nil {
		return failAgentArguments("auth.login", false, err)
	}
	if current.CAFile != "" && !filepath.IsAbs(current.CAFile) && existingPath != "" {
		current.CAFile = filepath.Join(filepath.Dir(existingPath), current.CAFile)
	}

	token, err := readLoginToken("remote-latexmk API token: ")
	if err != nil {
		return failAgent("auth.login", false, err)
	}
	token, err = config.NormalizeUserToken(token)
	if err != nil {
		return failAgentArguments("auth.login", false, err)
	}
	metadata, err := verifyAuthLogin(server, token, current)
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
	configPath, err := config.WriteUser(current)
	if err != nil {
		return failAgent("auth.login", false, err)
	}

	fmt.Printf("verified:    %s %s (protocol v%d)\n", metadata.Service, metadata.Version, metadata.ProtocolVersion)
	fmt.Printf("credentials saved for %s\ntoken file: %s\nconfig:     %s\n", server, tokenPath, configPath)
	return 0
}

func verifyAuthLogin(server, token string, current config.FileConfig) (protocol.Metadata, error) {
	remote, err := client.New(
		server,
		token,
		authLoginTimeout,
		current.InsecureSkipVerify,
		current.CAFile,
	)
	if err != nil {
		return protocol.Metadata{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), authLoginTimeout)
	defer cancel()
	return verifyRemoteAccess(ctx, remote)
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
  remote-latexmk auth login --server URL

The command reads the remote-latexmk API token from a hidden terminal prompt,
verifies the server and token, then stores it in the client user's private
configuration directory. The token is not placed in the command line, shell
history, paper directory, or user config JSON.
`)
}
