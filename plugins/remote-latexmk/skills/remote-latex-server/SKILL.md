---
name: remote-latex-server
description: Install, configure, update, or troubleshoot a remote-latexmk server. Use for native or Docker server deployment, service status, listen addresses, private networking, TLS, and server logs. Do not use for client login, paper compilation, or storage cleanup.
---

# Manage a Remote LaTeX Server

Keep server administration separate from client login and paper compilation.
Confirm the controlled server, operating system, deployment type, and intended
private connection path before proposing changes.

## Route the request

- Read [references/native-deployment.md](references/native-deployment.md) for a Linux user-owned installation under `~/.remote-latexmk`.
- Read [references/docker-deployment.md](references/docker-deployment.md) for Docker Compose deployment and container isolation.
- Read [references/server-troubleshooting.md](references/server-troubleshooting.md) only for service, listener, network, TLS, version, or TeX Live failures.

Require explicit user intent before installing, upgrading, uninstalling,
changing a listen address, or changing firewall, VPN, proxy, or TLS
configuration. Do not infer those mutations from a diagnosis request. Never
broaden exposure merely to make a connection test pass.

The native installer prints the API token in its final terminal summary. Tell
the user to run that installer in their own trusted terminal; do not run it
through an Agent tool. Never run `remote-latexmkctl token`, read
`~/.remote-latexmk/config/token`, or read token values from `.env`. Safe
server-side inspection may use status, doctor, and bounded logs when the Agent
is already authorized to operate that server.

Route client URL, token, CA, `auth login`, and client `doctor` problems to
`remote-latex-setup`. Route paper builds to `remote-latex`. Route cache,
snapshot, result, and project deletion to `remote-latex-maintenance`.
