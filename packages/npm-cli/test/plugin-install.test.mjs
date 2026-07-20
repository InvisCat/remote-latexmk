import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { chmod, mkdir, mkdtemp, readFile, readdir, stat, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';
import { parse } from 'jsonc-parser';
import { codexPluginURL, installCodexPlugin, parsePluginInstallArgs } from '../lib/plugin-install.js';

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const packageJSON = JSON.parse(await readFile(path.join(packageRoot, 'package.json'), 'utf8'));

test('Codex Plugin installer parses bounded non-secret options', () => {
  assert.deepEqual(parsePluginInstallArgs(['--dry-run', '--force', '--no-open']), {
    dryRun: true,
    force: true,
    open: false,
    help: false,
  });
  assert.throws(() => parsePluginInstallArgs(['--token', 'secret']), /unknown option/);
  assert.throws(() => parsePluginInstallArgs(['--home', '/tmp/home']), /unknown option/);
});

test('Codex Plugin installer preserves a personal marketplace and is idempotent', async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-codex-home-'));
  const marketplacePath = path.join(home, '.agents', 'plugins', 'marketplace.json');
  await mkdir(path.dirname(marketplacePath), { recursive: true });
  await writeFile(marketplacePath, `{
  // keep this personal entry
  "name": "personal",
  "interface": { "displayName": "My Plugins" },
  "plugins": [
    {
      "name": "example",
      "source": { "source": "local", "path": "./plugins/example" },
      "policy": { "installation": "AVAILABLE", "authentication": "ON_USE" },
      "category": "Development"
    }
  ]
}
`);
  let opened = '';
  const logs = [];
  const first = await installCodexPlugin([], {
    home,
    log: (line) => logs.push(line),
    openURL: async (url) => { opened = url; return true; },
  });

  const destination = path.join(home, 'plugins', 'remote-latexmk');
  assert.equal(first.destination, destination);
  assert.equal(first.pluginAction, 'install');
  assert.equal(opened, codexPluginURL(marketplacePath));
  assert.match(opened, /^codex:\/\/plugins\/remote-latexmk\?marketplacePath=/);
  const manifest = JSON.parse(await readFile(path.join(destination, '.codex-plugin', 'plugin.json'), 'utf8'));
  assert.equal(manifest.version, packageJSON.version);
  const mcp = JSON.parse(await readFile(path.join(destination, '.mcp.json'), 'utf8'));
  assert.ok(mcp.mcpServers['remote-latexmk'].args.includes(`remote-latexmk@${packageJSON.version}`));
  const marker = JSON.parse(await readFile(path.join(destination, '.remote-latexmk-managed.json'), 'utf8'));
  assert.equal(marker.managedBy, 'remote-latexmk');
  assert.equal(marker.packageVersion, packageJSON.version);

  const marketplaceText = await readFile(marketplacePath, 'utf8');
  assert.match(marketplaceText, /keep this personal entry/);
  const marketplace = parse(marketplaceText);
  assert.equal(marketplace.interface.displayName, 'My Plugins');
  assert.deepEqual(marketplace.plugins.map((entry) => entry.name), ['example', 'remote-latexmk']);
  assert.deepEqual(marketplace.plugins[1].source, { source: 'local', path: './plugins/remote-latexmk' });
  if (process.platform !== 'win32') {
    assert.equal((await stat(marketplacePath)).mode & 0o777, 0o600);
    assert.equal((await stat(destination)).mode & 0o777, 0o700);
  }

  const second = await installCodexPlugin(['--no-open'], { home, log: () => {} });
  assert.equal(second.pluginAction, 'unchanged');
  assert.equal(second.marketplaceChanged, false);
  assert.equal((await readdir(path.join(home, 'plugins'))).filter((name) => name.startsWith('remote-latexmk.backup-')).length, 0);
  assert.ok(logs.some((line) => line.includes('Restart Codex')));
});

test('Codex Plugin installer plans all conflicts before writing', async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-codex-conflict-'));
  const marketplacePath = path.join(home, '.agents', 'plugins', 'marketplace.json');
  await mkdir(path.dirname(marketplacePath), { recursive: true });
  await writeFile(marketplacePath, `{
  "name": "personal",
  "interface": { "displayName": "Personal" },
  "plugins": [{
    "name": "remote-latexmk",
    "source": { "source": "local", "path": "./plugins/something-else" },
    "policy": { "installation": "AVAILABLE", "authentication": "ON_INSTALL" },
    "category": "Development"
  }]
}
`);

  await assert.rejects(
    installCodexPlugin(['--no-open'], { home, log: () => {} }),
    /conflicting remote-latexmk entry/,
  );
  await assert.rejects(stat(path.join(home, 'plugins', 'remote-latexmk')), /ENOENT/);
});

test('Codex Plugin installer protects edits and backs up forced replacements', async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-codex-force-'));
  await installCodexPlugin(['--no-open'], { home, log: () => {} });
  const destination = path.join(home, 'plugins', 'remote-latexmk');
  const manifestPath = path.join(destination, '.codex-plugin', 'plugin.json');
  await writeFile(manifestPath, `${await readFile(manifestPath, 'utf8')}\n`);

  await assert.rejects(
    installCodexPlugin(['--no-open'], { home, log: () => {} }),
    /unmanaged changes/,
  );
  const forced = await installCodexPlugin(['--force', '--no-open'], { home, log: () => {} });
  assert.equal(forced.pluginAction, 'replace');
  const entries = await readdir(path.join(home, 'plugins'));
  assert.equal(entries.filter((name) => name.startsWith('remote-latexmk.backup-')).length, 1);
});

test('Codex Plugin installer dry-run and CLI dispatch do not need Codex CLI', async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-codex-dry-run-'));
  let opened = false;
  await installCodexPlugin(['--dry-run'], {
    home,
    log: () => {},
    openURL: async () => { opened = true; return true; },
  });
  assert.equal(opened, false);
  await assert.rejects(stat(path.join(home, '.agents')), /ENOENT/);
  await assert.rejects(stat(path.join(home, 'plugins')), /ENOENT/);

  const { stdout } = await execFileAsync(process.execPath, [
    path.join(packageRoot, 'bin', 'remote-latexmk.js'),
    'plugin', 'install', 'codex', '--dry-run', '--no-open',
  ], { env: { ...process.env, HOME: home } });
  assert.match(stdout, /would use.*plugins\/remote-latexmk/);
  assert.match(stdout, /Codex Plugin page: codex:\/\/plugins\/remote-latexmk/);

  const cliHome = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-codex-cli-'));
  const installed = await execFileAsync(process.execPath, [
    path.join(packageRoot, 'bin', 'remote-latexmk.js'),
    'plugin', 'install', 'codex', '--no-open',
  ], { env: { ...process.env, HOME: cliHome } });
  assert.match(installed.stdout, /Restart Codex/);
  assert.equal(
    JSON.parse(await readFile(path.join(cliHome, 'plugins', 'remote-latexmk', '.codex-plugin', 'plugin.json'), 'utf8')).name,
    'remote-latexmk',
  );
  assert.equal(
    parse(await readFile(path.join(cliHome, '.agents', 'plugins', 'marketplace.json'), 'utf8')).plugins[0].name,
    'remote-latexmk',
  );
});
