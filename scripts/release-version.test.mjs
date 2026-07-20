import assert from 'node:assert/strict';
import test from 'node:test';

import { compareVersions, parseVersion } from './release-version.mjs';

test('release versions follow semantic version precedence', () => {
  assert.ok(compareVersions('0.3.0-rc.5', '0.3.0-rc.2') > 0);
  assert.ok(compareVersions('0.3.0', '0.3.0-rc.9') > 0);
  assert.ok(compareVersions('0.4.0-rc.1', '0.3.9') > 0);
  assert.equal(compareVersions('1.2.3+build.2', '1.2.3+build.1'), 0);
  assert.ok(compareVersions('1.2.3-rc.10', '1.2.3-rc.2') > 0);
});

test('release versions use strict semantic version syntax', () => {
  assert.deepEqual(parseVersion('1.2.3-rc-alpha.1+build.4').prerelease, ['rc-alpha', '1']);
  assert.throws(() => parseVersion('01.2.3'), /invalid semantic version/);
  assert.throws(() => parseVersion('1.2.3-01'), /invalid semantic version/);
});
