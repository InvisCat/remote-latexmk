# Publishing remote-latexmk

This repository uses a Release PR for every version. The root `package.json`
is the only source of truth for the release version. Other package manifests,
Plugin metadata, Compose defaults, Skills, and live documentation are derived
from it and checked in CI.

## Repository identity

Use `remote-latexmk` as the public repository and product name. Keep the client
command `latexmk`, the `LATEXMK_*` environment variables, existing Compose
state names, and Go module paths unchanged for the first fork release. Those
interfaces affect existing users and can be reconsidered separately.

Suggested GitHub description:

> Self-hosted remote LaTeX compiler with Docker Compose, dependency-aware
> uploads, native/Docker clients, MCP, and Agent Skills.

Suggested topics:

```text
latex
latexmk
texlive
self-hosted
latex-compiler
remote-compilation
docker-compose
xelatex
lualatex
pdflatex
mcp-server
agent-skills
research-tools
```

Add Agent-specific topics such as `codex`, `claude-code`, and `opencode` only
after their documented installation paths have been tested from the public
repository.

Upload `docs/assets/remote-latexmk-social-preview.png` as the GitHub social
preview. Its source is `docs/assets/remote-latexmk-hero.svg`. Keep the product
name and main motivation readable when the preview is shown at a small size.

## Repository release checklist

1. Create or rename the public fork to `remote-latexmk`.
2. Confirm public links use `InvisCat/remote-latexmk` and GHCR image paths use
   the lowercase `inviscat` namespace.
3. Set the Git remote to the public fork. Do not silently rewrite the Go module
   path in the same change; that is a separate compatibility decision.
4. Add the description, topics, social preview, website URL if one exists, and
   MIT license metadata in GitHub repository settings.
5. Review the inherited Git history for credentials, private paths, paper
   content, and generated artifacts before making the fork public. Removing a
   value only from the latest tree does not remove it from Git history.
6. Run the full validation suite described in `VALIDATION.md`.
7. Test the source-build Compose quick start from a clean clone.
8. Test all four Skills from a clean checkout and validate the native and Docker
   MCP examples.
9. Confirm that the release workflow has permission to write packages and
   create releases, and that npm trusted publishing is configured.
10. Validate the shared Codex/Claude Plugin directory and both marketplace
    manifests. Install it from a clean temporary Agent home before publishing.

## Release artifacts

The Release PR workflow is designed to publish:

- `remote-latexmk-server` for `linux/amd64`;
- `remote-latexmk-server-full` for `linux/amd64`;
- `remote-latexmk-client` for `linux/amd64` and `linux/arm64`;
- native client archives for Linux, macOS, and Windows on amd64 and arm64;
- native server archives for Linux on amd64 and arm64;
- a versioned `install-server.sh` release asset;
- `SHA256SUMS` for all native archives and the installer;
- npm launcher and platform packages through trusted publishing;
- repository marketplace manifests for the Codex and Claude Code Plugin.

The full server image is intentionally separate because it is much larger than
the default slim server image. Do not claim multi-platform server support
until the server image matrix actually builds and passes on those platforms.

## Release PR workflow

Create a release branch from the current `main`, then let the version tool
update every derived reference:

```sh
git switch -c release/v0.3.0-rc.4
pnpm release:prepare 0.3.0-rc.4
pnpm test
```

Review the generated diff and open a PR. A Release PR should contain the
version change, generated version references, release notes or documentation
changes, and any release infrastructure changes intended for that version.
Do not create or move the Git tag by hand.

When the Release PR is merged into `main`, the root `package.json` change
starts `.github/workflows/release.yml`. The workflow:

1. verifies that the version increased and all derived files match it;
2. builds candidate images and native archives;
3. runs `make smoke-papers` against commit-specific candidate
   image tags. Only a passing run promotes those digests to the release's
   semver tags and, for stable releases, `latest`;
4. creates or verifies an annotated tag at the tested commit;
5. publishes the six npm platform packages and the launcher, verifying the
   unpacked contents of any package that already exists;
