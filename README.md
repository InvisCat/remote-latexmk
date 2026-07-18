# latexmk

A remote LaTeX compilation service for small research groups. The local Go CLI
safely packages a project and sends it to a Go server. The server runs `latexmk`
in a disposable workspace and returns the PDF, logs, SyncTeX, and allowed
auxiliary files to the local project.

The project emphasizes predictable compilation and practical isolation: it does
not expose a persistent remote workspace, ignores `latexmkrc` files, disables
shell escape by default, and limits upload size, expansion, concurrency, queued
jobs, logs, artifacts, and state storage.

Each queued job is bound to an immutable content-addressed source snapshot, so
later uploads to the same project cannot change what an existing job compiles.

## Self-hosted server quick start

Requirements: Docker with Docker Compose. No local Go, Node.js, pnpm, or TeX
installation is needed to start the server.

```sh
cp .env.example .env
# Set LATEXMK_API_TOKEN in .env to a new random value of at least 24 characters.
docker compose up -d
curl http://127.0.0.1:8080/healthz
docker compose run --rm client main.tex
```

After a tagged release has been published in your fork, the same deployment
can pull versioned GHCR images without building Go or TeX Live layers locally:

```sh
# In .env, set LATEXMK_GHCR_NAMESPACE and an exact LATEXMK_GHCR_VERSION.
docker compose -f compose.yaml -f compose.ghcr.yaml up -d
docker compose -f compose.yaml -f compose.ghcr.yaml run --rm client main.tex
```

The override uses `pull_policy: always`, so an unavailable release fails
instead of silently building a different local image. Set
`LATEXMK_GHCR_SERVER_IMAGE` to the full `latexmk-server-full` reference when
the full TeX Live profile is required. For the strongest deployment pin, replace
the version tag with the image digest reported by GHCR. Public GHCR packages
need no login; for private packages, authenticate Docker with a token that has
`read:packages` before starting Compose.

The default binds to `127.0.0.1`. Set `LATEXMK_BIND_ADDRESS=0.0.0.0` only when
a firewall, private LAN, VPN, or TLS reverse proxy protects the service. Source
blobs and results are stored in the `latexmk-state` named volume. The slim
self-hosted image enables XeLaTeX and PDFLaTeX by default. The token/state/TeX
server is attached only to an internal Docker network, so it has no default
route to the Internet. A separate Caddy `gateway` with no token or state volume
publishes localhost port 8080. The Compose client and optional HTTPS proxy reach
the server over the internal network.

The default client command compiles `examples/basic/main.tex`. Set
`LATEXMK_PROJECT_DIR` in `.env` to an absolute paper directory, then pass the
entry path relative to that mount. If the paper inherits ignore rules from a
parent Git repository, mount the repository root and pass a nested entry path.
The client image contains the Go CLI, Git, and CA certificates, but no TeX Live.
Use `--no-deps` with `docker compose run` when `LATEXMK_CLIENT_SERVER` points to
an already-running remote server and the local `server` service is not needed.
Client containers also join a separate egress network so they can reach a
remote HTTPS server; the server itself does not join that network.

For continuous compilation, set `LATEXMK_PROJECT_DIR` and
`LATEXMK_CLIENT_ENTRY`, then start the dedicated watch profile:

```sh
docker compose --profile watch up client-watch
```

The watcher compiles once immediately, then polls only the currently selected
dependency set and its relevant manifest/Git-ignore policy files. A change is
debounced and submitted as a new immutable job. It does not watch every file in
the repository or treat a newly created unrelated file as a dependency.

### Optional private HTTPS

The `https` profile adds Caddy in front of the server. It creates a private CA
and keeps its key in the `caddy-data` volume:

```sh
docker compose --profile https up -d proxy
docker compose cp proxy:/data/caddy/pki/authorities/local/root.crt \
  certs/caddy-local-root.crt
```

For the Compose client, set these values in `.env`:

```dotenv
LATEXMK_CLIENT_SERVER=https://latexmk.local:8443
LATEXMK_CLIENT_CA_FILE=/etc/latexmk/certs/caddy-local-root.crt
```

Then `docker compose run --rm client main.tex` verifies Caddy's certificate
against that CA. A native client can use
`LATEXMK_SERVER=https://localhost:8443` and
`LATEXMK_CA_FILE=$PWD/certs/caddy-local-root.crt`.

