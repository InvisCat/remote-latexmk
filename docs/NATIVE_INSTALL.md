# Native server installation

The native installer is an alternative to Docker Compose for a private Linux
server. It installs a tagged remote-latexmk server, TeX Live, configuration,
state, logs, and control tools below one user-owned directory. It does not use
`sudo` and does not edit shell startup files.

Supported server platforms are Linux amd64 and Linux arm64. Use Docker Compose
for other server operating systems.

## Fixed-version install

Use a release tag that contains `install-server.sh`, two native server
archives, and `SHA256SUMS`:

```sh
curl -fsSL https://github.com/InvisCat/remote-latexmk/releases/download/v0.4.2/install-server.sh | bash -s -- --version v0.4.2
```

In an interactive terminal, this opens a setup wizard. It asks for the TeX Live
profile, enabled engines, listen address, port, and service mode, then shows one
install plan for confirmation. Press Enter at each choice to keep the safe
defaults: full TeX Live, XeLaTeX plus PDFLaTeX, `127.0.0.1:8080`, and automatic
systemd user-service selection.

For automation, disable prompts and provide the choices explicitly:

```sh
bash install-server.sh --version vX.Y.Z --non-interactive \
  --profile full --engines xelatex,pdflatex \
  --listen 127.0.0.1:8080 --service auto
```

The installer verifies the native server archive against the release checksum.
The TeX Live network installer is obtained over HTTPS from CTAN and uses TeX
Live's normal repository verification. For a more controlled deployment,
download and inspect the installer first and set a trusted CTAN mirror with
`REMOTE_LATEXMK_TEXLIVE_REPOSITORY`.

The TeX Live profile and enabled server engines are separate settings. The
default full profile installs the complete package set, while the default
engine policy enables only XeLaTeX and PDFLaTeX. Enable LuaLaTeX explicitly
for trusted papers:

```sh
bash install-server.sh --version vX.Y.Z --profile full \
  --engines xelatex,lualatex,pdflatex
```

LuaLaTeX receives `--safer` and `--nosocket`, but LuaTeX remains programmable
and these flags are not a filesystem sandbox. `--profile slim` installs the
smaller XeLaTeX/PDFLaTeX package set used by the default Compose image.
Existing TeX Live files are reused during a server update.

## Installation layout

The default root is `~/.remote-latexmk`:

```text
bin/                 stable command symlinks
config/server.env    server settings (mode 0600)
config/server.override.env
                     persistent resource/retention tuning (mode 0600)
config/token         generated bearer token (mode 0600)
current/             symlink to the active tagged server release
releases/            versioned native server files
texlive/current/     private TeX Live installation
state/               snapshots, results, and retained server state
logs/                fallback-service logs
run/                 PID and temporary files
```

The installer creates `~/.config/systemd/user/remote-latexmk.service` when a
systemd user manager is available. That unit hides the rest of the user's home
directory and exposes only the active server release, TeX Live, read-only
configuration, state, and temporary directory to the service. Add
`~/.remote-latexmk/bin` to `PATH` yourself if you want the short command; this
is deliberately not automatic.
The installer reports when systemd user lingering is disabled. In that case,
the unit may stop after logout and will not start at boot until an administrator
enables lingering for the service account.

If no systemd user manager is available, `--service auto` installs but does not
start the server. The PID-file fallback cannot hide other files readable by the
Unix account, so selecting it requires the explicit `--service none` option.
Prefer enabling the user manager or running the server from a dedicated account
with no unrelated credentials or private files.

## Listen address and private networks

The default is localhost only:

```text
127.0.0.1:8080
```

The setup wizard lists loopback first, then IPv4 and IPv6 addresses discovered
from the server's active interfaces, a custom address, and the `0.0.0.0` and
`::` wildcards last. A wildcard is never the default, including during an
update. The installer cannot know which interface matches the intended trust
boundary, so the user still makes the choice. To configure a private interface
non-interactively, pass it explicitly:

```sh
bash install-server.sh --version vX.Y.Z \
  --listen PRIVATE_IP:8080
```

A server may have several LAN, VPN, container, or public addresses. Keep the
loopback default unless one private address is known. `0.0.0.0` and `::` are
available as explicitly confirmed fallbacks, but they listen on every IPv4 or
IPv6 interface and may expose the service publicly. If the client is not on
the same private network, use a VPN, SSH tunnel, or TLS reverse proxy. Do not
expose the plain HTTP listener directly to an untrusted network. The generated
bearer token is the only identity in the default single-user mode.

The native service also does not reproduce the Compose server's internal
no-egress Docker network. Disabled shell escape and LuaTeX `--nosocket` still
apply, but use host firewall policy when outbound isolation is required. TeX
can read ordinary system files that remain visible inside the systemd sandbox;
the native service is not a hostile-input public TeX sandbox.

## Control commands

```sh
remote-latexmkctl start
remote-latexmkctl stop
remote-latexmkctl restart
remote-latexmkctl status
remote-latexmkctl version
remote-latexmkctl logs
remote-latexmkctl logs -f
remote-latexmkctl doctor
remote-latexmkctl configure --listen PRIVATE_IP:8080
remote-latexmkctl upgrade --version vX.Y.Z
```

`configure` is preview-only without `--yes`. It prints the old and new listen
addresses. After review, repeat the same command with `--yes`; it updates the
saved setting and restarts only a service that was already running. If restart
fails, it restores the previous listener and attempts to restore the prior
running service.

`upgrade` prints the active and target versions, downloads the target release's
`SHA256SUMS` and `install-server.sh`, verifies the target installer, and runs it
non-interactively. The target installer preserves the existing profile,
engines, listener, service mode, token, TeX Live tree, and server state unless
an explicit installer option overrides a setting. It verifies the new server's
health and service identity after activation. If activation fails, it restores
the previous current release, configuration, service unit, and prior running
state. A failure before activation leaves the current release unchanged.

`config/server.env` is generated and replaced transactionally by the installer;
do not use it for persistent manual tuning. Supported database, CORS, timeout,
size, queue, log, retention, and sweep settings belong in the mode-0600
`config/server.override.env`. The installer validates that file against a
closed key list and preserves it across updates. An invalid override stops the
operation without printing its value, which may contain a secret.

The installer prints the generated token in its final setup summary and stores
it in `~/.remote-latexmk/config/token`. Run `remote-latexmkctl token` to show it
again. Copy it only into a protected client token file. Do not paste the token
into an Agent prompt or store it inside a paper directory.

## Removal

Normal removal requires confirmation and preserves configuration, TeX Live,
logs, and server state:

```sh
remote-latexmkctl uninstall
```

Complete removal is explicit:

```sh
remote-latexmkctl uninstall --purge
```

Non-interactive use must also pass `--yes`. The control tool refuses recursive
removal if the installation marker is missing.
