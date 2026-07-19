import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');

test('release workflow is tag-only and pins third-party actions', async () => {
  const workflow = await readFile(path.join(root, '.github/workflows/release.yml'), 'utf8');
  assert.match(workflow, /tags:\s*\n\s*- 'v\*'/);
  assert.doesNotMatch(workflow, /pull_request:|workflow_dispatch:/);
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
  assert.match(workflow, /provenance: mode=max/);
  assert.match(workflow, /sbom: true/);
  assert.match(workflow, /attestations: write/);
  assert.match(workflow, /release_args\+=\(--prerelease\)/);
});

test('CI installs pinned pnpm before setup-node enables pnpm caching', async () => {
  const workflow = await readFile(path.join(root, '.github/workflows/ci.yml'), 'utf8');
  const pnpmSetup = workflow.indexOf('pnpm/action-setup@b906affcce14559ad1aafd4ab0e942779e9f58b1');
  const nodeSetup = workflow.indexOf('actions/setup-node@');
  assert.ok(pnpmSetup >= 0, 'missing pinned pnpm setup action');
  assert.ok(nodeSetup > pnpmSetup, 'pnpm must be installed before setup-node resolves the pnpm cache');
  assert.doesNotMatch(workflow, /corepack enable pnpm/);
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
  assert.match(override, /LATEXMK_GHCR_VERSION:-0\.2\.0-rc\.1/);
  assert.doesNotMatch(override, /billstark001/);
  assert.doesNotMatch(override, /LATEXMK_GHCR_(?:NAMESPACE|VERSION):\?/);
  assert.doesNotMatch(override, /:latest/);
  assert.match(envExample, /^COMPOSE_PATH_SEPARATOR=:\s*$/m);
  assert.match(envExample, /^COMPOSE_FILE=compose\.yaml:compose\.ghcr\.yaml\s*$/m);
});