6. promotes the tested GHCR image digests; and
7. creates the public GitHub Release last, with checksums and attestations.

For a transient registry or runner failure, rerun the failed workflow jobs.
The same version can also be retried from the current `release.yml` by manually
dispatching it with the existing version. A manual retry checks out the
immutable tag and refuses a mismatched commit. Do not increase the version for
a retry that publishes identical artifacts. Use a new version when source,
package metadata, or artifact bytes change.

Validate Plugin metadata and generated Skills before merging the Release PR:

```sh
pnpm sync:plugin-skills
python3 /path/to/plugin-creator/scripts/validate_plugin.py \
  plugins/remote-latexmk
claude plugin validate plugins/remote-latexmk
claude plugin validate .
```

Run the Skill validator for `remote-latex`, `remote-latex-maintenance`,
`remote-latex-server`, and `remote-latex-setup`. The root test command also runs
`sync-plugin-skills.mjs --check` so stale generated Plugin Skills fail CI.

## npm trusted publishing

npm is a required release channel because the Agent Quick Start uses the npm
launcher. Before the first release from a new repository or scope:

1. reserve the public `remote-latexmk` package and the six
   `@rlatexmk/rlatexmk-*` platform packages;
2. configure each npm package to trust this repository's `release.yml`
   workflow;
3. confirm the npm account and scope ownership;
4. test a prerelease Release PR before a stable release.

The job stages platform packages from the same six deterministic client
archives attached to the GitHub release. It publishes platform packages first
and the main launcher last. None of the published packages has a lifecycle
script that downloads executable code. A retry skips an existing package only
when its file list, modes, sizes, and contents match the local package.

## Promote released paths in the README

Only after the artifacts exist:

1. Make the tagged GHCR Compose path the shortest production quick start.
2. Keep the source-build Compose path as a documented fallback.
3. Link the release page and checksum file from the native client section.
4. Remove the pre-release badge and statements that no public artifacts exist.
5. Test native Plugin installation with the public repository:

   ```sh
   codex plugin marketplace add InvisCat/remote-latexmk
   codex plugin add remote-latexmk@remote-latexmk

   claude plugin marketplace add InvisCat/remote-latexmk
   claude plugin install remote-latexmk@remote-latexmk
   ```

6. Test the manual cross-Agent Skill command used by OpenCode and advanced
   setups:

   ```sh
   npx skills add InvisCat/remote-latexmk -g \
     --skill remote-latex \
     --skill remote-latex-maintenance \
     --skill remote-latex-server \
     --skill remote-latex-setup \
     --agent codex \
     --agent claude-code \
     --agent opencode
   ```

7. Add Agent-specific GitHub topics only for the integrations that pass this
   test.

## Immutable deployment references

Human-readable versions are suitable for trying a release. Long-lived
deployments should use complete image references with digests:

```dotenv
LATEXMK_GHCR_SERVER_IMAGE=ghcr.io/inviscat/remote-latexmk-server@sha256:SERVER_DIGEST
LATEXMK_GHCR_CLIENT_IMAGE=ghcr.io/inviscat/remote-latexmk-client@sha256:CLIENT_DIGEST
```

Do not put `sha256:...` in `LATEXMK_GHCR_VERSION`; that produces an invalid
tag-shaped reference rather than a digest reference.

## Search and AI discovery audit

For each release, check that the repository name, description, topics, README
title, first paragraph, and release notes consistently use these plain search
terms where they are accurate:

- self-hosted remote LaTeX compiler;
- TeX Live server without local TeX Live;
- Docker Compose LaTeX compilation;
- dependency-aware and Git-ignore-aware upload;
- LaTeX MCP server and Agent Skills.

Keep the first README commands copyable and keep machine-relevant facts in
normal text, not only in the hero image. Review `docs/AI_AGENTS.md`, the checked
in Skills, JSON contracts, and examples whenever a CLI or policy behavior
changes. Accurate, stable instructions are more useful to coding agents than a
large list of loosely related keywords.
