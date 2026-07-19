<p align="center">
  <img src="docs/assets/remote-latexmk-hero.svg" alt="remote-latexmk connects CLI and local MCP coding-agent clients to a private TeX server and returns PDFs and diagnostics" width="100%">
</p>

![Status: pre-release](https://img.shields.io/badge/status-pre--release-e69f00)
![License: MIT](https://img.shields.io/badge/license-MIT-2f81f7)

**Compile on a private LaTeX server you control.** Connect from laptops,
containers, and coding agents through a native client, Docker, or MCP. Preview
dependency-aware uploads and receive PDFs, logs, and diagnostics without
installing TeX Live in each environment.

## Quick Start

Install the private server, then connect a coding agent. The server needs Linux
on amd64 or arm64. The client needs Node.js 20+, but no Go or TeX Live.

### 1. Server

The native installer puts the service and TeX Live under
`~/.remote-latexmk`. It uses no sudo, Docker, or system-wide TeX installation.

```sh
curl -fsSL https://github.com/InvisCat/remote-latexmk/releases/download/v0.3.0-rc.1/install-server.sh | bash -s -- --version v0.3.0-rc.1 --profile full --engines xelatex,pdflatex
```

> [!WARNING]
> Native mode has weaker isolation than Docker and may expose systemd-visible
> host files to TeX. Use it only for trusted papers on a dedicated account or
> server; LuaLaTeX remains opt-in. See [Security](docs/SECURITY.md).

For direct access, pass the server's reachable private LAN or VPN address to
`--listen`. Omit it to keep the `127.0.0.1:8080` default for an SSH tunnel. See
[Native server installation](docs/NATIVE_INSTALL.md) for other deployments.

### 2. Client (Agent)

If the Agent cannot reach the server over the same private network, use a VPN
or an SSH tunnel. For an SSH tunnel:

```sh
ssh -N -L 18080:127.0.0.1:8080 user@your-private-server
```

The installer prints its configured listen URL and a remote-latexmk API token.
Use a URL reachable from the client, or the tunnel endpoint above. Install the
Plugin for your Agent; it contains the Skills and local MCP launcher, and does
not install TeX Live.

#### Codex

```sh
codex plugin marketplace add InvisCat/remote-latexmk
codex plugin add remote-latexmk@remote-latexmk
```

#### Claude Code

```sh
claude plugin marketplace add InvisCat/remote-latexmk
claude plugin install remote-latexmk@remote-latexmk
```

Save the connection once on the client. Replace `SERVER_URL` with the reachable
server URL, or the local endpoint when using the tunnel above:

```sh
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 auth login --server SERVER_URL
```

Paste the remote-latexmk API token at the hidden prompt. The command verifies
the server and token, then stores the login in the client user's private
configuration directory, not in shell history or the paper.

Start a new Agent session from the paper directory and ask:

> Preview the Remote LaTeX upload, then compile this paper.

The Agent can now inspect the upload manifest, compile the paper, read bounded
diagnostics and logs, and download the PDF through the local MCP server.

#### Direct CLI

The saved login also configures direct CLI use:

```sh
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 files main.tex
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 main.tex
```

For OpenCode and other Agent hosts, see [AI coding agents](docs/AI_AGENTS.md).

## Alternative Installation and Usage Paths

The Quick Start above is a complete Server + Agent path. The options below are
alternatives for users who prefer Docker, a native client, or manual Agent
configuration.

### Docker Compose from Source

Requirements: Git, Docker, Docker Compose, and `curl` for the health check. You
do not need local Go, Node.js, pnpm, Perl, latexmk, or TeX Live.

```sh
git clone https://github.com/InvisCat/remote-latexmk.git
cd remote-latexmk
cp .env.example .env

# Set LATEXMK_API_TOKEN in .env to a new random value of at least 24 characters.
# For example, `openssl rand -hex 32` prints a suitable value.

docker compose -f compose.yaml up -d --build server gateway
curl --fail --retry 30 --retry-all-errors --retry-delay 1 \
  http://127.0.0.1:8080/readyz

export LATEXMK_PROJECT_DIR="/absolute/path/to/your/paper"
docker compose -f compose.yaml run --rm --no-deps --build client main.tex
```

The final command compiles `$LATEXMK_PROJECT_DIR/main.tex` and writes the
returned artifacts into that paper directory. The client container contains
Git and CA certificates, but no TeX Live. The first build can take time because
it pulls a TeX Live base image; later builds and starts reuse Docker's cache.
After the first client build, omit `--build` for normal recompiles.

Preview the exact upload without contacting the server:

```sh
docker compose -f compose.yaml run --rm --no-deps client files main.tex
```

If the paper inherits ignore rules from a parent Git repository, set
`LATEXMK_PROJECT_DIR` to that Git root and pass the entry path relative to it,
for example `papers/my-paper/main.tex`.

The explicit `-f compose.yaml` selects the definitions that build the server
and Docker client from this checkout. The prebuilt release-image path is under
[Prebuilt images](#prebuilt-images-and-digest-pinning). The service binds to
`127.0.0.1:8080` by default; protect any non-local binding with a private
network, firewall, VPN, or TLS reverse proxy.

### Native Client Instead of npm

The npm launcher used above selects the same tagged native client through npm
platform packages. It has no install script that fetches or executes a binary
from another URL. Install a native client when you do not want Node.js or a
client container.

Choose either a release binary or a source build, then configure the client.

#### Download a Release Binary

The [`v0.3.0-rc.1` prerelease](https://github.com/InvisCat/remote-latexmk/releases/tag/v0.3.0-rc.1)
provides client archives for Linux, macOS, and Windows on amd64 and arm64.
Verify downloads with the attached `SHA256SUMS`. See
[Publishing](docs/PUBLISHING.md) for the release process.

#### Build the Native Client from Source

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

#### Configure the Native Client

Save the server connection once, then compile a paper:

```sh
latexmk auth login --server http://127.0.0.1:8080
cd /absolute/path/to/paper
latexmk cache ignore
latexmk files main.tex
latexmk main.tex
```

`latexmk cache ignore` is explicit. It appends `.latexmk-cache/` to the project
`.gitignore` only when needed. `git clean -fdX` deletes ignored cache files and
therefore resets the local project identity.

### Install Agent Skills Manually

The repository-based Skills installer remains available for native and Docker
client setups:

```sh
npx skills add InvisCat/remote-latexmk -g \
  --skill remote-latex \
  --skill remote-latex-maintenance \
  --skill remote-latex-server \
  --skill remote-latex-setup \
  --agent codex --agent claude-code --agent opencode
```

Manual user-level locations are:

| Agent | Skill directory |
|---|---|
| Codex | `~/.agents/skills/<skill-name>/SKILL.md` |
| Claude Code | `~/.claude/skills/<skill-name>/SKILL.md` |
| OpenCode | `~/.config/opencode/skills/<skill-name>/SKILL.md` or `~/.agents/skills/<skill-name>/SKILL.md` |

Codex and Claude Code users should normally use the native Plugin in the Quick
Start. Manual Skill installation is mainly for OpenCode, Docker clients, or
custom Agent setups.

OpenCode, manual MCP configuration, and the older project-bound `agent install`
path are documented in [AI coding agents](docs/AI_AGENTS.md).

### Run the Local MCP Server Manually

The same client binary exposes strict STDIO MCP tools:

```sh
latexmk mcp serve --stdio --project-root /absolute/path/to/paper
```

For a Docker-based MCP host, first set `LATEXMK_PROJECT_DIR` in the repository
`.env` to the paper directory. The following command uses the local Compose
server and its client token settings:

```sh
docker compose --project-directory /absolute/path/to/remote-latexmk \
  -f /absolute/path/to/remote-latexmk/compose.yaml \
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

## What Gets Uploaded?

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

## Engines and Images

| Deployment | Default engines | Notes |
|---|---|---|
| Root source Compose | XeLaTeX, PDFLaTeX | Smaller TeX Live image |
| Full server image | XeLaTeX, LuaLaTeX, PDFLaTeX | Larger TeX Live image; LuaLaTeX runs with `--safer` and `--nosocket` |

When selecting a full GHCR image, set the matching released client image and
all three server policy values. Then use the base and GHCR Compose files shown
under [Prebuilt images](#prebuilt-images-and-digest-pinning):

```dotenv
LATEXMK_GHCR_SERVER_IMAGE=ghcr.io/inviscat/remote-latexmk-server-full:0.3.0-rc.1
LATEXMK_GHCR_CLIENT_IMAGE=ghcr.io/inviscat/remote-latexmk-client:0.3.0-rc.1
LATEXMK_IMAGE_PROFILE=texlive-full
LATEXMK_ENGINES=xelatex,lualatex,pdflatex
```

## Executable Paper Examples

The repository keeps two synthetic papers as both documentation and test
fixtures. [`examples/slim`](examples/slim) uses the standard `article` class
against the default slim image. [`examples/ieee`](examples/ieee) uses
`IEEEtran`, BibTeX, an external figure, and several common paper packages
against the full image. Both reveal in their acknowledgments that they are test
data, not real academic papers.

Run the complete Compose smoke flow without installing TeX Live locally:

```sh
make smoke-papers
```

The test previews the exact dependency manifests, compiles both papers with
PDFLaTeX, downloads their artifacts, checks result APIs, and renders the first
pages when Poppler is available. It uses isolated Compose projects and removes
only the temporary resources it creates.

## Common Workflows

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

## Security Boundary

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

## Prebuilt Images and Digest Pinning

The current public release candidate is
[`v0.3.0-rc.1`](https://github.com/InvisCat/remote-latexmk/releases/tag/v0.3.0-rc.1).
The copied `.env` selects the release pinned in `compose.ghcr.yaml` for bare
`docker compose` commands. The commands below list both files explicitly. To
select an exact version, set:

```dotenv
LATEXMK_GHCR_NAMESPACE=inviscat
LATEXMK_GHCR_VERSION=0.3.0-rc.1
```

```sh
export LATEXMK_PROJECT_DIR="/absolute/path/to/your/paper"
docker compose -f compose.yaml -f compose.ghcr.yaml up -d
docker compose -f compose.yaml -f compose.ghcr.yaml \
  run --rm --no-deps client main.tex
```

For immutable deployment pins, use full `@sha256:` references instead of
putting a digest in `LATEXMK_GHCR_VERSION`:

```dotenv
LATEXMK_GHCR_SERVER_IMAGE=ghcr.io/inviscat/remote-latexmk-server@sha256:SERVER_DIGEST
LATEXMK_GHCR_CLIENT_IMAGE=ghcr.io/inviscat/remote-latexmk-client@sha256:CLIENT_DIGEST
```

To use only the Docker client with an existing server, set
`LATEXMK_CLIENT_SERVER`, `LATEXMK_CLIENT_TOKEN`, and any required
`LATEXMK_CLIENT_CA_FILE` in `.env`, keep `LATEXMK_PROJECT_DIR` set to the paper,
and run:

```sh
docker compose -f compose.yaml -f compose.ghcr.yaml \
  run --rm --no-deps client main.tex
```

The release workflow builds server images for `linux/amd64`, a client image for
`linux/amd64` and `linux/arm64`, native client archives for Linux, macOS, and
Windows on amd64 and arm64, native Linux server archives, and the versioned
server installer. npm publication is separately controlled by the repository's
trusted-publishing setting.

## Documentation

- [Architecture and design choices](docs/ARCHITECTURE.md)
- [AI Agent integrations and discovery](docs/AI_AGENTS.md)
- [Dependency discovery and upload policy](docs/DEPENDENCIES.md)
- [MCP tools](docs/MCP.md)
- [Agent-facing CLI and JSON contracts](docs/AGENT_CLI.md)
- [HTTP API](docs/API.md)
- [Operations and HTTPS](docs/OPERATIONS.md)
- [Native server installation](docs/NATIVE_INSTALL.md)
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

## Changelog

### Upstream 0.1.0 Baseline

remote-latexmk started as a fork of
[`billstark001/latexmk`](https://github.com/billstark001/latexmk) at commit
[`a338808`](https://github.com/billstark001/latexmk/commit/a338808)
on 2026-07-17.

- The upstream version provided a **Go CLI and Go compilation server** with
  shared-token and PostgreSQL authentication.
- It supported **content-addressed incremental uploads and queued jobs**,
  retention limits, result downloads, and a development dashboard.
- It compiled each request in a **disposable workspace** with `latexmk -norc`,
  shell escape disabled by default, path validation, and bounded resources.
- Its deployment bundler targeted **PaaS and research-group deployments**, and
  it included an initial Agent Skill.

### remote-latexmk 0.2.0-rc.1

- Added a root-level **Docker Compose quick start** so researchers and labs can
  run a private compiler without first learning the PaaS deployment bundler or
  installing Go, Node.js, pnpm, or TeX Live.
- Added a **TeX-free Docker client**, watch mode, native client archives, and
  GHCR images so laptops, containers, and coding-agent environments can use one
  server-side TeX Live installation.
- Added **safer project boundaries**, Git ignore handling, a built-in denylist,
  manifest previews, and dry runs so users can inspect and limit uploads when a
  paper repository also contains unrelated or sensitive files.
- Added **static dependency discovery and explicit manifests**, recorder input
  caching, and bounded missing-file retries so common papers can upload only
  the files they need without silently falling back to the whole repository.
- Added **immutable job snapshots** and selected-dependency watching so queued
  jobs compile the exact submitted version even when files change or several
  clients use the same server.
- Extended the existing compilation and deployment controls with
  **hardened LuaLaTeX options and restricted network egress**, random local
  project identities, and bounded logs and artifacts. These controls make
  private and shared lab deployments easier to operate under local security
  and data-handling requirements.
- Added **preview-first cleanup** for local caches and remote results,
  snapshots, and projects so operators can apply their own retention, deletion,
  and compliance policies without broad unreviewed deletion.
- Added versioned JSON commands, detached jobs, structured diagnostics with raw
  log locations, **Agent Skills, and a local STDIO MCP server** so coding agents
  can compile and debug papers without local TeX Live or unrestricted server
  tools.
- Added **pinned release inputs and release smoke tests**, native release
  checksums, provenance, attestations, and executable paper examples so
  published clients and images are easier to verify before deployment.

### remote-latexmk 0.3.0-rc.1

- Added a **version-pinned, non-root Linux server installer** with checksum
  verification, a private TeX Live tree, loopback-only defaults, and user-level
  status, upgrade, and uninstall commands.
- Added **version-pinned npm launcher and platform packages** for the existing
  Go client, without postinstall downloads or a local Go/TeX Live requirement.
- Added installable **Codex and Claude Code Plugins** that bundle the Skills and
  local MCP launcher, discover the current Agent workspace root, and reuse one
  verified user-level login across paper projects.

## Roadmap

This roadmap describes intended directions and does not promise release dates.

- To avoid conflicts and confusion with the standard local Perl `latexmk`
  command, adopt a distinct remote-client command such as **`rlatexmk`**, keep
  `latexmk` as an optional compatibility alias during a documented transition,
  and update Docker examples, MCP configurations, Agent Skills, and release
  packages together.
- To support Japanese journal and conference submissions that depend on
  pLaTeX, validate and add **pLaTeX and upLaTeX workflows** with `dvipdfmx`,
  including representative Japanese document classes, dependency discovery,
  Docker image coverage, and server-controlled engine arguments.
- To make released containers easier to reproduce and audit over time, pin
  **dated Debian and TeX Live package sources** in addition to the existing
  base image digests.
- To reduce client installation friction without weakening the current upload
  policy, evaluate a **pure TypeScript CLI/MCP** only after it matches the Go
  client's Git-ignore, manifest, path-boundary, dependency-discovery, cleanup,
  and cross-platform security behavior.
- To support shared lab servers and deployments that require stronger process
  and credential separation, add an **optional compiler-worker profile** where
  TeX receives only an immutable job snapshot and cannot access API
  credentials, database connections, or the complete service state.
- To reduce the effect of unexpected TeX behavior inside that worker, run each
  job **without outbound network access by default**, under a separate
  low-privilege identity, in a temporary per-job filesystem with explicit
  process, memory, time, log, and artifact limits.
- To keep more complex papers on the minimal-upload path, broaden
  **dependency discovery and diagnostic coverage** while keeping upload
  selection fail-closed and retaining bounded raw compiler logs as a fallback.

## License

[MIT](LICENSE)
