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
VERSION=v0.3.0-rc.1
curl -fsSL \
  "https://github.com/InvisCat/remote-latexmk/releases/download/${VERSION}/install-server.sh" \
  | bash -s -- --version "${VERSION}" --profile full
```

The installer verifies the native server archive against the release checksum.
The TeX Live network installer is obtained over HTTPS from CTAN and uses TeX
Live's normal repository verification. For a more controlled deployment,
download and inspect the installer first and set a trusted CTAN mirror with
`REMOTE_LATEXMK_TEXLIVE_REPOSITORY`.

The first full installation is large and can take time. `--profile slim`
installs the smaller XeLaTeX/PDFLaTeX package set used by the default Compose
image. Existing TeX Live files are reused during a server update.

## Installation layout

The default root is `~/.remote-latexmk`:

```text
bin/                 stable command symlinks
config/server.env    server settings and generated token (mode 0600)
current/             symlink to the active tagged server release
releases/            versioned native server files
texlive/current/     private TeX Live installation
state/               snapshots, results, and retained server state
logs/                fallback-service logs
run/                 PID and temporary files
```

The installer creates `~/.config/systemd/user/remote-latexmk.service` when a
systemd user manager is available. That unit hides the rest of the user's home
directory and exposes only the active server release, TeX Live, state, and
temporary directory to the service. Add `~/.remote-latexmk/bin` to `PATH`
yourself if you want the short command; this is deliberately not automatic.
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

To listen on a private interface, pass an explicit address:

```sh
bash install-server.sh --version vX.Y.Z \
  --listen 192.0.2.10:8080
```

Do not expose the plain HTTP listener directly to an untrusted network. Put it
behind a private VPN, firewall, SSH tunnel, or TLS reverse proxy. The generated
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
remote-latexmkctl logs
remote-latexmkctl logs -f
remote-latexmkctl doctor
remote-latexmkctl upgrade --version vX.Y.Z
```

`remote-latexmkctl token` is the only command that prints the secret. Use it
only for an explicit transfer into a protected client token file. Do not paste
the token into an Agent prompt or store it inside a paper directory.

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
