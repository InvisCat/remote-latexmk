# Agent-facing CLI contract

Status: version 1 implemented for user setup, detached compile, jobs, logs,
diagnostics, artifacts, and local cache inspection/cleanup.

This contract is for local agents and scripts. The CLI uses the same token,
CA, timeout, and HTTPS configuration as interactive commands. It never prints
the token or Authorization header.

## Compatibility

`setup --json`, detached `compile --json`, jobs, logs, diagnostics, artifacts,
and local cache commands use a versioned JSON envelope. Existing JSON success output from
synchronous `compile --json` (without `--detach`), `entries`, `files`, and
`meta` remains unchanged for now. `remote clean` also remains outside the versioned envelope;
its two-stage preview and apply success shapes are documented below. These
compatibility commands will move to the versioned contract only through an
explicit compatibility mechanism. Their top-level success shapes will not
change silently, and they do not promise a versioned JSON error envelope.

## Envelope

Success:

```json
{
  "schemaVersion": 1,
  "ok": true,
  "command": "jobs.show",
  "data": {}
}
```

Failure:

```json
{
  "schemaVersion": 1,
  "ok": false,
  "command": "jobs.show",
  "error": {
    "code": "not_found",
    "message": "server returned 404 Not Found: job not found",
    "details": {"httpStatus": 404},
    "retryable": false
  }
}
```

For versioned commands, `--json` writes exactly one JSON value followed by a
newline. Progress text belongs on stderr. Consumers must check both `ok` and
the process exit status. Unknown fields must be ignored within the same schema
version. Compatibility commands guarantee their documented JSON shape only on
success.

Exit status:

- `0`: command succeeded;
- `1`: remote or operational failure;
- `2`: invalid arguments or local configuration;
- `124`: timeout.

Stable error codes currently include:

- `invalid_arguments`;
- `authentication_failed`;
- `not_found`;
- `conflict`;
- `rate_limited`;
- `http_error`;
- `server_error`;
- `network_error`;
- `timeout`;
- `cancelled`;
- `command_failed`.
- `unsupported_capability`.
- `result_not_ready`;
- `result_unavailable`.
- `artifact_not_found`.

## User setup

Human users can create the client-side token file and user configuration in
one interactive command:

```sh
latexmk auth login --server https://latex.example.edu
```

The remote-latexmk API token is read from a hidden terminal prompt. This
command verifies the server and token before saving either file. It is
intentionally interactive and has no Agent JSON mode. Agents must not request
the token or run the prompt on the user's behalf. The setup commands below
remain the non-secret preview/apply interface for an existing token file.

```sh
latexmk setup --server https://latex.example.edu \
  --token-file /absolute/path/to/latexmk-token --json
latexmk setup --server https://latex.example.edu \
  --token-file /absolute/path/to/latexmk-token --yes --json
```

The first command is preview-only and returns `setup.preview`. The second
returns `setup.apply` and writes the primary user configuration atomically with
mode `0600`. Both results include the server URL, user configuration path,
token file path, and optional CA file path. Neither returns or stores the token
value. Raw `--token` values are rejected. On Unix, the token file must not be
readable by group or other users.

The setup command accepts `--ca-file` for a private CA and
`--clear-ca-file` to remove a previous CA setting. It preserves unrelated user
options. An existing legacy user configuration is read for migration, while
new writes go to the `remote-latexmk` user configuration directory.

The server address is normalized before any token prompt. A bare host or an
explicit `http://` URL without a port uses port 8080. An `https://` URL without
a port uses standard port 443. `auth login` checks public health, service
identity, and protocol compatibility before reading the token, then verifies
authenticated read access before saving the login.

## Entry discovery and upload manifest

When the entry is unknown, use the deterministic, policy-filtered entry
discovery command:

```sh
latexmk entries --project-root . --json
```

Its compatibility JSON result contains `status`, `selected`, `unambiguous`,
bounded `candidates`, counts, and optional warnings. Use `selected` only when
`unambiguous` is true. Ask the user to choose when candidates are ambiguous.
Do not build another candidate list by searching or reading project files.

After choosing the entry, use the normal manifest command:

```sh
latexmk files --project-root . --json main.tex
```

The returned file set is the sole authority for upload dependencies. An Agent
must not add, remove, or replace paths with its own filesystem searches, file
reads, or model reasoning. Both `entries` and `files` keep their documented
unversioned compatibility success shapes; check process status and stderr on
failure.

## Detached compile

```sh
latexmk compile --detach --json main.tex
```

The command applies the normal project-root, Git-ignore, denylist, dependency,
manifest, CA, and token policies. It uploads the selected files, commits one
immutable snapshot, and returns after the queued job is created. It does not
poll, download artifacts, or perform automatic missing-file retries.

The success command is `compile.start`. Its data contains `job` and optional
manifest-selection `warnings`. Detached compile requires a server that supports
queued jobs and incremental uploads. Use `jobs show` to poll the returned job
ID.

## Jobs

```sh
latexmk jobs list --limit 50 --json
latexmk jobs show JOB_ID --json
latexmk jobs cancel JOB_ID --json
```

