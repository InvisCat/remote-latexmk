package main

import (
	"context"
	"fmt"

	"github.com/billstark001/latexmk/packages/cli/internal/client"
	"github.com/billstark001/latexmk/packages/cli/internal/protocol"
)

// verifyRemoteService checks public endpoints before asking for a secret.
func verifyRemoteService(ctx context.Context, remote *client.Client) (protocol.Metadata, error) {
	if err := remote.Health(ctx); err != nil {
		return protocol.Metadata{}, fmt.Errorf("server health check failed for %s: %w", remote.BaseURL, err)
	}
	metadata, err := remote.Metadata(ctx)
	if err != nil {
		return protocol.Metadata{}, fmt.Errorf("server metadata check failed: %w", err)
	}
	if metadata.Service != "remote-latexmk" {
		return protocol.Metadata{}, fmt.Errorf("server identifies as %q, not remote-latexmk", metadata.Service)
	}
	if metadata.ProtocolVersion != protocol.Version {
		return protocol.Metadata{}, fmt.Errorf(
			"server protocol v%d does not match client protocol v%d",
			metadata.ProtocolVersion,
			protocol.Version,
		)
	}
	return metadata, nil
}

// verifyRemoteAuthentication checks one authenticated, read-only endpoint
// without returning job data.
func verifyRemoteAuthentication(ctx context.Context, remote *client.Client) error {
	if _, err := remote.ListJobs(ctx, 1); err != nil {
		return fmt.Errorf("remote-latexmk API token verification failed: %w", err)
	}
	return nil
}

// verifyRemoteAccess checks the public service identity and configured token.
func verifyRemoteAccess(ctx context.Context, remote *client.Client) (protocol.Metadata, error) {
	metadata, err := verifyRemoteService(ctx, remote)
	if err != nil {
		return protocol.Metadata{}, err
	}
	if err := verifyRemoteAuthentication(ctx, remote); err != nil {
		return protocol.Metadata{}, err
	}
	return metadata, nil
}
