import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { parseArgs, targets } from '../scripts/stage-packages.mjs';

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
    assert.doesNotMatch(skill, /(?:^|[`\n])latexmk (?:auth|setup|doctor|meta|files|compile|jobs|diagnostics|logs|artifacts|cache|remote|help)/m);
  }
  const compileSkill = await readFile(path.join(packageRoot, 'bundled-skills', 'remote-latex', 'SKILL.md'), 'utf8');
  assert.match(compileSkill, /npm launcher command named `remote-latexmk`/);
  assert.match(compileSkill, /remote-latexmk doctor/);
});
