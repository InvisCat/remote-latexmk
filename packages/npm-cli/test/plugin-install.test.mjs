import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { chmod, cp, mkdir, mkdtemp, readFile, readdir, stat, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';
import { parse } from 'jsonc-parser';
import {
  codexPluginURL,
  installCodexPlugin,
  parsePluginInstallArgs,
  renderConnectionSetupCallout,
} from '../lib/plugin-install.js';

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const packageJSON = JSON.parse(await readFile(path.join(packageRoot, 'package.json'), 'utf8'));

test('Codex Plugin installer help names the installed rlatexmk command', async () => {
  const { stdout } = await execFileAsync(process.execPath, [
    path.join(packageRoot, 'bin', 'rlatexmk.js'), 'plugin', 'install', 'codex', '--help',
  ]);
  assert.match(stdout, /^Usage: rlatexmk plugin install codex/m);
  assert.doesNotMatch(stdout, /^Usage: remote-latexmk plugin install codex/m);
});

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

test('connection setup callout is boxed, width-aware, and optionally highlighted', () => {
  const plain = renderConnectionSetupCallout('1.2.3', { columns: 58 });
  const normalized = plain.replace(/[│\n]/g, ' ').replace(/\s+/g, ' ');
  assert.match(plain, /┌─+┐/);
  assert.match(plain, /CONNECTION SETUP/);
  assert.match(plain, /remote-latexmk@1\.2\.3/);
  assert.match(normalized, /auth login --server SERVER_HOST/);
  assert.doesNotMatch(plain, /\u001b\[/);
  assert.ok(plain.split('\n').every((line) => line.length <= 58));

  const styled = renderConnectionSetupCallout('1.2.3', { columns: 58, style: true });
  assert.match(styled, /\u001b\[7m/);
  assert.match(styled, /\u001b\[0m/);

  const narrow = renderConnectionSetupCallout('1.2.3', { columns: 32 });
  assert.ok(narrow.split('\n').every((line) => line.length <= 32));
  assert.ok(narrow.split('\n').filter((line) => line.includes('remote-latexmk@') || line.includes('auth login')).every((line) => line.startsWith('│ ') && line.endsWith(' │')));
});

test('Plugin installation highlights setup only on a capable terminal', async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-callout-'));
  const styledLogs = [];
  await installCodexPlugin(['--dry-run'], {
    home,
    log: (line) => styledLogs.push(line),
    output: { isTTY: true, columns: 64 },
    env: { TERM: 'xterm-256color' },
  });
  const styledOutput = styledLogs.join('\n');
  const normalizedStyled = styledOutput
    .replace(/\u001b\[[0-9;]*m/g, '')
    .replace(/[│\n]/g, ' ')
    .replace(/\s+/g, ' ');
  assert.match(styledOutput, /\u001b\[7m npx .*remote-latexmk@0\.4\.3/);
  assert.match(styledOutput, /\u001b\[7m --server SERVER_HOST/);
  assert.match(normalizedStyled, /remote-latexmk@0\.4\.3 auth login --server SERVER_HOST/);

  const plainLogs = [];
  await installCodexPlugin(['--dry-run'], {
    home,
    log: (line) => plainLogs.push(line),
    output: { isTTY: true, columns: 64 },
    env: { TERM: 'xterm-256color', NO_COLOR: '1' },
  });
  assert.doesNotMatch(plainLogs.join('\n'), /\u001b\[/);
});

test('Codex Plugin installer preserves a personal marketplace and is idempotent', async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-codex-home-'));
  const marketplacePath = path.join(home, '.agents', 'plugins', 'marketplace.json');
  const cacheSentinel = path.join(home, '.codex', 'plugins', 'cache', 'personal', 'remote-latexmk', 'old-version', 'sentinel');
  await mkdir(path.dirname(marketplacePath), { recursive: true });
  await mkdir(path.dirname(cacheSentinel), { recursive: true });
  await writeFile(cacheSentinel, 'keep private cache untouched\n');
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
  assert.equal(first.previousVersion, null);
  assert.equal(first.targetVersion, packageJSON.version);
  assert.equal(first.marketplaceAction, 'update');
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
  assert.equal(await readFile(cacheSentinel, 'utf8'), 'keep private cache untouched\n');
  assert.ok(logs.includes(`Plugin source: install ${packageJSON.version}`));
  assert.ok(logs.includes('Personal marketplace: update'));
  assert.ok(logs.some((line) => line.includes("does not edit Codex's private Plugin cache")));
  assert.ok(logs.some((line) => line.includes('select Install or Update')));
  assert.ok(logs.some((line) => line.includes('start a new task')));
  assert.ok(logs.some((line) => line.includes('Existing remote-latexmk login: preserved if present')));
  assert.match(
    logs.join('\n').replace(/[│\n]/g, ' ').replace(/\s+/g, ' '),
    new RegExp(`remote-latexmk@${packageJSON.version.replaceAll('.', '\\.')} auth login --server SERVER_HOST`),
  );

  const secondLogs = [];
  const second = await installCodexPlugin(['--no-open'], { home, log: (line) => secondLogs.push(line) });
  assert.equal(second.pluginAction, 'unchanged');
  assert.equal(second.previousVersion, packageJSON.version);
  assert.equal(second.targetVersion, packageJSON.version);
  assert.equal(second.marketplaceAction, 'unchanged');
  assert.equal(second.marketplaceChanged, false);
  assert.equal((await readdir(path.join(home, 'plugins'))).filter((name) => name.startsWith('remote-latexmk.backup-')).length, 0);
  assert.ok(secondLogs.includes(`Plugin source: unchanged ${packageJSON.version}`));
  assert.ok(secondLogs.includes('Personal marketplace: unchanged'));
  assert.equal(await readFile(cacheSentinel, 'utf8'), 'keep private cache untouched\n');
});

test('Codex Plugin installer reports a managed source update with both versions', async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-codex-update-'));
  const oldSource = path.join(home, 'old-plugin-source');
  await cp(path.join(packageRoot, 'bundled-plugin'), oldSource, { recursive: true });
  const oldMcpPath = path.join(oldSource, '.mcp.json');
  await writeFile(oldMcpPath, `${await readFile(oldMcpPath, 'utf8')}\n`);

  await installCodexPlugin(['--no-open'], { home, source: oldSource, log: () => {} });
  const destination = path.join(home, 'plugins', 'remote-latexmk');
  const markerPath = path.join(destination, '.remote-latexmk-managed.json');
  const marker = JSON.parse(await readFile(markerPath, 'utf8'));
  marker.packageVersion = '0.2.0';
  await writeFile(markerPath, `${JSON.stringify(marker, null, 2)}\n`);

  const logs = [];
  const updated = await installCodexPlugin(['--no-open'], { home, log: (line) => logs.push(line) });
  assert.equal(updated.pluginAction, 'update');
  assert.equal(updated.previousVersion, '0.2.0');
  assert.equal(updated.targetVersion, packageJSON.version);
  assert.equal(updated.marketplaceAction, 'unchanged');
  assert.ok(logs.includes(`Plugin source: update 0.2.0 -> ${packageJSON.version}`));
  assert.ok(logs.includes('Personal marketplace: unchanged'));
  assert.equal((await readdir(path.join(home, 'plugins'))).filter((name) => name.startsWith('remote-latexmk.backup-')).length, 1);
  assert.equal(
    JSON.parse(await readFile(path.join(destination, '.codex-plugin', 'plugin.json'), 'utf8')).version,
    packageJSON.version,
  );
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
  const logs = [];
  const forced = await installCodexPlugin(['--force', '--no-open'], { home, log: (line) => logs.push(line) });
  assert.equal(forced.pluginAction, 'replace');
  assert.equal(forced.previousVersion, packageJSON.version);
  assert.equal(forced.targetVersion, packageJSON.version);
  assert.ok(logs.includes(`Plugin source: replace conflicting content with ${packageJSON.version}`));
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
    path.join(packageRoot, 'bin', 'rlatexmk.js'),
    'plugin', 'install', 'codex', '--dry-run', '--no-open',
  ], { env: { ...process.env, HOME: home } });
  assert.ok(stdout.includes(`Plugin source plan: install ${packageJSON.version}\n`));
  assert.match(stdout, /Plugin source path: .*plugins\/remote-latexmk/);
  assert.match(stdout, /Personal marketplace plan: create/);
  assert.match(stdout, /Codex Plugin page: codex:\/\/plugins\/remote-latexmk/);
  assert.match(stdout, /does not edit Codex's private Plugin cache/);
  assert.match(stdout, /Next after applying: select Install or Update/);
  assert.match(stdout, /Existing remote-latexmk login: preserved if present/);
  assert.match(stdout.replace(/[│\n]/g, ' ').replace(/\s+/g, ' '), /remote-latexmk@[^\s]+ auth login --server SERVER_HOST/);

  const cliHome = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-codex-cli-'));
  const installed = await execFileAsync(process.execPath, [
    path.join(packageRoot, 'bin', 'rlatexmk.js'),
    'plugin', 'install', 'codex', '--no-open',
  ], { env: { ...process.env, HOME: cliHome } });
  assert.ok(installed.stdout.includes(`Plugin source: install ${packageJSON.version}\n`));
  assert.match(installed.stdout, /Personal marketplace: create/);
  assert.match(installed.stdout, /select Install or Update/);
  assert.match(installed.stdout, /start a new task/);
  assert.match(installed.stdout, /Existing remote-latexmk login: preserved if present/);
  assert.match(installed.stdout.replace(/[│\n]/g, ' ').replace(/\s+/g, ' '), /remote-latexmk@[^\s]+ auth login --server SERVER_HOST/);
  assert.equal(
    JSON.parse(await readFile(path.join(cliHome, 'plugins', 'remote-latexmk', '.codex-plugin', 'plugin.json'), 'utf8')).name,
    'remote-latexmk',
  );
  assert.equal(
    parse(await readFile(path.join(cliHome, '.agents', 'plugins', 'marketplace.json'), 'utf8')).plugins[0].name,
    'remote-latexmk',
  );
});
