import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

import { compareVersions, parseVersion } from './release-version.mjs';

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('release versions follow semantic version precedence', () => {
  assert.ok(compareVersions('0.4.3', '0.3.0-rc.2') > 0);
  assert.ok(compareVersions('0.3.0', '0.3.0-rc.9') > 0);
  assert.ok(compareVersions('0.4.3-rc.1', '0.3.9') > 0);
  assert.equal(compareVersions('1.2.3+build.2', '1.2.3+build.1'), 0);
  assert.ok(compareVersions('1.2.3-rc.10', '1.2.3-rc.2') > 0);
});

test('release versions use strict semantic version syntax', () => {
  assert.deepEqual(parseVersion('1.2.3-rc-alpha.1+build.4').prerelease, ['rc-alpha', '1']);
  assert.throws(() => parseVersion('01.2.3'), /invalid semantic version/);
  assert.throws(() => parseVersion('1.2.3-01'), /invalid semantic version/);
});

test('README changelog is complete and follows descending semantic version order', async () => {
  const readme = await readFile(path.join(repositoryRoot, 'README.md'), 'utf8');
  const manifest = JSON.parse(await readFile(path.join(repositoryRoot, 'package.json'), 'utf8'));
  const start = readme.indexOf('\n## Changelog');
  const end = readme.indexOf('\n## Roadmap', start);
  assert.ok(start >= 0 && end > start, 'README changelog boundaries are missing');

  const changelog = readme.slice(start, end);
  const headings = [...changelog.matchAll(/^### (.+)$/gm)].map((match) => match[1]);
  const versions = headings
    .filter((heading) => heading.startsWith('remote-latexmk '))
    .map((heading) => heading.slice('remote-latexmk '.length));

  assert.ok(versions.length > 0, 'README changelog has no remote-latexmk releases');
  assert.equal(versions[0], manifest.version, 'latest changelog entry does not match package.json');
  for (let index = 1; index < versions.length; index += 1) {
    assert.ok(
      compareVersions(versions[index - 1], versions[index]) > 0,
      `changelog is not descending: ${versions[index - 1]} before ${versions[index]}`,
    );
  }
  assert.equal(headings.length, versions.length + 1, 'changelog has an unexpected section heading');
  assert.equal(headings.at(-1), 'Upstream 0.1.0 Baseline', 'upstream baseline must be last');
});
