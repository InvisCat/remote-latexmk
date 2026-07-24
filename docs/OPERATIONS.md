# Operations guide

## Deployment presets

The deployment bundler can emit resource settings tuned for the usual small
deployment shapes. Use a preset with a secure authentication mode; an external
PostgreSQL service is supported without bundling a database container.

```sh
node packages/deploy/dist/index.js bundle \
  --profile slim \
  --preset railway-serverless \
  --auth postgres --database postgres --external-database \
  --out dist/railway-serverless
```

| Preset | Intended use | Compile / queue | State cache | Retention |
|---|---|---:|---:|---:|
| `railway-serverless` | Short-lived Railway instance, low-to-medium use | 1 / 2 | 256 MiB tmpfs | results 24 h; snapshots/blobs 48 h |
| `lightsail-tokyo` | Always-on 2 GiB Lightsail instance in Tokyo | 1 / 12 | 3 GiB volume | 7 days |
| `railway` | Always-on Railway service | 1 / 5 | 512 MiB volume | 72 h |

All three presets keep a single compiler running at a time. This is deliberate:
XeLaTeX, Biber, TikZ, large images, and CJK fonts can transiently consume far
more memory and temporary disk than a small instance has available. Raise
concurrency only after measuring the largest real document on the selected
image and instance size.

The server still enforces `LATEXMK_MAX_STATE_BYTES` synchronously. The periodic
sweeper bounds normal cache growth: result archives expire first, project
snapshots expire next, and blobs are deleted only when no live upload, current
project snapshot, or queued/running job snapshot refers to them. A result whose
retention period has elapsed and its terminal job metadata are removed on the
same sweep schedule.

Database migration adds immutable snapshot fields to queued jobs. Historical
finished jobs from older versions remain readable. An old `queued` or `running`
job without a stored manifest is marked failed during startup and must be
submitted again; the server never substitutes the current project version.

For Railway Serverless, the state directory is deliberately in `/tmp`; cache
reuse lasts only for the running instance. For Lightsail and always-on Railway,
the generated Compose configuration uses a named volume. A volume is helpful
for repeated work on one paper, but it is not a database backup.

## Baseline sizing

For a research-group instance start with:

- 2 vCPU;
- 2–4 GiB RAM;
- 1–4 GiB of `/tmp` space;
- `LATEXMK_MAX_CONCURRENT_COMPILES=1` (or `2` only after testing);
- an edge/PaaS request timeout at least 30 seconds above
  `LATEXMK_COMPILE_TIMEOUT`.

TikZ, complex fonts, Glossaries, Biber, and large images substantially increase
CPU, memory, and temporary-disk requirements.

## Server binary environment defaults

The values below are the defaults of the server process when an environment
variable is absent. The root `compose.yaml` deliberately overrides several of
them for the self-hosted profile. Use `.env.example` and `compose.yaml` as the
source of truth for root Compose defaults.

| Variable | Bare server default |
|---|---|
| `PORT` | `8080` |
| `LATEXMK_AUTH_MODE` | `token` |
| `LATEXMK_API_TOKEN` | unset; token value for token auth |
| `LATEXMK_API_TOKEN_FILE` | unset; alternative file containing one token |
| `LATEXMK_IMAGE_PROFILE` | `development` |
| `LATEXMK_ENGINES` | `xelatex,lualatex,pdflatex` |
| `LATEXMK_TOOLCHAIN_PATH` | `/usr/local/bin:/usr/bin:/bin` |
| `LATEXMK_COMPILE_TIMEOUT` | `2m` |
| `LATEXMK_MAX_UPLOAD_BYTES` | `64MiB` per v2 blob |
| `LATEXMK_MAX_EXPANDED_BYTES` | `256MiB` per project |
| `LATEXMK_MAX_ARTIFACT_BYTES` | `128MiB` |
| `LATEXMK_MAX_FILES` | `10000` |
| `LATEXMK_MAX_CONCURRENT_COMPILES` | `CPU/2`, minimum 1 |
| `LATEXMK_MAX_QUEUED_JOBS` | `100` |
| `LATEXMK_MAX_UPLOAD_SESSIONS` | `64` |
| `LATEXMK_MAX_LOG_BYTES` | `8MiB` per stream |
| `LATEXMK_MAX_STATE_BYTES` | `2GiB` |
| `LATEXMK_RESULT_RETENTION` | `168h` |
| `LATEXMK_SNAPSHOT_RETENTION` | `168h` |
| `LATEXMK_BLOB_RETENTION` | `168h` |
| `LATEXMK_STATE_SWEEP_INTERVAL` | `1h` |
| `LATEXMK_ALLOW_SHELL_ESCAPE` | `false` |
| `LATEXMK_ENABLE_LEGACY_COMPILE` | `false` |
| `LATEXMK_TEMP_DIR` | system temporary directory |
| `LATEXMK_STATE_DIR` | `/tmp/latexmk-state` |
| `LATEXMK_DATABASE_MODE` | `postgres` (`pglite` is supported) |
| `LATEXMK_CORS_ORIGINS` | empty (same-origin); comma-separated exact Dashboard origins |

