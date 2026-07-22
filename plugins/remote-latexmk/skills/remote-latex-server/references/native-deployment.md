# Native deployment

Use this path only on a controlled Linux amd64 or arm64 server. It installs the
service and a private TeX Live tree below `~/.remote-latexmk`, without `sudo`,
Docker, or shell-startup edits.

## Before installation

1. Confirm the server account and architecture.
2. Choose the connection path. The interactive installer lists loopback first,
   then discovered IPv4/IPv6 interface addresses, a custom value, and the
   `0.0.0.0`/`::` wildcards last. A wildcard is never the default.
   Keep `127.0.0.1:8080` for local use or an SSH tunnel. Select one known
   private address for direct LAN or VPN access. Treat `0.0.0.0` as an explicit
   fallback because it may include a public interface.
3. Explain that native mode has weaker filesystem and outbound-network isolation than Docker. Use trusted papers and preferably a dedicated account or server.
4. Tell the user that the installer will print the API token. The user must run it in a trusted terminal outside the Agent.

## Install

Give the user this command with the chosen release. In an interactive terminal,
the installer asks for the TeX Live profile, enabled engines, listen address,
port, and service mode. Pressing Enter keeps the recommended choices:

```sh
curl -fsSL https://github.com/InvisCat/remote-latexmk/releases/download/v0.4.1/install-server.sh | bash -s -- --version v0.4.1
```

Use `--non-interactive` with explicit `--profile`, `--engines`, `--listen`,
and `--service` values for automation. Do not use it to bypass a decision the
user has not made.

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
`configure --listen HOST:PORT` to preview a listener change, then repeat the
same command with `--yes` only after approval. A failed restart restores the
previous listener.

Use `version` to report the active release. Use `upgrade --version vX.Y.Z`
only after confirming the immutable target release and client compatibility.
The control tool downloads the target release's installer and checksum list,
verifies that installer, and then lets the target installer preserve the
existing profile, engines, listener, service mode, state, and TeX Live tree.
It also preserves validated resource and retention tuning from
`config/server.override.env`; `config/server.env` is installer-managed.
If activation or the new health and identity checks fail, the installer restores
the previous current release, configuration, service unit, and prior running
state. A pre-activation failure leaves the current release active.

`uninstall` preserves state by default; `uninstall --purge` is destructive and
requires a separate explicit decision.

If systemd user services are unavailable, do not silently fall back to a less
isolated background process. Explain the dedicated-account and
`--service none` trade-off first.
