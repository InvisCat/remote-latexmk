# Native deployment

Use this path only on a controlled Linux amd64 or arm64 server. It installs the
service and a private TeX Live tree below `~/.remote-latexmk`, without `sudo`,
Docker, or shell-startup edits.

## Before installation

1. Confirm the server account and architecture.
2. Choose the connection path:
   - keep the default `127.0.0.1:8080` for an SSH tunnel;
   - use one known private LAN or VPN address for direct private access;
   - do not guess an interface or expose plain HTTP to an untrusted network.
3. Explain that native mode has weaker filesystem and outbound-network isolation than Docker. Use trusted papers and preferably a dedicated account or server.
4. Tell the user that the installer will print the API token. The user must run it in a trusted terminal outside the Agent.

## Install

Give the user this command with the release and private address chosen above.
The safe example keeps the loopback default. Add `--listen PRIVATE_IP:8080`
only after the user chooses a known private interface:

```sh
curl -fsSL https://github.com/InvisCat/remote-latexmk/releases/download/v0.3.0-rc.2/install-server.sh | bash -s -- --version v0.3.0-rc.2 --profile full --engines xelatex,pdflatex
```

The full TeX Live profile and enabled engine policy are separate. Keep
XeLaTeX and PDFLaTeX as the ordinary engine set. LuaLaTeX remains opt-in for
trusted papers; `--safer` and `--nosocket` do not make it a filesystem sandbox.

## Operate

These commands do not print the token:

```sh
~/.remote-latexmk/bin/remote-latexmkctl status
~/.remote-latexmk/bin/remote-latexmkctl doctor
~/.remote-latexmk/bin/remote-latexmkctl logs
~/.remote-latexmk/bin/remote-latexmkctl logs -f
```

Use `start`, `stop`, or `restart` only when requested. Use
`upgrade --version vX.Y.Z` only after confirming the target release and
compatibility with clients. `uninstall` preserves state by default;
`uninstall --purge` is destructive and requires a separate explicit decision.

If systemd user services are unavailable, do not silently fall back to a less
isolated background process. Explain the dedicated-account and
`--service none` trade-off first.
