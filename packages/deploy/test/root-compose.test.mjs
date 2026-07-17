import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');

test('root Compose keeps the compiler server off egress networks', async () => {
  const compose = await readFile(path.join(repositoryRoot, 'compose.yaml'), 'utf8');
  const server = compose.match(/\n  server:\n([\s\S]*?)\n  gateway:\n/)?.[1];
  const gateway = compose.match(/\n  gateway:\n([\s\S]*?)\n  client:\n/)?.[1];
  assert.ok(server, 'server service section was not found');
  assert.ok(gateway, 'gateway service section was not found');
  assert.match(server, /networks:\n      - latexmk-backend/);
  assert.doesNotMatch(server, /client-egress|\n      - edge|ports:/);
  assert.match(gateway, /Caddyfile\.gateway/);
  assert.match(gateway, /- latexmk-backend/);
  assert.match(gateway, /- edge/);
  assert.doesNotMatch(gateway, /LATEXMK_API_TOKEN|latexmk-state/);
  assert.match(compose, /latexmk-backend:\n    internal: true/);
  assert.match(compose, /x-client-service:[\s\S]*?- client-egress/);
});

test('HTTP gateway has a fixed internal upstream and no admin endpoint', async () => {
  const caddyfile = await readFile(path.join(repositoryRoot, 'packages/deploy/templates/Caddyfile.gateway'), 'utf8');
  assert.match(caddyfile, /admin off/);
  assert.match(caddyfile, /auto_https off/);
  assert.match(caddyfile, /reverse_proxy server:8080/);
});
