import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const pluginRoot = path.join(root, 'plugins/remote-latexmk');

const codexManifest = await readJSON(path.join(pluginRoot, '.codex-plugin/plugin.json'));
const claudeManifest = await readJSON(path.join(pluginRoot, '.claude-plugin/plugin.json'));
const codexMarketplace = await readJSON(path.join(root, '.agents/plugins/marketplace.json'));
const claudeMarketplace = await readJSON(path.join(root, '.claude-plugin/marketplace.json'));
const mcp = await readJSON(path.join(pluginRoot, '.mcp.json'));

assert.equal(codexManifest.name, 'remote-latexmk');
assert.equal(claudeManifest.name, codexManifest.name);
assert.equal(claudeManifest.version, codexManifest.version);
assert.equal(claudeMarketplace.metadata.version, codexManifest.version);
assert.equal(claudeMarketplace.plugins[0].version, codexManifest.version);
assert.equal(codexMarketplace.name, 'remote-latexmk');
assert.equal(codexMarketplace.plugins[0].name, codexManifest.name);
assert.equal(codexMarketplace.plugins[0].source.path, './plugins/remote-latexmk');

const mcpConfig = mcp.mcpServers?.['remote-latexmk'];
assert.equal(mcpConfig?.command, 'npx');
assert.ok(mcpConfig.args.includes(`remote-latexmk@${codexManifest.version}`));
assert.ok(mcpConfig.args.includes('--root-from-client'));
assert.ok(!mcpConfig.args.includes('--project-root'));
assert.deepEqual(
  mcpConfig.args.slice(mcpConfig.args.indexOf('--fallback-workspace-root')),
  ['--fallback-workspace-root', '.'],
);
assert.equal(mcpConfig.env, undefined);

for (const file of [
  'skills/remote-latex/SKILL.md',
  'skills/remote-latex-maintenance/SKILL.md',
  'skills/remote-latex-server/SKILL.md',
  'skills/remote-latex-setup/SKILL.md',
]) {
  const content = await readFile(path.join(pluginRoot, file), 'utf8');
  assert.ok(!content.includes('[TODO:'), `${file} contains a TODO placeholder`);
}

async function readJSON(file) {
  return JSON.parse(await readFile(file, 'utf8'));
}