HTTPS also binds only to `127.0.0.1` by default. For a private LAN, set
`LATEXMK_HTTPS_BIND_ADDRESS=0.0.0.0` and `LATEXMK_TLS_HOST` to the DNS name used
by clients. Distribute only the copied root certificate, never files containing
the Caddy CA private key. A server with a certificate already trusted by the
operating system needs no CA-file option.

## Monorepo

| Package | Implementation | Purpose |
|---|---|---|
| `@latexmk/cli` | Go | Local proxy, `latexmk` command, and engine-symlink compatibility |
| `@latexmk/server` | Go (Gin + GORM) | Compile API, incremental upload, job queue, metadata, authentication, limits, and PostgreSQL user/token management |
| `@latexmk/dashboard` | Preact + Vite | Console for jobs, capabilities, members, and API tokens |
| `@latexmk/deploy` | TypeScript | Standalone OCI/Docker context, Compose file, and deployment configuration generator |

The server uses **Gin** and **GORM/pgx** over the PostgreSQL protocol. Full
PostgreSQL and PGlite socket use the same connection interface. Use full
PostgreSQL for production; PGlite is limited to one development, demo, or test
instance. The connection pool is intentionally limited to one connection for
PGlite compatibility.

## Development quick start

Requirements: Go 1.23+, Node.js 22+, pnpm 11, and (for local end-to-end tests)
`latexmk` plus a TeX engine.

```sh
corepack enable pnpm
pnpm build
pnpm test
```

Start an explicitly unauthenticated local development server:

```sh
LATEXMK_AUTH_MODE=none \
LATEXMK_IMAGE_PROFILE=local-texlive \
./packages/server/dist/latexmk-server
```

Configure the client and compile:

```sh
cd examples/basic
../../../packages/cli/dist/latexmk init --server http://127.0.0.1:8080
../../../packages/cli/dist/latexmk main.tex
```

An engine-like symlink is also supported:

```sh
ln -s /absolute/path/to/latexmk/packages/cli/dist/latexmk ~/.local/bin/xelatex
xelatex -interaction=nonstopmode main.tex
```

The CLI selects its engine from its executable name. It validates common flags
and never passes unknown command-line arguments through to the server shell.

## Use the CLI from Bash

After building the CLI, place a symlink in a directory on Bash's `PATH`. The
following example uses `~/.local/bin`, keeps the command updated as you rebuild
the repository, and applies to subsequent Bash sessions:

```sh
cd /absolute/path/to/latexmk
pnpm --filter @latexmk/cli build

mkdir -p "$HOME/.local/bin"
ln -sf "$PWD/packages/cli/dist/latexmk" "$HOME/.local/bin/latexmk"
touch "$HOME/.bashrc"
grep -qxF 'export PATH="$HOME/.local/bin:$PATH"' "$HOME/.bashrc" || \
  printf '\n%s\n' 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.bashrc"
source "$HOME/.bashrc"

command -v latexmk
latexmk version
```

Tagged releases attach client archives for Linux, macOS, and Windows on amd64
and arm64. Each archive contains the client, `LICENSE`, and `README.md`. Verify
an archive before installing it:

```sh
# Linux
sha256sum -c SHA256SUMS --ignore-missing
# macOS: compare this output with the matching SHA256SUMS line
shasum -a 256 latexmk_*.tar.gz
# Windows PowerShell
Get-FileHash .\latexmk_*.zip -Algorithm SHA256
```

The native client does not need TeX Live, Go, Node.js, or pnpm at runtime. It
does need `git` when Git-ignore discovery is enabled (the default inside a Git
repository), plus the operating system CA store for HTTPS.

Add the `PATH` line only once. If your Bash startup files use `~/.bash_profile`
instead of `~/.bashrc`, add it there (or source `~/.bashrc` from that file).
The CLI name intentionally shadows a locally installed TeX Live `latexmk`; use
the absolute CLI path if you need both in the same shell.

## Client configuration

The CLI first reads the user config at `$XDG_CONFIG_HOME/latexmk/config.json`
(or the platform user config directory), then searches upward for a project
`.latexmk.json`:

```json
{
  "server": "https://latex.example.edu",
  "rootMode": "entry",
  "uploadMode": "auto",
  "respectGitignore": true,
  "caFile": "/absolute/path/to/lab-root-ca.pem",
  "engine": "xelatex",
  "timeout": "3m",
  "exclude": [".git", "node_modules", ".latexmk-cache", "*.aux", "*.fdb_latexmk", "*.fls", "*.log", "*.synctex.gz", "*.xdv"]
}
```

