import assert from 'node:assert/strict';
import { mkdtemp, readFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { parseArgs, targets } from '../scripts/stage-packages.mjs';
import { copyBundledPlugin } from '../scripts/plugin-bundle.mjs';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('npm staging covers all native client targets', () => {
  assert.equal(targets.length, 6);
  assert.deepEqual(new Set(targets.map((target) => `${target.goos}/${target.goarch}`)), new Set([
    'darwin/arm64', 'darwin/amd64', 'linux/arm64', 'linux/amd64', 'windows/arm64', 'windows/amd64',
  ]));
});

test('npm staging requires an immutable semantic version', () => {
  assert.throws(() => parseArgs(['--version', 'main', '--artifacts', '.', '--out', 'dist']), /semantic version/);
  assert.equal(parseArgs(['--version', '1.2.3-rc.1', '--artifacts', '.', '--out', 'dist']).version, '1.2.3-rc.1');
});

test('bundled npm Skills use the non-conflicting launcher command', async () => {
  for (const name of ['remote-latex', 'remote-latex-maintenance', 'remote-latex-server', 'remote-latex-setup']) {
    const skill = await readFile(path.join(packageRoot, 'bundled-skills', name, 'SKILL.md'), 'utf8');
    assert.doesNotMatch(skill, /(?:^|[`\n])latexmk (?:auth|setup|doctor|meta|entries|files|compile|jobs|diagnostics|logs|artifacts|cache|remote|help)/m);
  }
  const compileSkill = await readFile(path.join(packageRoot, 'bundled-skills', 'remote-latex', 'SKILL.md'), 'utf8');
  assert.match(compileSkill, /npm launcher command named `remote-latexmk`/);
  assert.match(compileSkill, /remote-latexmk doctor/);
  assert.match(compileSkill, /remote-latexmk entries --json --project-root \./);
  assert.doesNotMatch(compileSkill, /remote-latexmk help/);

  const cliReference = await readFile(path.join(packageRoot, 'bundled-skills', 'remote-latex', 'references', 'cli.md'), 'utf8');
  assert.doesNotMatch(cliReference, /packages\/cli\/dist\/latexmk/);
  assert.match(cliReference, /remote-latexmk entries --json --project-root \./);
  assert.match(cliReference, /only authority for upload dependencies/);
  assert.doesNotMatch(cliReference, /remote-latexmk help/);
});

test('bundled Codex Plugin pins its manifest, MCP launcher, and Skills to the npm version', async () => {
  const repositoryRoot = path.resolve(packageRoot, '../..');
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-plugin-bundle-'));
  const destination = path.join(temp, 'remote-latexmk');
  const version = '9.8.7-rc.2';
  await copyBundledPlugin(path.join(repositoryRoot, 'plugins', 'remote-latexmk'), destination, version);
  const manifest = JSON.parse(await readFile(path.join(destination, '.codex-plugin', 'plugin.json'), 'utf8'));
  const mcp = JSON.parse(await readFile(path.join(destination, '.mcp.json'), 'utf8'));
  const skill = await readFile(path.join(destination, 'skills', 'remote-latex', 'SKILL.md'), 'utf8');
  assert.equal(manifest.version, version);
  assert.ok(mcp.mcpServers['remote-latexmk'].args.includes(`remote-latexmk@${version}`));
  assert.match(skill, new RegExp(`remote-latexmk@${version.replaceAll('.', '\\.')}`));
  assert.doesNotMatch(skill, /remote-latexmk@0\.3\.0-rc\.2/);
});
