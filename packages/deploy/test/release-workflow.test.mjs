import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');

test('release workflow is tag-triggered with guarded npm recovery and pinned actions', async () => {
  const workflow = await readFile(path.join(root, '.github/workflows/release.yml'), 'utf8');
  assert.match(workflow, /tags:\s*\n\s*- 'v\*'/);
  assert.doesNotMatch(workflow, /pull_request:/);
  assert.match(workflow, /workflow_dispatch:[\s\S]*?tag:[\s\S]*?run_id:/);
  assert.match(workflow, /validate:\s*\n\s*if: github\.event_name == 'push'/);
  const uses = [...workflow.matchAll(/^\s*-?\s*uses:\s*([^\s]+)(?:\s+#.*)?$/gm)].map((match) => match[1]);
  assert.ok(uses.length >= 10, `expected pinned actions, got ${uses.length}`);
  for (const use of uses) {
    assert.match(use, /@[0-9a-f]{40}$/, `action is not pinned to a full SHA: ${use}`);
  }
  assert.equal((workflow.match(/goos:/g) ?? []).length, 6);
  for (const target of ['linux, goarch: amd64', 'linux, goarch: arm64', 'darwin, goarch: amd64', 'darwin, goarch: arm64', 'windows, goarch: amd64', 'windows, goarch: arm64']) {
    assert.ok(workflow.includes(`goos: ${target}`), `missing target ${target}`);
  }
  for (const image of ['remote-latexmk-server', 'remote-latexmk-server-full', 'remote-latexmk-client']) {
    assert.match(workflow, new RegExp(`image: ${image}(?:\\n|$)`));
  }
  assert.match(workflow, /SHA256SUMS/);
  assert.match(workflow, /server-binaries:[\s\S]*?goarch: \[amd64, arm64\]/);
  assert.match(workflow, /remote-latexmk-server_\*/);
  assert.match(workflow, /scripts\/install-server\.sh dist\/install-server\.sh/);
  assert.match(workflow, /test "\$\(wc -l < SHA256SUMS\)" -eq 9/);
  assert.match(workflow, /provenance: mode=max/);
  assert.match(workflow, /sbom: true/);
  assert.match(workflow, /attestations: write/);
  assert.match(workflow, /release_args\+=\(--prerelease\)/);
  assert.match(workflow, /smoke-papers:[\s\S]*?run: make smoke-papers/);
  assert.match(workflow, /tags: type=raw,value=candidate-\$\{\{ github\.sha \}\}/);
  assert.match(workflow, /SMOKE_SLIM_SERVER_IMAGE: ghcr\.io[^\n]+:candidate-\$\{\{ github\.sha \}\}/);
  assert.match(workflow, /SMOKE_FULL_SERVER_IMAGE: ghcr\.io[^\n]+:candidate-\$\{\{ github\.sha \}\}/);
  assert.match(workflow, /SMOKE_CLIENT_IMAGE: ghcr\.io[^\n]+:candidate-\$\{\{ github\.sha \}\}/);
  assert.match(workflow, /publish-images:[\s\S]*?needs: \[validate, images, smoke-papers\]/);
  assert.match(workflow, /publish-images:[\s\S]*?packages: write/);
  assert.match(workflow, /publish-images:[\s\S]*?docker buildx imagetools create/);
  assert.match(workflow, /needs: \[validate, binaries, server-binaries, publish-images\]/);
  assert.match(workflow, /npm-packages:[\s\S]*?if: github\.event_name == 'push' && vars\.NPM_PUBLISH_ENABLED == 'true'/);
  assert.match(workflow, /stage-packages\.mjs/);
  assert.match(workflow, /if \[\[ "\$\{package_version\}" == \*-\* \]\]; then/);
  assert.match(workflow, /npm publish "\$\{package_dir\}" --access public --provenance --tag "\$\{npm_tag\}"/);
  assert.equal((workflow.match(/registry-url: 'https:\/\/registry\.npmjs\.org'/g) ?? []).length, 2);
  assert.equal((workflow.match(/package-manager-cache: false/g) ?? []).length, 2);
  const recoveryJob = workflow.slice(workflow.indexOf('\n  npm-recovery:'));
  assert.match(recoveryJob, /if: github\.event_name == 'workflow_dispatch' && vars\.NPM_PUBLISH_ENABLED == 'true'/);
  assert.match(recoveryJob, /actions: read/);
  assert.match(recoveryJob, /id-token: write/);
  assert.match(recoveryJob, /git rev-parse "\$\{RELEASE_TAG\}\^\{commit\}"/);
  assert.match(recoveryJob, /test "\$\(jq -r '\.head_sha'/);
  assert.match(recoveryJob, /\.github\/workflows\/release\.yml/);
  assert.match(recoveryJob, /pattern: client-\*/);
  assert.match(recoveryJob, /run-id: \$\{\{ inputs\.run_id \}\}/);
  assert.doesNotMatch(recoveryJob, /go run|build-push-action/);
  const imagesJob = workflow.slice(workflow.indexOf('\n  images:'), workflow.indexOf('\n  smoke-papers:'));
  const publishJob = workflow.slice(workflow.indexOf('\n  publish-images:'), workflow.indexOf('\n  release:'));
  assert.doesNotMatch(imagesJob, /type=semver|value=latest/);
  assert.match(publishJob, /type=semver/);
  assert.match(publishJob, /value=latest/);
  assert.match(publishJob, /manifest_digest\(\)/);
  assert.match(publishJob, /digest == "" \{ digest = \$2 \} END \{ if \(digest == ""\) exit 1; print digest \}/);
  assert.doesNotMatch(publishJob, /print \$2; exit/);
});

test('release recovery only republishes artifacts from the matching failed tag run', async () => {
  const workflow = await readFile(path.join(root, '.github/workflows/release-recovery.yml'), 'utf8');
  assert.match(workflow, /workflow_dispatch:/);
  assert.match(workflow, /actions: read/);
  assert.match(workflow, /contents: write/);
  assert.match(workflow, /attestations: write/);
  const uses = [...workflow.matchAll(/^\s*-?\s*uses:\s*([^\s]+)(?:\s+#.*)?$/gm)].map((match) => match[1]);
  assert.ok(uses.length >= 4, `expected pinned actions, got ${uses.length}`);
  for (const use of uses) {
    assert.match(use, /@[0-9a-f]{40}$/, `action is not pinned to a full SHA: ${use}`);
  }
  assert.match(workflow, /git rev-parse "\$\{RELEASE_TAG\}\^\{commit\}"/);
  assert.match(workflow, /test "\$\(jq -r '\.head_sha'/);
  assert.match(workflow, /test "\$\(jq -r '\.path'/);
  assert.match(workflow, /\.github\/workflows\/release\.yml/);
  assert.match(workflow, /pattern: client-\*/);
  assert.match(workflow, /pattern: server-\*/);
  assert.equal((workflow.match(/run-id: \$\{\{ inputs\.run_id \}\}/g) ?? []).length, 2);
  assert.equal((workflow.match(/github-token: \$\{\{ secrets\.GITHUB_TOKEN \}\}/g) ?? []).length, 2);
  assert.match(workflow, /test "\$\(wc -l < SHA256SUMS\)" -eq 9/);
  assert.match(workflow, /gh release create "\$\{RELEASE_TAG\}" dist\/\*/);
  assert.doesNotMatch(workflow, /go run|build-push-action|npm publish/);
});

test('CI installs pinned pnpm before setup-node enables pnpm caching', async () => {
  const workflow = await readFile(path.join(root, '.github/workflows/ci.yml'), 'utf8');
  const pnpmSetup = workflow.indexOf('pnpm/action-setup@0ebf47130e4866e96fce0953f49152a61190b271');
  const nodeSetup = workflow.indexOf('actions/setup-node@');
  assert.ok(pnpmSetup >= 0, 'missing pinned pnpm setup action');
  assert.ok(nodeSetup > pnpmSetup, 'pnpm must be installed before setup-node resolves the pnpm cache');
  assert.doesNotMatch(workflow, /corepack enable pnpm/);
  assert.match(workflow, /- run: pnpm test\n\s+- run: pnpm build/);
});

test('container inputs and GHCR compose path are pinned', async () => {
  const files = await Promise.all([
    'compose.yaml',
    '.env.example',
    'packages/cli/Dockerfile',
    'packages/deploy/templates/Dockerfile.slim',
    'packages/deploy/templates/Dockerfile.full',
    'packages/deploy/src/index.ts',
  ].map(async (name) => [name, await readFile(path.join(root, name), 'utf8')]));
  for (const [name, content] of files) {
    for (const line of content.split('\n')) {
      if (/(?:image:|IMAGE[:=])/.test(line) && /\b(?:golang|debian|caddy|postgres|node|texlive\/texlive):/.test(line)) {
        assert.match(line, /@sha256:[0-9a-f]{64}/, `${name} has an unpinned image: ${line}`);
      }
    }
  }
  const override = await readFile(path.join(root, 'compose.ghcr.yaml'), 'utf8');
  const envExample = await readFile(path.join(root, '.env.example'), 'utf8');
  assert.equal((override.match(/pull_policy: always/g) ?? []).length, 3);
  assert.match(override, /ghcr\.io\/\$\{LATEXMK_GHCR_NAMESPACE/);
  assert.match(override, /remote-latexmk-server/);
  assert.match(override, /remote-latexmk-client/);
  assert.match(override, /LATEXMK_GHCR_NAMESPACE:-inviscat/);
  assert.match(override, /LATEXMK_GHCR_VERSION:-0\.3\.0-rc\.2/);
  assert.doesNotMatch(override, /billstark001/);
  assert.doesNotMatch(override, /LATEXMK_GHCR_(?:NAMESPACE|VERSION):\?/);
  assert.doesNotMatch(override, /:latest/);
  assert.match(envExample, /^COMPOSE_PATH_SEPARATOR=:\s*$/m);
  assert.match(envExample, /^COMPOSE_FILE=compose\.yaml:compose\.ghcr\.yaml\s*$/m);
});
