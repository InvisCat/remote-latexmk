# CLI workflow

Select the repository binary at `packages/cli/dist/latexmk` while developing this repository. Otherwise use the installed client binary. Confirm `latexmk help` describes the remote compiler before continuing.

Run commands from the paper root:

```sh
latexmk doctor
latexmk meta --json
latexmk files --json --project-root . main.tex
latexmk compile --detach --json --project-root . main.tex
latexmk jobs show --json JOB_ID
latexmk diagnostics --json JOB_ID
latexmk logs --json --source all --tail 200 --max-bytes 65536 JOB_ID
latexmk artifacts list --json JOB_ID
latexmk artifacts get --json --out-dir . JOB_ID ARTIFACT_ID
```

Use `.latexmk.json` for non-secret settings. Prefer `LATEXMK_TOKEN` or `LATEXMK_TOKEN_FILE` for the credential. Environment variables also support server, CA, engine, project ID, and upload-policy configuration.

The JSON commands use a versioned envelope. Check `ok`; on failure inspect `error.code`, `error.message`, `error.details`, and `error.retryable`. Do not scrape human-readable output when JSON is available.

`--detach` returns an immutable job. Poll at a reasonable interval and stop on a terminal state. Do not start repeated jobs while an earlier one is still useful.
