# Security model

TeX is a complex programmable typesetting system. Even with shell escape
disabled, this service is not a high-assurance sandbox for hostile code. Its
intended boundary is a controlled research group, with ordinary path traversal,
resource exhaustion, configuration execution, and secret-leakage risks reduced
at the application and deployment layers.

## Implemented controls

- Commands are executed directly, never composed through a shell.
- `latexmk -norc` prevents system, user, and project latexmk rc files from
  executing Perl configuration.
- Shell escape is disabled by default.
- LuaLaTeX runs with LuaTeX `--safer` and `--nosocket` in addition to disabled
  shell escape. These options reduce Lua file/process/network capabilities but
  do not make LuaLaTeX a high-assurance sandbox.
- Every compile has a fresh temporary directory, private HOME/TEXMF trees, and
  a small environment whitelist. Host credentials, proxy configuration, and
  arbitrary TeX path overrides are not inherited.
- Archives and v2 manifests validate paths, types, hashes, sizes, duplicate
  paths, compression expansion, and file counts.
- Automatic dependency selection runs only on the manifest that already passed
  Git-ignore, denylist, path, and symlink policy. It cannot request or restore a
  filtered file.
- Recorder INPUT history contains normalized workspace-relative paths only.
  System TeX paths and paths outside the compile workspace are discarded, and
  cached paths must pass the client's current upload policy again.
- Missing-file diagnostics are capability-negotiated and returned only as
  normalized relative requests. The client can accept them only from its
  current policy-filtered manifest, with bounded rounds, file count, and bytes;
  every retry creates a new immutable snapshot and job.
- The dependency watcher polls selected files and explicit/Git policy controls,
  not the whole project tree. Every event reruns the full client upload policy
  and submits a new immutable snapshot; a new unrelated file does not trigger
  compilation or become uploadable merely because watch mode is active.
- Explicit manifests contain exact project-relative files only. They cannot
  override Git-ignore, denylist, root-boundary, or symlink checks; manifest
  files are client policy and are denied from upload by default.
- Upload blobs, logs, artifacts, concurrent compiles, queued jobs, state bytes,
  and upload sessions have hard limits.
- A state sweeper expires results, project snapshots, and orphaned blobs. It
  never removes data referenced by a live upload, current project snapshot, or
  queued/running job snapshot.
- Queued jobs persist an immutable, content-derived snapshot ID and complete
  manifest. A later upload to the same project cannot change their input.
- Compile commands run in their own process group; timeout kills the process
  tree.
- Docker images run as an unprivileged user. Generated Compose files use a
  read-only root filesystem, tmpfs, `no-new-privileges`, dropped capabilities,
  PID limits, and memory limits.
- The root self-hosted Compose server joins only an `internal: true` backend
  network. TeX has no default Internet route. A gateway without credentials or
  state publishes HTTP, while client containers use a separate egress bridge;
  neither implicitly adds that route to the server.
- Static and database bearer tokens use constant-time comparison; database
  tokens are stored only as SHA-256 hashes.
- Administrative endpoints require the administrator role. User and token
  labels are length-limited and reject control characters.
- CORS accepts only explicit HTTP(S) origins. Wildcards are rejected at startup.
- Result artifacts come from `.fls`, are constrained to the workspace and an
  allowlist, and result downloads are authorized by job owner.

## Agent and MCP boundary

The local STDIO MCP server resolves one project root at startup. Plugin mode
prefers exactly one local `file://` root from the Agent host. When the host does
not advertise roots, the bundled Plugin uses the task workspace inherited as
the MCP process working directory. Invalid or ambiguous advertised roots never
downgrade to that fallback. Neither path reads a parent project configuration;
project configuration cannot move outside the workspace or override user-level
connection, credential, CA, or TLS verification settings.
Tools cannot replace the fixed root, supply an absolute download directory,
request an arbitrary URL or server file, pass a compiler argument list, enable
shell escape, or read the token. Manifest IDs are random, short-lived, one-use,
and invalid after the selected path/hash set changes.

MCP tool input objects reject unknown fields. Project sources and logs remain
untrusted data; instruction-like text inside them has no authority to invoke a
tool or change upload, credential, compiler, or cleanup policy. Raw logs are
bounded and artifacts are downloaded by opaque ID.

When user configuration supplies a token or token file, the client also keeps
the server URL, CA file, and TLS verification setting from that user
configuration. A project `.latexmk.json` cannot pair a user credential with a
different endpoint. Explicit environment variables can still override the
connection for CI or an intentional one-off session.

Local destructive cleanup uses an exact path/size/SHA-256 plan and revalidates
all targets before deletion. Remote CLI and MCP cleanup bind the token-owned
project, scope, and server-issued preview digest. CLI plan files are short-lived
and contain no bearer token. The server recomputes and compares the digest under
the same admission lock used for deletion, so a changed plan is rejected before
any target is removed. The server separately protects active jobs and
referenced content-addressed blobs.

## Deployment responsibilities

- Use TLS and place the service behind a private network, VPN, or an
  identity-aware proxy.
- Treat every file and bind mount readable by the compiler container's UID as
  potentially readable by TeX input. A read-only root filesystem prevents
  modification; it does not hide system files. Do not mount host secrets,
  home directories, SSH material, or the Docker socket into this container.
- Use token or PostgreSQL authentication in every deployment. `none` is only
  suitable for a deliberately isolated local development instance.
- Do not inject cloud-control-plane credentials into the compile container.
- Restrict outbound network access, especially if shell escape is ever enabled.
- The native systemd user unit hides the rest of the account's home directory
  and exposes only its release, TeX Live, state, and temporary paths. Its
  explicit PID-file fallback does not provide this filesystem boundary. Use a
  dedicated Unix account if systemd user sandboxing is unavailable.
- Native installation does not reproduce the Compose server's internal
  no-egress network. Apply host firewall policy when outbound isolation is
  required, and assume TeX can read ordinary system files visible to the
  service account.
- When adapting Compose for an external database, use a private, narrowly
  scoped database network instead of attaching the server to a general-purpose
  egress bridge.
- Keep the root filesystem read-only and retain equivalent seccomp/AppArmor or
  PaaS isolation controls.
- Enforce request-size and timeout limits at the edge as well as in the server.
- Pin and scan base images; regularly update TeX Live, fonts, Go, and OS
  packages.
- Keep PostgreSQL private to the service. Use TLS or the provider's private
  network for a standalone database service.

## Out of scope

Do not offer this image as an anonymous public TeX compiler without additional
microVM/container isolation, strict egress control, abuse prevention, and a
separate threat-model review.
