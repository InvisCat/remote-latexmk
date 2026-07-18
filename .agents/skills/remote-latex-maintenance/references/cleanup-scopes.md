# Cleanup scopes

| Scope | Location | Removes | Does not remove |
| --- | --- | --- | --- |
| `local-generated` | Client project | Known `.aux`, `.log`, `.fls`, SyncTeX, bibliography, index, and related generated files | Sources, unknown files, symlinks, `.git`, `node_modules`, `.latexmk-cache` |
| `local-client-cache` | Client project | `.latexmk-cache/dependencies.json` | `.latexmk-cache/project-id`, sources |
| `results` | Server | Terminal job results and result archives in the selected project namespace | Current snapshot, active jobs |
| `snapshot` | Server | The current project snapshot when server policy permits | Results unless the API reports them as part of the selected scope |
| `project` | Server | The project namespace's removable snapshot, terminal results, jobs, and unreferenced blobs | Active jobs, blobs still referenced elsewhere |

The MCP names for the three server scopes are `remote-results`,
`remote-snapshots`, and `remote-project`. The CLI names remain `results`,
`snapshot`, and `project`.

Local cleanup is an immutable two-phase plan. Preview stores relative path, size, and SHA-256 for each target for ten minutes. Apply validates every target before deleting any. A missing, replaced, resized, or changed file invalidates the operation.

Remote cleanup uses the server's preview response and the identical scope for confirmation. It is separate from the local plan store.

Never use `project` when a narrower scope meets the request. Never infer permission to delete from a request to compile, diagnose, or download.
