import assert from 'node:assert/strict';
import test from 'node:test';
import { platformPackageName, resolveNativeBinary } from '../lib/platform.js';

test('maps every released client target to a scoped optional package', () => {
  assert.equal(platformPackageName('darwin', 'arm64'), '@inviscat/remote-latexmk-darwin-arm64');
  assert.equal(platformPackageName('darwin', 'x64'), '@inviscat/remote-latexmk-darwin-x64');
  assert.equal(platformPackageName('linux', 'arm64'), '@inviscat/remote-latexmk-linux-arm64');
  assert.equal(platformPackageName('linux', 'x64'), '@inviscat/remote-latexmk-linux-x64');
  assert.equal(platformPackageName('win32', 'arm64'), '@inviscat/remote-latexmk-win32-arm64');
  assert.equal(platformPackageName('win32', 'x64'), '@inviscat/remote-latexmk-win32-x64');
  assert.equal(platformPackageName('freebsd', 'x64'), null);
});

test('missing optional package produces an actionable error', () => {
  assert.throws(() => resolveNativeBinary('freebsd', 'x64'), /no native client is published/);
});
