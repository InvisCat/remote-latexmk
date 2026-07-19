# remote-latexmk — self-hosted remote LaTeX compiler

<p align="center">
  <img src="docs/assets/remote-latexmk-hero.svg" alt="remote-latexmk connects CLI and local MCP coding-agent clients to a private TeX server and returns PDFs and diagnostics" width="100%">
</p>

![Status: pre-release](https://img.shields.io/badge/status-pre--release-e69f00)
![License: MIT](https://img.shields.io/badge/license-MIT-2f81f7)

**Compile on a private LaTeX server you control.** Connect from laptops,
containers, and coding agents through a native client, Docker, or MCP. Preview
dependency-aware uploads and receive PDFs, logs, and diagnostics without
installing TeX Live in each environment.

## Docker quick start

The current public release candidate is `v0.2.0-rc.1`. The copied `.env` uses
its pinned server and client images by default.

Requirements: Git, Docker, Docker Compose, and `curl` for the health check. You
do not need local Go, Node.js, pnpm, Perl, latexmk, or TeX Live.

```sh
git clone https://github.com/InvisCat/remote-latexmk.git
cd remote-latexmk
cp .env.example .env

# Set LATEXMK_API_TOKEN in .env to a new random value of at least 24 characters.
# For example, `openssl rand -hex 32` prints a suitable value.

docker compose up -d
curl --fail --retry 15 --retry-connrefused --retry-delay 1 \
  http://127.0.0.1:8080/healthz
docker compose run --rm client main.tex
```

The last command compiles `examples/basic/main.tex` and returns its PDF to the
example directory. The first pull can take time because the server image
contains TeX Live. Later starts reuse the local image.

The default service binds to `127.0.0.1:8080`. Do not expose it on a public
interface without a private network, firewall, VPN, or TLS reverse proxy.

To build the server and client from the current checkout instead, select only
the source Compose file explicitly:

```sh
docker compose -f compose.yaml up -d --build
docker compose -f compose.yaml run --rm --build client main.tex
```

## Compile your own paper

Mount an absolute paper directory and pass the entry path relative to that
directory:

```sh
LATEXMK_PROJECT_DIR=/absolute/path/to/paper \
  docker compose run --rm client main.tex
```

If the paper uses ignore rules inherited from a parent Git repository, mount
that Git root and pass a nested entry path. The client container contains Git
and CA certificates, but no TeX Live.

Preview exactly what would be uploaded:

```sh
LATEXMK_PROJECT_DIR=/absolute/path/to/paper \
  docker compose run --rm client files main.tex
```

Use `--no-deps` when the container client points to an already-running remote
server instead of the Compose server. Set `LATEXMK_CLIENT_SERVER`,
`LATEXMK_CLIENT_TOKEN`, and any required `LATEXMK_CLIENT_CA_FILE` in `.env`
first; keep `LATEXMK_PROJECT_DIR` set to the paper:

```sh
docker compose run --rm --no-deps client main.tex
```

## Install a client

### Docker client

The Compose commands above are the shortest current path. They do not install
software in the paper directory. They write only returned artifacts and small
client state under `.latexmk-cache`.

### Native client from source

Building the native client requires Go 1.23+. The resulting binary does not
need Go or TeX Live at runtime; it needs Git when Git-aware selection is active:

```sh
mkdir -p "$HOME/.local/bin"
go build -trimpath -o "$HOME/.local/bin/latexmk" \
  ./packages/cli/cmd/latexmk
export PATH="$HOME/.local/bin:$PATH"
latexmk version
```

Add `$HOME/.local/bin` to the shell's startup configuration if it is not
already on `PATH`. The client uses the operating-system CA store for normal
HTTPS.

Configure one paper and compile it:

```sh
cd /absolute/path/to/paper
latexmk init --server http://127.0.0.1:8080
export LATEXMK_TOKEN='the same token from the server .env'
latexmk cache ignore
latexmk files main.tex
latexmk main.tex
```

`latexmk cache ignore` is explicit. It appends `.latexmk-cache/` to the project
`.gitignore` only when needed. `git clean -fdX` deletes ignored cache files and
therefore resets the local project identity.

### Release binaries

The [`v0.2.0-rc.1` prerelease](https://github.com/InvisCat/remote-latexmk/releases/tag/v0.2.0-rc.1)
provides client archives for Linux, macOS, and Windows on amd64 and arm64.
Verify downloads with the attached `SHA256SUMS`. See
[Publishing](docs/PUBLISHING.md) for the release process.

## AI agent setup

Install the client first, or configure the Docker MCP command below. The Skills
guide agents through manifest review, queued compilation, diagnostics with raw
log fallback, artifact download, and explicit cleanup previews.

Install both Skills into Codex, Claude Code, and OpenCode with the cross-Agent
installer:

```sh
npx skills add InvisCat/remote-latexmk -g \
  --skill remote-latex \
  --skill remote-latex-maintenance \
  --agent codex \
  --agent claude-code \
  --agent opencode
```

Review third-party Skill instructions before installing them. The maintenance
Skill can propose destructive cleanup, but requires a preview and explicit
confirmation before apply.

Manual user-level locations are:

| Agent | Skill directory |
|---|---|
| Codex | `~/.agents/skills/<skill-name>/SKILL.md` |
| Claude Code | `~/.claude/skills/<skill-name>/SKILL.md` |
| OpenCode | `~/.config/opencode/skills/<skill-name>/SKILL.md` or `~/.agents/skills/<skill-name>/SKILL.md` |

Codex and OpenCode can discover this repository's checked-in `.agents/skills`
directories directly. Claude Code needs its native directory, the installer,
or a future plugin wrapper.

### Local MCP server

The same client binary exposes strict STDIO MCP tools:

```sh
latexmk mcp serve --stdio --project-root /absolute/path/to/paper
```

For a Docker-based MCP host, first set `LATEXMK_PROJECT_DIR` in the repository
`.env` to the paper directory. The following command uses the local Compose
server and its client token settings:

```sh
docker compose --project-directory /absolute/path/to/remote-latexmk \
  run --rm -T client mcp serve --stdio --project-root /workspace
```

For an existing remote server, also set `LATEXMK_CLIENT_SERVER`,
`LATEXMK_CLIENT_TOKEN`, and any CA file in `.env`, then add `--no-deps` after
`run --rm` so Compose does not start its local server.

MCP fixes one project root at startup and exposes structured manifest, compile,
job, bounded log, diagnostic, artifact, and cleanup operations. It has no tool
for arbitrary shell commands, URLs, server paths, compiler argument lists, or
token reads. See [AI Agent integrations](docs/AI_AGENTS.md) and
[the MCP contract](docs/MCP.md).

## What gets uploaded?

The default `auto` mode starts from the entry file and discovers supported
literal LaTeX dependencies. Successful remote compiles add workspace-local
`.fls` input history, and a bounded missing-file retry can add an exact file
only after the client rechecks its current policy-filtered manifest.

Before any upload, the client applies:

- the selected project root, which defaults to the entry file's directory;
- Git's tracked, untracked, nested ignore, repository exclude, and global
  exclude rules;
- a non-overridable denylist for local configuration, `.env`, key material,
  client cache, and manifest-policy files;
- user exclusions and `.latexmkignore`;
- regular-file, project-boundary, size, and symlink checks.

Inspect the content-addressed manifest without contacting the server:

```sh
latexmk files main.tex
latexmk files --json main.tex
latexmk --dry-run main.tex
```

Static discovery cannot prove that every custom macro or package reference is
complete. Use an exact manifest for sensitive or dynamic projects; use
`--upload-mode all` only as an explicit compatibility fallback after reviewing
the manifest. See [Dependency discovery](docs/DEPENDENCIES.md).

## Engines and images

| Deployment | Default engines | Notes |
|---|---|---|
| Root source Compose | XeLaTeX, PDFLaTeX | Slim CJK-oriented TeX Live image |
| Full server image | XeLaTeX, LuaLaTeX, PDFLaTeX | Larger TeX Live image; LuaLaTeX runs with `--safer` and `--nosocket` |

When selecting a full GHCR image, set the matching released client image and
all three server policy values. Then use the base and GHCR Compose files shown
under [Prebuilt images](#prebuilt-images-and-digest-pinning):

```dotenv
LATEXMK_GHCR_SERVER_IMAGE=ghcr.io/inviscat/remote-latexmk-server-full:0.2.0-rc.1
LATEXMK_GHCR_CLIENT_IMAGE=ghcr.io/inviscat/remote-latexmk-client:0.2.0-rc.1
LATEXMK_IMAGE_PROFILE=texlive-full
LATEXMK_ENGINES=xelatex,lualatex,pdflatex
```

## Common workflows

```sh
# Compile once or watch selected dependencies.
latexmk main.tex
latexmk watch main.tex

# Inspect server capabilities and local policy health.
latexmk meta
latexmk doctor

# Work with immutable queued jobs and bounded diagnostics.
latexmk compile --detach --json main.tex
latexmk jobs list --limit 50 --json
latexmk diagnostics JOB_ID --json
latexmk logs JOB_ID --tail 200 --max-bytes 65536 --json
latexmk artifacts list JOB_ID --json
```

Local and remote deletion are preview-first:

```sh
latexmk cache inspect --project-root . --json
latexmk cache clean --project-root . --scope local-generated --json
latexmk remote clean --scope results
latexmk remote clean --plan-id PLAN_ID --yes
```

Both cleanup paths return a ten-minute plan ID. Remote apply accepts only that
plan ID, not a repeated scope. The server rejects the apply before deletion if
the remote report changed since preview. A successful apply consumes the plan.
Active jobs and shared content-addressed blobs remain protected. See
[Agent-facing CLI](docs/AGENT_CLI.md) for the JSON compatibility boundary and
full plan lifecycle.

## Private HTTPS

The optional Compose HTTPS profile runs Caddy with a private local CA:

```sh
docker compose --profile https up -d proxy
docker compose cp proxy:/data/caddy/pki/authorities/local/root.crt \
  certs/caddy-local-root.crt
```

Distribute only the copied root certificate, never the CA private key. Native
clients can use `LATEXMK_CA_FILE`; the Compose client can mount the copied
certificate. See [Operations](docs/OPERATIONS.md) for LAN, VPN, trusted
certificate, resource, retention, and network guidance.

## Security boundary

- Shell escape is disabled by default and `latexmk -norc` ignores rc files.
- Every compile uses a fresh temporary workspace and a restricted environment.
- Queued jobs bind to immutable content-addressed source snapshots.
- The root Compose server has no normal Internet route; a credential-free
  gateway publishes the localhost port.
- Uploads, expanded files, logs, artifacts, jobs, processes, and retained state
  have limits.

Uploaded snapshots and results are retained until configured expiry or explicit
cleanup. There is no shared mutable editing workspace, but this is still
persistent remote storage.

TeX remains a programmable and complex input format. The current server and TeX
process share one container identity. Do not run this as an anonymous compiler
for hostile documents without a separate worker sandbox or stronger runtime
isolation. Read [Security](docs/SECURITY.md) before exposing the service.

## Prebuilt images and digest pinning

The copied `.env` already selects the release pinned in `compose.ghcr.yaml`.
To select an exact version explicitly, set:

```dotenv
LATEXMK_GHCR_NAMESPACE=inviscat
LATEXMK_GHCR_VERSION=0.2.0-rc.1
```

```sh
docker compose -f compose.yaml -f compose.ghcr.yaml up -d
docker compose -f compose.yaml -f compose.ghcr.yaml run --rm client main.tex
```

For immutable deployment pins, use full `@sha256:` references instead of
putting a digest in `LATEXMK_GHCR_VERSION`:

```dotenv
LATEXMK_GHCR_SERVER_IMAGE=ghcr.io/inviscat/remote-latexmk-server@sha256:SERVER_DIGEST
LATEXMK_GHCR_CLIENT_IMAGE=ghcr.io/inviscat/remote-latexmk-client@sha256:CLIENT_DIGEST
```

The release workflow currently builds server images for `linux/amd64`, a
client image for `linux/amd64` and `linux/arm64`, and native client archives for
Linux, macOS, and Windows on amd64 and arm64.

## Documentation

- [Architecture and design choices](docs/ARCHITECTURE.md)
- [AI Agent integrations and discovery](docs/AI_AGENTS.md)
- [Dependency discovery and upload policy](docs/DEPENDENCIES.md)
- [MCP tools](docs/MCP.md)
- [Agent-facing CLI and JSON contracts](docs/AGENT_CLI.md)
- [HTTP API](docs/API.md)
- [Operations and HTTPS](docs/OPERATIONS.md)
- [Security model](docs/SECURITY.md)
- [Publishing, repository metadata, and social preview](docs/PUBLISHING.md)
- [Advanced PaaS bundler](packages/deploy/README.md)

## Development

Requirements: Go 1.23+, Node.js 22+, and pnpm 11. Local end-to-end tests also
need a TeX engine and the upstream Perl latexmk tool.

```sh
corepack enable pnpm
pnpm install --frozen-lockfile
pnpm test
pnpm lint
pnpm build
```

The repository contains the Go client and server, an optional development
Dashboard, deployment generators, and Agent integrations. The Dashboard is not
embedded in the server and is not started by the default Compose quick start.
Implementation details and selection reasons live in
[Architecture](docs/ARCHITECTURE.md).

## License

[MIT](LICENSE)