`jobs.list` returns `jobs`, `count`, and the applied `limit`. Jobs are sorted
newest first, with ID as the stable tie-breaker. `jobs.show` and `jobs.cancel`
return one job object. Cancel only succeeds while the remote job is queued.

## Logs, diagnostics, and artifacts

```sh
latexmk logs JOB_ID --source all --tail 200 --max-bytes 65536 --json
latexmk diagnostics JOB_ID --json
latexmk artifacts list JOB_ID --json
latexmk artifacts get JOB_ID ARTIFACT_ID --out-dir ./build --json
```

Logs distinguish `stdout`, `stderr`, and TeX-generated `compiler` logs. The
byte limit applies across all returned entries and is capped at 4 MiB. Content
is streamed through a bounded tail buffer; large PDFs and unrelated artifacts
are not loaded into memory. Compiler logs are checked against job metadata.

`diagnostics.get` scans the complete raw log streams without retaining them in
memory. It returns at most 100 deduplicated common TeX errors and warnings.
Each diagnostic contains `severity`, optional project-relative `file` and
`line`, `message`, optional bounded `context`, and one or more `logLocations`.
`fileInferred: true` distinguishes a filename inferred from TeX's open-file
trace from an explicit `-file-line-error` location.
A location identifies the raw log `source`, `path`, and exact `startLine` and
`endLine`. Duplicate messages from stdout and a compiler log are merged while
keeping both locations. Messages, source context, locations, selected log
count, and input line length are bounded. `incomplete: true` means the caller
must inspect `logs`; an empty or apparently insufficient index should also
fall back to raw logs. Diagnostics are an index, not an authoritative parser or
a replacement for the original output.

Artifact list returns an opaque, deterministic 128-bit ID derived from the
declared project-relative artifact path. Download accepts only that ID, checks
size and SHA-256, rejects unsafe output paths and symlinks, and returns the
absolute local path and MIME type. Binary data is never embedded in JSON.

List output is bounded to 1 through 200 jobs. Log, diagnostic, and artifact
commands use separate bounded contracts; they do not embed PDF data or
unbounded logs in this envelope.

## Local cache inspection and cleanup

```sh
latexmk cache inspect --project-root . --json
latexmk cache ignore --project-root . --json
latexmk cache clean --project-root . --scope local-generated --json
latexmk cache clean --project-root . --plan-id PLAN_ID --yes --json
```

Inspection returns dependency-cache entry counts without returning cached input
paths, plus the count and size of known local generated files, including common
index products (`.idx`, `.ind`, and `.ilg`). Cleanup preview
creates a random, ten-minute plan under the platform user cache directory. Each
target is bound by project-relative path, size, and SHA-256. Apply revalidates
every target before deleting any and rejects a changed, missing, symlinked, or
expired plan. `local-client-cache` deletes only dependency discovery state and
preserves the random project ID.

If a filesystem removal fails after deletion starts, the command returns a
`cleanup_apply_failed` envelope with `removed`, `reclaimedBytes`, `failedPath`,
and `remainingTargets`. Inspect current state and create a new preview; do not
reuse the partially applied plan.

`cache ignore` is the explicit opt-in command that appends `.latexmk-cache/` to
the project `.gitignore`. Its JSON result reports the absolute project root,
the `.gitignore` path, and whether the file changed. It is idempotent and does
not modify an existing effective ignore policy. `git clean -fdX` still removes
ignored cache files and therefore resets the local project identity.

There is no direct `--scope ... --yes` local cleanup form. The caller must use
the `planId` returned by preview.

## Remote cleanup

```sh
latexmk remote clean --scope results|snapshot|project --json
latexmk remote clean --plan-id PLAN_ID --yes --json
```

Preview asks the server for the selected token-owned project report, then
stores a random local plan for ten minutes. The plan contains the normalized
server URL, project ID, scope, and server-issued `planDigest`; it does not store
the bearer token. Apply reloads those values and requires current credentials.
It accepts only `--plan-id PLAN_ID --yes`: `--scope` cannot be repeated or
changed during apply, and `--plan-id` without `--yes` is invalid.

The server recomputes the cleanup report digest under the same admission lock
used for deletion. If server state changed, apply is rejected before removing
anything. A successful apply consumes the local plan. Expired, consumed,
wrong-server, and wrong-project plans are rejected; create a fresh preview
instead of editing or reusing a plan file.

`remote clean --json` remains a compatibility command rather than a version 1
envelope. Preview success is a top-level object with `planId`, `expiresAt`, and
`report`; apply success has `planId` and `report`. Consumers check the process
status and these command-specific fields. Failure does not promise a JSON error
envelope.

## MCP mapping

`latexmk mcp serve --stdio` exposes the same client operations as strict MCP
tools. `--project-root` fixes an explicit root; `--root-from-client` requests
one local workspace root from a Plugin host and fixes that root for the
process. MCP success and error results contain structured JSON plus an
equivalent text content item for older hosts. See [MCP.md](MCP.md) for tool
schemas, manifest lifetime, cleanup plans, and native/Docker configuration.