Invalid booleans, durations, byte sizes, resource limits, or CORS origins fail
server startup instead of silently falling back. A CORS origin must be an exact
`http://` or `https://` origin; `*` and path-bearing URLs are rejected.
Set only one of `LATEXMK_API_TOKEN` and `LATEXMK_API_TOKEN_FILE`. The native
installer uses the file form so the generated token is kept separately from
the server settings.

The scheduler supports one server process. Do not run multiple replicas against
the same database or state directory. Multi-instance claiming and shared object
storage are not implemented yet.

## Image pinning

The root Compose file and Dockerfiles use readable base-image tags paired with
immutable digests. When updating a base image, review the upstream release and
change the tag and digest together. A direct slim server build from the
repository root uses this command shape:

```sh
docker build \
  --file packages/deploy/templates/Dockerfile.slim \
  --build-arg SERVER_SOURCE=packages/server \
  --build-arg DEPLOY_ASSETS=packages/deploy/templates \
  --build-arg TEXLIVE_IMAGE='texlive/texlive:TAG@sha256:DIGEST' \
  --build-arg VERSION='0.4.3' \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  --build-arg BUILD_DATE="$(date -u +%FT%TZ)" \
  -t registry.example.edu/remote-latexmk-server:0.4.3 .
```

Use `rlatexmk meta` to verify the remote toolchain actually running the image.

## Compose watcher

The root Compose file includes an optional long-running client:

```sh
docker compose --profile watch up -d client-watch
docker compose logs -f client-watch
```

Set `LATEXMK_PROJECT_DIR`, `LATEXMK_CLIENT_ENTRY`,
`LATEXMK_CLIENT_WATCH_INTERVAL`, and `LATEXMK_CLIENT_WATCH_DEBOUNCE` in `.env`.
On Linux, also set `LATEXMK_CLIENT_UID` and `LATEXMK_CLIENT_GID` so returned
artifacts are writable by the host user. Restart `client-watch` after changing
client configuration or environment variables.

## Compose network boundary

The root self-hosted Compose file uses three networks:

- `latexmk-backend` is `internal: true`. The token/state/TeX server joins only
  this network and has no default Internet route.
- `edge` is used by the credential-free HTTP gateway and optional HTTPS proxy
  to publish host ports. These proxies can reach the server on the backend but
  do not receive its environment or state volume.
- `client-egress` is a normal bridge used only by `client` and `client-watch`.
  It lets a containerized client reach an external HTTPS server while the same
  client can still resolve the local `server` service on the backend network.

Run `docker compose up -d` so both `server` and `gateway` start. Starting only
the `server` service deliberately does not expose a host port.

This topology is intended for the default single-user token deployment, which
does not need an external database. If an advanced deployment adds an external
PostgreSQL service, attach the server to a narrowly scoped database network or
use the provider's private network. Do not attach the compiler-facing server to
a general egress bridge merely for convenience.

Network isolation does not create a hostile-code sandbox by itself. TeX still
runs in the API container and shares its filesystem identity. A separate
compiler worker or stronger runtime sandbox remains required before treating
untrusted papers as safe.

## PostgreSQL and PGlite

The server uses GORM/pgx over the PostgreSQL protocol and creates its tables and
indexes on startup. Full PostgreSQL should only be reachable by the server, via
TLS or a PaaS private network. PGlite socket does not provide TLS and should be
restricted to single-instance development or demonstrations; its URL must use
`sslmode=disable`.

`LATEXMK_STATE_DIR` holds content-addressed source blobs and job result
archives. Do not expose it as a static-file directory. Keep its volume
permission-restricted, bounded, and covered by an explicit retention policy.

When Dashboard and API have different origins, add the Dashboard's exact origin
(for example `https://latex-console.example.edu`) to
`LATEXMK_CORS_ORIGINS`. Do not use `*` for a console that holds administrative
tokens.

Store bootstrap tokens in the PaaS secret manager, never in the image or the
repository. The bootstrap token is an emergency administrator credential; rotate
or remove it after a separate recovery route exists.

## Monitoring

Collect at least:

- HTTP 5xx count;
- compilation `success=false` rate;
- `duration_ms` distribution and timeouts;
- upload and queue rejection reasons;
- temporary-disk, state-volume, and memory use;
- active compiles, queued jobs, and PaaS queue time;
- cache sweep errors and reclaimed bytes.

Logs are JSON and include `request_id`; they do not log project content, tokens,
or database URLs.
