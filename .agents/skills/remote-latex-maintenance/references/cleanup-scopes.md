# Cleanup scopes

| Scope | Location | Removes | Does not remove |
| --- | --- | --- | --- |
| `local-generated` | Client project | Regular files ending in `.aux`, `.bbl`, `.bcf`, `.blg`, `.fdb_latexmk`, `.fls`, `.idx`, `.ilg`, `.ind`, `.log`, `.out`, `.run.xml`, `.synctex.gz`, `.toc`, or `.xdv` | Sources, unknown files, symlinks, `.git`, `node_modules`, `.latexmk-cache` |
| `local-client-cache` | Client project | `.latexmk-cache/dependencies.json` | `.latexmk-cache/project-id`, sources |
| `results` | Server | Stored result archives for terminal jobs in the selected project namespace | Terminal job metadata, current snapshot, active jobs |
| `snapshot` | Server | The current project snapshot and source blobs that become unreferenced | Result archives and job metadata; apply is rejected while a job is active |
| `project` | Server | Result archives, terminal job metadata, the current snapshot, and source blobs that become unreferenced | Apply is rejected while a job is active; blobs still referenced elsewhere |

The MCP names for the three server scopes are `remote-results`,
`remote-snapshots`, and `remote-project`. The CLI names remain `results`,
`snapshot`, and `project`.

Local cleanup uses a stored two-phase plan. Preview stores relative path, size,
and SHA-256 for each target for ten minutes. Apply validates every target before
deleting any. A missing, replaced, resized, or changed file invalidates the
operation. A later filesystem removal error can still produce a partial apply;
the JSON error reports removed and remaining target counts.

CLI remote cleanup is also a stored two-phase plan. Preview with `--scope`
stores a random, token-free plan for ten minutes. The plan binds the normalized
server URL, project ID, scope, and server-issued `planDigest`. Apply accepts
only `--plan-id PLAN_ID --yes`; combining `--scope` with apply is invalid. The
server recomputes the digest and compares it under the same admission lock used
for deletion, so a changed report is rejected before anything is removed. A
successful apply consumes the local plan.

With `--json`, remote preview returns the command-specific top-level object
`{planId, expiresAt, report}` and apply returns `{planId, report}`. These are
not version 1 envelopes and errors do not promise a JSON envelope. MCP remote
cleanup uses the same server digest binding through its own `cleanup_preview`
and `cleanup_apply` tools.

Never use `project` when a narrower scope meets the request. Never infer permission to delete from a request to compile, diagnose, or download.