Without an explicit `projectRoot`, the project root is the directory containing
the entry TeX file. This prevents a command run in a subdirectory from silently
uploading its parent Git repository. Set `rootMode` to `git`, pass
`--root-mode git`, or set `--project-root` to request a wider root explicitly.

`uploadMode: "auto"` selects the entry file and supported literal LaTeX
dependencies after Git-ignore and deny rules have been applied. It reports
unresolved recognized dependencies before contacting the server. Missing,
ignored, and denied references share one `unavailable` diagnostic because the
scanner does not inspect filtered file contents. `--upload-mode all` uploads
every policy-allowed candidate and is an explicit compatibility fallback; it
still does not override the denylist.
Static scanning cannot prove that custom macros or unsupported packages do not
load more files. Always inspect `latexmk files` for a sensitive project. See
[`docs/DEPENDENCIES.md`](docs/DEPENDENCIES.md) for supported commands and
limitations.

Successful compiles cache workspace-local `.fls` INPUT paths in
`.latexmk-cache/dependencies.json`, keyed by entry and engine. Cached paths must
still pass the current Git-ignore and deny policies. When history covers a
dynamic reference, the CLI warns because the path set may be stale; it never
falls back to `all` automatically.

The first queued compile also creates `.latexmk-cache/project-id`. This is a
random project identity, not a credential. It stays with the paper directory
and is never uploaded as a project file. In particular, separate papers remain
separate even when the Docker client mounts each one at `/workspace`. Advanced
users can set `projectId`, `LATEXMK_PROJECT_ID`, or `--project-id`, but IDs must
not be reused for unrelated papers under the same token.

If stale history misses a file and the server reports a recognized TeX
missing-file diagnostic, `auto` mode can make a bounded retry. The client
resolves the exact request only inside its current policy-filtered manifest and
creates a new immutable snapshot. It never lets the server bypass Git-ignore,
the denylist, root checks, or symlink checks. Retries stop after 3 rounds, 64
new files, or 64 MiB. `manifest` mode remains strict and never adds files this
way.

Use `includeFiles`, repeatable `--include-file`, or a line-based
`manifestFile`/`--manifest` to add exact project-relative dependencies. In
`auto` mode they supplement static discovery. `uploadMode: "manifest"` selects
only the entry file and those explicit files, without reading recorder history
or inferring any other dependency. Explicit files still must pass Git-ignore,
denylist, root-boundary, and symlink policy. See
[`docs/DEPENDENCIES.md`](docs/DEPENDENCIES.md).

The project config may contain a `token` for a private, single-user setup. A
user config, environment variable, or token file is safer when the paper
directory is committed or shared:

```sh
export LATEXMK_TOKEN='lm_...'
# Or mount a Docker/Kubernetes secret and point to it:
export LATEXMK_TOKEN_FILE=/run/secrets/latexmk_token
```

Token priority is: CLI `--token`/`--token-file`, `LATEXMK_TOKEN`,
`LATEXMK_TOKEN_FILE`, user config, then project config. Project settings other
than the token override user defaults. A token file must contain exactly one
non-empty token; a trailing newline is accepted.

Standard HTTPS certificates use the operating system trust store. For a lab CA,
set `caFile`, `LATEXMK_CA_FILE`, or `--ca-file`. `--insecure-skip-verify` remains
an explicit debugging option and is not needed for the Compose HTTPS profile.

The client does not upload `.latexmk.json`, `.latexmkignore`, `.env` files, or
common private-key files by default, even when a project replaces the ordinary
exclude list. In a Git work tree it selects tracked files plus untracked files
that are not ignored, using Git's own nested, repository-local, and global
exclude rules. Use `--no-gitignore` only when ignored files are intentional
compile inputs. Add further exclusions in `.latexmkignore`. Symlinks are not
followed; the client fails when it encounters one so files outside the project
root cannot be uploaded.

Inspect the exact content-addressed manifest without contacting the server:

```sh
latexmk files main.tex
latexmk files --json main.tex
latexmk --dry-run main.tex
latexmk files --upload-mode all main.tex
```

Continuously compile the same policy-filtered dependency set:

```sh
latexmk watch main.tex
latexmk watch --watch-interval 500ms --watch-debounce 500ms main.tex
```

