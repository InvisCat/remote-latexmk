import assert from 'node:assert/strict';
import test from 'node:test';
import { platformPackageName, resolveNativeBinary } from '../lib/platform.js';

test('maps every released client target to a scoped optional package', () => {
  assert.equal(platformPackageName('darwin', 'arm64'), '@rlatexmk/rlatexmk-darwin-arm64');
  assert.equal(platformPackageName('darwin', 'x64'), '@rlatexmk/rlatexmk-darwin-x64');
  assert.equal(platformPackageName('linux', 'arm64'), '@rlatexmk/rlatexmk-linux-arm64');
  assert.equal(platformPackageName('linux', 'x64'), '@rlatexmk/rlatexmk-linux-x64');
  assert.equal(platformPackageName('win32', 'arm64'), '@rlatexmk/rlatexmk-win32-arm64');
  assert.equal(platformPackageName('win32', 'x64'), '@rlatexmk/rlatexmk-win32-x64');
  assert.equal(platformPackageName('freebsd', 'x64'), null);
});

test('missing optional package produces an actionable error', () => {
  assert.throws(() => resolveNativeBinary('freebsd', 'x64'), /no native client is published/);
});
