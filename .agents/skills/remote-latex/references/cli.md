# CLI workflow

Select the repository binary at `packages/cli/dist/rlatexmk` while developing this repository. Otherwise use the installed client binary. Do not run an extra `help` probe during a normal compile workflow.

Run commands from the paper root:

```sh
rlatexmk doctor
rlatexmk meta --json
rlatexmk entries --json --project-root .
rlatexmk files --json --project-root . main.tex
rlatexmk compile --detach --json --project-root . main.tex
rlatexmk jobs show --json JOB_ID
rlatexmk diagnostics --json JOB_ID
rlatexmk logs --json --source all --tail 200 --max-bytes 65536 JOB_ID
rlatexmk artifacts list --json JOB_ID
rlatexmk artifacts get --json --out-dir . JOB_ID ARTIFACT_ID
```

Run `entries` only when the entry file is unknown. Use its `selected` path only
when `unambiguous` is true. If it returns multiple candidates, ask the user to
choose one. Do not use filesystem searches or source reads to create another
candidate list.

`files --json` is the only authority for upload dependencies for the chosen
entry. Do not construct, add to, or trim its dependency set with Agent
reasoning or other filesystem tools.

Use `.latexmk.json` for non-secret settings. Prefer `LATEXMK_TOKEN` or `LATEXMK_TOKEN_FILE` for the credential. Environment variables also support server, CA, engine, project ID, and upload-policy configuration.

`compile --detach`, `jobs`, `logs`, `diagnostics`, `artifacts`, and `cache` use
a versioned JSON envelope. Check `ok` and the process exit status. On success,
read the command payload under `data`; on failure inspect `error.code`,
`error.message`, `error.details`, and `error.retryable`.

Synchronous `compile --json` (without `--detach`), `entries`, `files`, and
`meta` retain their original top-level success shapes. `remote clean` also remains outside
the versioned envelope; its preview returns `planId`, `expiresAt`, and `report`,
while apply returns `planId` and `report`. These commands do not promise the
versioned JSON error envelope. Parse successful output according to the
documented fields instead of looking for `ok`, and use the process exit status
plus stderr on failure. Do not scrape human-readable output when JSON is
available.

`--detach` returns an immutable job. Poll at a reasonable interval and stop on a terminal state. Do not start repeated jobs while an earlier one is still useful.
