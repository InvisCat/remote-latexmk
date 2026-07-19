package main

import (
	"context"
	"fmt"

	"github.com/billstark001/latexmk/packages/cli/internal/client"
	"github.com/billstark001/latexmk/packages/cli/internal/protocol"
)

// verifyRemoteAccess checks the public service identity and one authenticated,
// read-only endpoint without returning job data.
func verifyRemoteAccess(ctx context.Context, remote *client.Client) (protocol.Metadata, error) {
	if err := remote.Health(ctx); err != nil {
		return protocol.Metadata{}, fmt.Errorf("server health check failed: %w", err)
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
	if _, err := remote.ListJobs(ctx, 1); err != nil {
		return protocol.Metadata{}, fmt.Errorf("remote-latexmk API token verification failed: %w", err)
	}
	return metadata, nil
}
