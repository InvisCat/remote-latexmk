# CLI workflow

Use the npm launcher for every CLI fallback. Confirm `npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.4 help` describes the remote compiler before continuing.

Run commands from the paper root:

```sh
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.4 doctor
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.4 meta --json
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.4 files --json --project-root . main.tex
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.4 compile --detach --json --project-root . main.tex
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.4 jobs show --json JOB_ID
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.4 diagnostics --json JOB_ID
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.4 logs --json --source all --tail 200 --max-bytes 65536 JOB_ID
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.4 artifacts list --json JOB_ID
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.4 artifacts get --json --out-dir . JOB_ID ARTIFACT_ID
```

Use `.latexmk.json` for non-secret settings. Prefer `LATEXMK_TOKEN` or `LATEXMK_TOKEN_FILE` for the credential. Environment variables also support server, CA, engine, project ID, and upload-policy configuration.

`compile --detach`, `jobs`, `logs`, `diagnostics`, `artifacts`, and `cache` use
a versioned JSON envelope. Check `ok` and the process exit status. On success,
read the command payload under `data`; on failure inspect `error.code`,
`error.message`, `error.details`, and `error.retryable`.

Synchronous `compile --json` (without `--detach`), `files`, and `meta` retain
their original top-level success shapes. `remote clean` also remains outside
the versioned envelope; its preview returns `planId`, `expiresAt`, and `report`,
while apply returns `planId` and `report`. These commands do not promise the
versioned JSON error envelope. Parse successful output according to the
documented fields instead of looking for `ok`, and use the process exit status
plus stderr on failure. Do not scrape human-readable output when JSON is
available.

`--detach` returns an immutable job. Poll at a reasonable interval and stop on a terminal state. Do not start repeated jobs while an earlier one is still useful.