After each compile the watcher refreshes static, recorder, explicit-manifest,
and validated `needsFiles` dependencies. Edits made while a remote compile is
running schedule another immutable compile. Compile failures do not terminate
the watcher; fix a watched input to retry. `--json` emits one JSON result per
compile. Restart the watcher after changing `.latexmk.json`, user configuration,
environment variables, or command-line options.

```sh
latexmk compile --engine xelatex main.tex
latexmk watch main.tex
latexmk main.tex
latexmk meta
latexmk doctor
latexmk clean main.tex
latexmk remote clean --scope project
latexmk --json main.tex
```

Remote cleanup is a preview unless `--yes` is present:

```sh
# Remove compiled result archives, but keep job metadata and source snapshot.
latexmk remote clean --scope results
latexmk remote clean --scope results --yes

# Remove the current source snapshot and immediately collect unshared blobs.
latexmk remote clean --scope snapshot --yes

# Remove the snapshot, terminal jobs, results, and unshared source blobs.
latexmk remote clean --scope project --yes

# The same command through the client container.
docker compose run --rm client remote clean --scope project
docker compose run --rm client remote clean --scope project --yes
```

Snapshot and project deletion refuse to run while that project has queued or
running jobs. Content-addressed blobs still referenced by another project,
active upload, or active job are preserved. The API always scopes cleanup to
the authenticated token owner. For data created before random local project
IDs were introduced, run the preview with `--legacy-project-id`; use that flag
only for this migration case.

## Deployment

Build a slim XeLaTeX/CJK context for an existing PostgreSQL service:

```sh
pnpm --filter @latexmk/deploy build
node packages/deploy/dist/index.js bundle \
  --profile slim \
  --preset railway \
  --auth postgres --database postgres --external-database \
  --out dist/paas-slim
```

The supplied low-cost resource presets are:

| Preset | State storage | Queue / retention policy |
|---|---|---|
| `railway-serverless` | ephemeral tmpfs | 1 compiler, 2 queued jobs, results 24 h, snapshots/blobs 48 h |
| `lightsail-tokyo` | 3 GiB named volume | 1 compiler, 12 queued jobs, seven-day cache retention |
| `railway` | 512 MiB named volume | 1 compiler, 5 queued jobs, 72-hour cache retention |

Use `--profile full` for the full TeX Live image. The bundler writes
`.env.example`, `compose.yaml`, and `latexmk-deploy.json`; replace all secret
placeholders. `--external-database` connects to an already provisioned private
PostgreSQL service rather than adding another database container.

## Release process

The release workflow runs only for a semantic-version tag such as `v0.1.0`.
It performs these actions:

- builds deterministic client archives for six OS/architecture targets;
- writes one `SHA256SUMS` file and GitHub build attestations;
- publishes `latexmk-server`, `latexmk-server-full`, and multi-architecture
  `latexmk-client` images under the current repository owner's GHCR namespace;
- publishes OCI provenance and SBOM attestations for each image;
- creates or updates the matching GitHub release.

All third-party Actions are pinned to full commit SHAs. Docker build inputs use
readable tags plus immutable manifest digests. Dependabot is configured to
propose Action pin updates. Creating a local Git tag alone does not publish
anything; the workflow runs only after that tag is pushed to GitHub. Protect
release tags in repository settings before using this flow for public releases.

Client archives are byte-for-byte deterministic for identical source and build
metadata. Container base images are immutable, but the Dockerfiles still run
`apt` and, for the slim server, `tlmgr` against package repositories. Therefore
container provenance is auditable, but a clean rebuild is not yet guaranteed to
have the same final image digest. A future hardening step can move those package
installs to dated repository snapshots.

To build and export an OCI/Docker image:

```sh
node packages/deploy/dist/index.js bundle \
  --profile slim \
  --auth token \
  --out dist/paas-slim \
  --tag registry.example.edu/latexmk:0.1.0 \
  --build \
  --save dist/latexmk-0.1.0.tar
```

The templates are in `packages/deploy/templates/`. Their default Go, TeX Live,
Debian, Caddy, PostgreSQL, and Node images are pinned by manifest digest. Update
each readable tag and digest together; the release tests reject unpinned build
inputs.

## Server modes

### `none`

Only for an intentionally isolated local development instance. It is not the
bundler default and cannot be used with a deployment preset.

```sh
LATEXMK_AUTH_MODE=none
```

