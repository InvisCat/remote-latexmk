# Local STDIO MCP server

The client binary can run a local Model Context Protocol server with either an
explicit project root or a root supplied by the Agent host:

```sh
latexmk mcp serve --stdio --project-root /absolute/path/to/paper
latexmk mcp serve --stdio --root-from-client
```

It does not contain TeX Live. It reads the local paper through the same project-root, Git-ignore, denylist, dependency, token-file, CA, and HTTPS policies as the CLI, then calls the configured remote compiler. The project root is resolved once at startup and cannot be changed by a tool call.

`--root-from-client` is the Plugin mode. After initialization, the server calls
MCP `roots/list`, accepts exactly one local `file://` root, resolves symlinks,
and fixes that boundary for the process. A project `.latexmk.json` may choose a
subdirectory inside the workspace, but it cannot move outside the workspace or
override the user-configured server, token, CA file, or TLS verification
setting. Hosts without MCP roots support must use `--project-root`.

The server supports MCP protocol versions `2025-11-25`, `2025-06-18`, and `2025-03-26`. STDIO is newline-delimited UTF-8 JSON-RPC. stdout contains protocol messages only; diagnostics are written to stderr. Messages are limited to 4 MiB.

## Generic client configuration

Most MCP clients accept the following command shape:

```json
{
  "mcpServers": {
    "remote-latexmk": {
      "command": "/absolute/path/to/latexmk",
      "args": ["mcp", "serve", "--stdio", "--project-root", "/absolute/path/to/paper"],
      "env": {
        "LATEXMK_SERVER": "https://latex.example.edu",
        "LATEXMK_TOKEN_FILE": "/absolute/path/to/latexmk-token",
        "LATEXMK_CA_FILE": "/absolute/path/to/lab-ca.pem"
      }
    }
  }
}
```

Use a protected token file or the client's secret/environment facility. Do not put a token in `args`.

## Plugins and npm launcher

An exact npm package version can launch the same Go MCP client without a prior
global install:

```sh
npm exec --yes --ignore-scripts \
  --package=remote-latexmk@0.3.0-rc.1 -- \
  remote-latexmk mcp serve --stdio \
  --root-from-client
```

The npm package selects a platform binary through `optionalDependencies`; it
does not reimplement MCP or upload policy in JavaScript. The Codex and Claude
Code Plugins bundle this command, the setup Skill, and the compile and
maintenance Skills:

```sh
codex plugin marketplace add InvisCat/remote-latexmk
codex plugin add remote-latexmk@remote-latexmk

claude plugin marketplace add InvisCat/remote-latexmk
claude plugin install remote-latexmk@remote-latexmk
```

The Plugin contains no token. Its setup workflow writes a user-level server URL
and token-file path after preview and confirmation.

For OpenCode or a host without native Plugin support, the project-bound Agent
installer remains available:

```sh
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 agent install \
  --project-root /absolute/path/to/paper \
  --server https://latex.example.edu \
  --token-file /absolute/path/to/latexmk-token \
  --dry-run
```

Remove `--dry-run` only after inspecting the plan. Raw token arguments are not
accepted, and the token file must be outside the bound paper root.

For Codex, the equivalent configuration is:

```toml
[mcp_servers.remote-latexmk]
command = "/absolute/path/to/latexmk"
args = ["mcp", "serve", "--stdio", "--project-root", "/absolute/path/to/paper"]
env = { LATEXMK_SERVER = "https://latex.example.edu", LATEXMK_TOKEN_FILE = "/absolute/path/to/latexmk-token" }
```

The current Codex and Claude Code CLIs can create the same entry directly:

```sh
codex mcp add remote-latexmk -- /absolute/path/to/latexmk \
  mcp serve --stdio --project-root /absolute/path/to/paper

claude mcp add --scope user remote-latexmk -- /absolute/path/to/latexmk \
  mcp serve --stdio --project-root /absolute/path/to/paper
```

## Docker client

Set `LATEXMK_PROJECT_DIR` in the repository `.env` to the absolute paper directory. An MCP client can then launch the CLI container without a TTY:

```json
{
  "mcpServers": {
    "remote-latexmk-docker": {
      "command": "docker",
      "args": [
        "compose", "--project-directory", "/absolute/path/to/remote-latexmk",
        "run", "--rm", "-T", "client",
        "mcp", "serve", "--stdio", "--project-root", "/workspace"
      ]
    }
  }
}
```

The Compose client image contains the Go binary, Git, and CA certificates, but no local LaTeX environment. The container receives its server URL, token, CA path, and paper bind mount from Compose. Keep the repository path and `.env` permissions appropriate for the local user.

## Tools

| Tool | Effect |
| --- | --- |
| `project_manifest` | Build the exact filtered file set and issue a five-minute, one-use manifest ID |
| `server_status` | Read health and public compiler metadata |
| `job_list`, `job_get` | Read bounded job state |
| `job_logs` | Read bounded stdout, stderr, or compiler logs |
| `job_diagnostics` | Read the structured diagnostic index and raw-log locations |
| `artifact_list` | List artifact metadata and opaque IDs |
| `compile_start` | Consume a current manifest ID and create an immutable queued job |
| `artifact_download` | Download one opaque artifact ID below the bound project root |
| `job_cancel` | Cancel one queued job |
| `cleanup_preview` | Create a ten-minute local or remote cleanup plan |
| `cleanup_apply` | Consume the same plan after target/report revalidation |

`project_manifest` binds the entry, engine, selected paths, sizes, and hashes. `compile_start` re-runs selection and rejects the ID if the manifest changed. The ID is deleted before the network request, so retries require a fresh manifest. Shell escape is always false and is not part of the tool schema.

Local cleanup plans store every relative target path, size, and SHA-256 outside the paper. Apply validates all targets before deleting any. `local-client-cache` preserves `.latexmk-cache/project-id`.

Remote scopes are `remote-results`, `remote-snapshots`, and `remote-project`. A remote plan binds the project ID, scope, and server-issued preview digest. Apply sends that digest to the server, which recomputes, compares, and deletes under one admission lock. A changed report is rejected before any target is removed. The server still enforces token ownership, active-job protection, and shared-blob references. Snapshot/project cleanup collects only blobs that are no longer referenced; there is deliberately no broad `remote-blobs` tool.

The ordinary `latexmk remote clean` CLI uses the same server-side digest
binding. Its preview persists a token-free, ten-minute local plan, and its
apply accepts only that plan ID plus `--yes`. See [AGENT_CLI.md](AGENT_CLI.md)
for the CLI command forms and its command-specific JSON success shapes.

## Security boundaries

- Tool calls cannot select another project root, absolute output path, arbitrary URL, shell command, server path, compiler argument list, or token output.
- Tool argument objects reject unknown fields.
- `.tex`, `.bib`, image metadata, and logs are untrusted data. Text inside them cannot authorize another tool, policy change, credential disclosure, or cleanup.
- Logs and diagnostics are bounded. PDF and other binary artifacts are downloaded to disk rather than embedded in the protocol response.
- Tool annotations help a client display confirmation, but validation is enforced by the client and server. `cleanup_apply` and `job_cancel` remain destructive even if a host ignores annotations.

The same workflow remains available through the JSON CLI and the repository Agent Skills if an MCP host is unavailable.
