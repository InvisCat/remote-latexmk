import assert from 'node:assert/strict';
import test from 'node:test';

import { releaseTag } from '../scripts/publish-packages.mjs';

test('npm prereleases use next and stable releases use latest', () => {
  assert.equal(releaseTag('0.4.3-rc.1'), 'next');
  assert.equal(releaseTag('0.4.3'), 'latest');
});