### `token`

One shared Bearer token without a database. This is the secure default.

```sh
LATEXMK_AUTH_MODE=token
LATEXMK_API_TOKEN='a random value at least 24 characters long'
```

### `postgres`

PostgreSQL stores users and API tokens; a bootstrap token provides initial
administration.

```sh
LATEXMK_AUTH_MODE=postgres
LATEXMK_DATABASE_MODE=postgres
DATABASE_URL='postgres://latexmk:password@postgres:5432/latexmk?sslmode=require'
LATEXMK_BOOTSTRAP_TOKEN='a random value at least 24 characters long'
```

Administration endpoints are `GET/POST /v1/admin/users`,
`PATCH /v1/admin/users/{id}`, and `POST /v1/admin/users/{id}/tokens`. A
plaintext API token is returned only once, in its creation response.

### PGlite development database

PGlite socket uses the PostgreSQL protocol, so the Go server needs no alternate
store implementation. It provides neither TLS nor production concurrency.

```sh
npm install -g @electric-sql/pglite-socket
pglite-server --db=.latexmk-pglite --host=127.0.0.1 --port=5432

LATEXMK_AUTH_MODE=postgres \
LATEXMK_DATABASE_MODE=pglite \
DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable' \
LATEXMK_BOOTSTRAP_TOKEN='a random value at least 24 characters long' \
./packages/server/dist/latexmk-server
```

The bundler supports `--auth postgres --database pglite` for local/demo Compose
only. The default database mode uses full PostgreSQL.

## Incremental uploads, jobs, and retention

The CLI creates the same validated project manifest as the legacy archive path,
addresses every file by SHA-256, asks the server for missing hashes, uploads
only changed content, and commits a project snapshot to a bounded queue. Each
job runs in a separate workspace. Result archives are available through the job
API. The synchronous `POST /v1/compile` endpoint remains for v1 clients.

`LATEXMK_STATE_DIR` defaults to `/tmp/latexmk-state`; container bundles normally
use `/var/lib/latexmk`. `LATEXMK_MAX_STATE_BYTES` is a hard combined source-cache
and result-archive limit. A periodic sweeper expires results, snapshots, and
unreferenced blobs according to TTL settings while preserving data referenced by
a live upload, current project snapshot, or queued/running job snapshot. The
state directory never stores plaintext API tokens. The authenticated project
cleanup API can remove retained data before its TTL; the CLI requires an
explicit scope, previews by default, and requires `--yes` for deletion.

## Dashboard

```sh
pnpm --filter @latexmk/dashboard dev
```

The development server proxies `/v1` to the local server. The console can use a
different API URL and Bearer token, displays jobs and capabilities, downloads
results, and manages users/tokens in administrator mode. Compilation remains
submitted through the safe local CLI.

## Metadata

`GET /v1/meta` returns the protocol, server version, commit, build date, image
profile, engines, resource and cache-retention limits, shell-escape/workspace/
rc-file policies, toolchain versions, and Go/OS/architecture information. Each
compile result also contains `serverVersion` and `imageProfile`.

## Security boundaries and limitations

- `latexmk -norc` ignores system, user, and project rc files.
- Shell escape is disabled by default and compilers receive a restricted
  environment rather than the PaaS process environment.
- Upload archives reject absolute paths, `..`, backslashes, duplicates,
  symlinks, hard links, and special files.
- Each request gets a disposable directory. Compile process groups are fully
  terminated after a timeout.
- The container is non-root; generated Compose settings use a read-only root,
  tmpfs, dropped capabilities, memory limits, and PID limits.
- The root self-hosted Compose topology puts the server/TeX process only on an
  internal backend network. A credential-free gateway publishes HTTP; client
  containers have a separate egress network. Neither component grants the
  server a default Internet route.
- Logs, artifacts, uploads, sessions, queues, and state storage have hard
  limits. Result artifacts must be workspace-local and allowed by `.fls` or a
  valid job-name rule.

Enabling shell escape is equivalent to allowing the uploader to run commands in
the container. Do not enable it unless every compiler is trusted and the PaaS
has no sensitive credentials, restricted networking, and strong isolation.

Logs are delivered after a job completes; SSE/WebSocket streaming is not yet
implemented. PGlite is a single-instance development database. Use full
PostgreSQL for production multi-instance deployments, long retention, or higher
concurrency.

See `docs/` for full API, operations, and security documentation.
