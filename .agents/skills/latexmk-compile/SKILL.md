---
name: latexmk-compile
description: Compile a LaTeX project through this repository's remote latexmk CLI and verify remote compiler readiness. Use for requests to build a `.tex` entry point in the current or specified directory, discover the configured project root, check the service health or available TeX engines/toolchain, or diagnose why this CLI cannot compile a project.
---

# Latexmk Compile

Use the remote CLI, not a TeX Live installation with the same `latexmk` name.

## Select the CLI and project directory

1. Treat the current directory as the project directory unless a directory is
   explicitly supplied. Resolve a supplied directory to an absolute path and
   store it in `PROJECT_DIR`; confirm that the entry `.tex` file exists within
   it. For the current directory, use `export PROJECT_DIR="$(pwd -P)"`.
2. Prefer the repository build at `packages/cli/dist/latexmk` while working in
   this repository. Otherwise use the installed command or `LATEXMK_CLI`.
3. Set `LATEXMK_CLI` to the selected executable, then verify it before relying
   on it:

   ```sh
   # Use an absolute repository build path here when working from this repository.
   export LATEXMK_CLI="${LATEXMK_CLI:-latexmk}"
   "$LATEXMK_CLI" help
   ```

   Its help must identify it as the "remote, PaaS-hosted LaTeX compiler". Build
   the repository CLI with `pnpm --filter @latexmk/cli build` if that executable
   is absent. Do not accidentally invoke the unrelated local TeX Live program.

## Check remote compilation readiness

Run checks from the target project directory so that the upward search for
`.latexmk.json` uses the project's configuration:

```sh
(
  cd -- "$PROJECT_DIR"
  "$LATEXMK_CLI" doctor
  "$LATEXMK_CLI" meta --json
)
```

`doctor` checks the health endpoint and then retrieves metadata. `meta` reports
the supported engines and available toolchain versions. Stop before compiling
if either command fails, or if the requested engine is not in `engines`.

Configure connectivity through `.latexmk.json` in the project or a parent
directory. Prefer `LATEXMK_TOKEN` for credentials; do not write a token to the
project or expose it in a command line. `LATEXMK_SERVER` and `LATEXMK_ENGINE`
override the corresponding file settings.

## Compile the target project

Compile from the project directory and set its upload and artifact roots
explicitly. This prevents a parent Git repository from being uploaded when the
target project is only a subdirectory:

```sh
(
  cd -- "$PROJECT_DIR"
  "$LATEXMK_CLI" --project-root . --out-dir . --engine xelatex main.tex
)
```

Replace `xelatex` and `main.tex` with the chosen supported engine and entry
file. Omit `--engine` only when the configured default is intended. Returned
PDFs, SyncTeX, logs, and allowed auxiliary files are written under `--out-dir`.
Use `--json` for machine-readable results and `latexmk clean main.tex` to
remove the supported generated files after a run.

Pass only CLI-supported options. This CLI accepts structured flags such as
`--timeout`, `--jobname`, `--no-synctex`, and `--quiet`; it deliberately rejects
arbitrary TeX or `latexmk` flags. Do not add `--shell-escape` unless the server
explicitly permits it and the user has authorized that risk.
