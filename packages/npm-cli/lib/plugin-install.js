import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, cp, lstat, mkdir, readFile, readdir, rename, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';
import { applyEdits, modify, parse } from 'jsonc-parser';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const packageJSON = JSON.parse(await readFile(path.join(packageRoot, 'package.json'), 'utf8'));
const pluginName = 'remote-latexmk';
const markerName = '.remote-latexmk-managed.json';
const formattingOptions = { insertSpaces: true, tabSize: 2, eol: '\n' };

function usage() {
  return `Usage: rlatexmk plugin install codex [options]

Install the Remote LaTeX Plugin for the Codex desktop app.

Options:
  --dry-run  Print the complete plan without changes
  --force    Back up and replace a conflicting unmanaged Plugin or entry
  --no-open  Do not open the Codex Plugin page
  -h, --help Show this help`;
}

export function parsePluginInstallArgs(args) {
  const options = { dryRun: false, force: false, open: true, help: false };
  for (const arg of args) {
    if (arg === '--dry-run') options.dryRun = true;
    else if (arg === '--force') options.force = true;
    else if (arg === '--no-open') options.open = false;
    else if (arg === '-h' || arg === '--help') options.help = true;
    else throw new Error(`unknown option: ${arg}`);
  }
  return options;
}

async function pathState(target) {
  try {
    return await lstat(target);
  } catch (error) {
    if (error.code === 'ENOENT') return null;
    throw error;
  }
}

async function directoryDigest(root, ignoreMarker = false) {
  const hash = createHash('sha256');
  async function visit(directory, prefix = '') {
    const entries = await readdir(directory, { withFileTypes: true });
    entries.sort((left, right) => left.name.localeCompare(right.name));
    for (const entry of entries) {
      if (ignoreMarker && prefix === '' && entry.name === markerName) continue;
      const relative = path.join(prefix, entry.name);
      const absolute = path.join(directory, entry.name);
      if (entry.isSymbolicLink()) throw new Error(`refusing symlink in Plugin: ${relative}`);
      if (entry.isDirectory()) {
        hash.update(`directory:${relative}\0`);
        await visit(absolute, relative);
      } else if (entry.isFile()) {
        hash.update(`file:${relative}\0`);
        hash.update(await readFile(absolute));
        hash.update('\0');
      } else {
        throw new Error(`refusing non-file in Plugin: ${relative}`);
      }
    }
  }
  await visit(root);
  return hash.digest('hex');
}

async function readMarker(destination) {
  try {
    const marker = JSON.parse(await readFile(path.join(destination, markerName), 'utf8'));
    if (marker?.managedBy !== 'remote-latexmk' || typeof marker.sourceDigest !== 'string') return null;
    return marker;
  } catch (error) {
    if (error.code === 'ENOENT' || error instanceof SyntaxError) return null;
    throw error;
  }
}

function backupSuffix() {
  return `${new Date().toISOString().replace(/[:.]/g, '-')}-${process.pid}`;
}

function managedMarker(sourceDigest) {
  return `${JSON.stringify({
    managedBy: 'remote-latexmk',
    packageVersion: packageJSON.version,
    sourceDigest,
  }, null, 2)}\n`;
}

async function planPlugin(destination, sourceDigest, force) {
  const info = await pathState(destination);
  if (!info) {
    return {
      action: 'install',
      markerOnly: false,
      previousVersion: null,
      targetVersion: packageJSON.version,
    };
  }
  if (info.isSymbolicLink()) throw new Error(`refusing symlink at Plugin destination: ${destination}`);
  if (!info.isDirectory()) throw new Error(`Plugin destination is not a directory: ${destination}`);
  const marker = await readMarker(destination);
  const previousVersion = typeof marker?.packageVersion === 'string' && marker.packageVersion.trim() !== ''
    ? marker.packageVersion
    : null;
  const destinationDigest = await directoryDigest(destination, true);
  if (destinationDigest === sourceDigest) {
    return {
      action: marker?.packageVersion === packageJSON.version && marker?.sourceDigest === sourceDigest ? 'unchanged' : 'adopt',
      markerOnly: true,
      previousVersion,
      targetVersion: packageJSON.version,
    };
  }
  if (marker && destinationDigest === marker.sourceDigest) {
    return {
      action: 'update',
      markerOnly: false,
      previousVersion,
      targetVersion: packageJSON.version,
    };
  }
  if (force) {
    return {
      action: 'replace',
      markerOnly: false,
      previousVersion,
      targetVersion: packageJSON.version,
    };
  }
  throw new Error(`Plugin destination has unmanaged changes: ${destination}; inspect it or pass --force`);
}

function describePluginPlan(plan) {
  const previous = plan.previousVersion;
  const target = plan.targetVersion;
  switch (plan.action) {
    case 'install':
      return `install ${target}`;
    case 'update':
      return previous && previous !== target
        ? `update ${previous} -> ${target}`
        : `update ${target} content`;
    case 'unchanged':
      return `unchanged ${target}`;
    case 'adopt':
      return `adopt existing ${target} content`;
    case 'replace':
      return `replace conflicting content with ${target}`;
    default:
      throw new Error(`unknown Plugin plan action: ${plan.action}`);
  }
}

async function installPluginDirectory(source, destination, sourceDigest, plan) {
  if (plan.action === 'unchanged') return null;
  if (plan.markerOnly) {
    await writeFile(path.join(destination, markerName), managedMarker(sourceDigest), { mode: 0o600 });
    return null;
  }

  const parent = path.dirname(destination);
  const temporary = path.join(parent, `.${pluginName}.install-${process.pid}-${Date.now()}`);
  const backup = path.join(parent, `${pluginName}.backup-${backupSuffix()}`);
  await mkdir(parent, { recursive: true, mode: 0o700 });
  await cp(source, temporary, { recursive: true, errorOnExist: true });
  await chmod(temporary, 0o700);
  await writeFile(path.join(temporary, markerName), managedMarker(sourceDigest), { mode: 0o600 });

  const existing = await pathState(destination);
  try {
    if (existing) await rename(destination, backup);
    await rename(temporary, destination);
  } catch (error) {
    await rm(temporary, { recursive: true, force: true });
    if (existing && !(await pathState(destination)) && await pathState(backup)) {
      await rename(backup, destination);
    }
    throw error;
  }
  return existing ? backup : null;
}

function marketplaceEntry() {
  return {
    name: pluginName,
    source: { source: 'local', path: `./plugins/${pluginName}` },
    policy: { installation: 'AVAILABLE', authentication: 'ON_INSTALL' },
    category: 'Development',
  };
}

function parseMarketplace(text, target) {
  const errors = [];
  const marketplace = parse(text, errors, { allowTrailingComma: true, disallowComments: false });
  if (errors.length > 0 || !marketplace || typeof marketplace !== 'object' || Array.isArray(marketplace)) {
    throw new Error(`cannot safely parse ${target}; no changes were made`);
  }
  return marketplace;
}

function planMarketplace(text, target, force) {
  let next = text;
  let marketplace = parseMarketplace(next, target);
  if (marketplace.name === undefined) {
    next = applyEdits(next, modify(next, ['name'], 'personal', { formattingOptions }));
  } else if (typeof marketplace.name !== 'string' || marketplace.name.trim() === '') {
    throw new Error(`${target} has an invalid marketplace name`);
  }
  marketplace = parseMarketplace(next, target);
  if (marketplace.interface === undefined) {
    next = applyEdits(next, modify(next, ['interface'], { displayName: 'Personal' }, { formattingOptions }));
  } else if (!marketplace.interface || typeof marketplace.interface !== 'object' || Array.isArray(marketplace.interface)) {
    throw new Error(`${target} has an invalid interface object`);
  } else if (marketplace.interface.displayName === undefined) {
    next = applyEdits(next, modify(next, ['interface', 'displayName'], 'Personal', { formattingOptions }));
  }
  marketplace = parseMarketplace(next, target);
  if (marketplace.plugins === undefined) {
    next = applyEdits(next, modify(next, ['plugins'], [], { formattingOptions }));
  } else if (!Array.isArray(marketplace.plugins)) {
    throw new Error(`${target} has a non-array plugins field`);
  }

  marketplace = parseMarketplace(next, target);
  const matches = marketplace.plugins
    .map((entry, index) => ({ entry, index }))
    .filter(({ entry }) => entry?.name === pluginName);
  if (matches.length > 1) throw new Error(`${target} has duplicate ${pluginName} entries`);
  if (matches.length === 0) {
    next = applyEdits(next, modify(next, ['plugins', marketplace.plugins.length], marketplaceEntry(), { formattingOptions }));
  } else {
    const { entry, index } = matches[0];
    const source = entry?.source;
    const expectedPath = `./plugins/${pluginName}`;
    if (source?.source !== 'local' || source?.path !== expectedPath) {
      if (!force) throw new Error(`${target} has a conflicting ${pluginName} entry; inspect it or pass --force`);
      next = applyEdits(next, modify(next, ['plugins', index], marketplaceEntry(), { formattingOptions }));
    }
  }
  return { text: next, changed: next !== text };
}

async function writeMarketplace(target, original, next) {
  if (original === next) return null;
  const directory = path.dirname(target);
  const temporary = `${target}.new-${process.pid}`;
  const existing = await pathState(target);
  const backup = `${target}.backup-${backupSuffix()}`;
  await mkdir(directory, { recursive: true, mode: 0o700 });
  await writeFile(temporary, next, { mode: 0o600 });
  try {
    if (existing) await rename(target, backup);
    await rename(temporary, target);
  } catch (error) {
    await rm(temporary, { force: true });
    if (existing && !(await pathState(target)) && await pathState(backup)) {
      await rename(backup, target);
    }
    throw error;
  }
  return existing ? backup : null;
}

export function codexPluginURL(marketplacePath) {
  return `codex://plugins/${pluginName}?marketplacePath=${encodeURIComponent(marketplacePath)}`;
}

function openCodex(url) {
  let result;
  if (process.platform === 'darwin') {
    result = spawnSync('open', [url], { stdio: 'ignore', windowsHide: true });
  } else if (process.platform === 'win32') {
    result = spawnSync('rundll32.exe', ['url.dll,FileProtocolHandler', url], { stdio: 'ignore', windowsHide: true });
  } else {
    return false;
  }
  return !result.error && result.status === 0;
}

export async function installCodexPlugin(args, dependencies = {}) {
  const options = parsePluginInstallArgs(args);
  const log = dependencies.log ?? console.log;
  if (options.help) {
    log(usage());
    return { help: true };
  }

  const home = path.resolve(dependencies.home ?? os.homedir());
  const source = path.resolve(dependencies.source ?? path.join(packageRoot, 'bundled-plugin'));
  const destination = path.join(home, 'plugins', pluginName);
  const marketplacePath = path.join(home, '.agents', 'plugins', 'marketplace.json');
  const sourceInfo = await pathState(source);
  if (!sourceInfo?.isDirectory()) throw new Error(`bundled Codex Plugin is missing: ${source}`);
  const sourceDigest = await directoryDigest(source);

  const marketplaceInfo = await pathState(marketplacePath);
  if (marketplaceInfo?.isSymbolicLink()) throw new Error(`refusing symlink at marketplace path: ${marketplacePath}`);
  if (marketplaceInfo && !marketplaceInfo.isFile()) throw new Error(`marketplace path is not a file: ${marketplacePath}`);
  const marketplaceOriginal = marketplaceInfo ? await readFile(marketplacePath, 'utf8') : '{\n}\n';
  const marketplacePlan = planMarketplace(marketplaceOriginal, marketplacePath, options.force);
  const pluginPlan = await planPlugin(destination, sourceDigest, options.force);
  const marketplaceAction = !marketplaceInfo ? 'create' : (marketplacePlan.changed ? 'update' : 'unchanged');

  log(`${options.dryRun ? 'Plugin source plan' : 'Plugin source'}: ${describePluginPlan(pluginPlan)}`);
  log(`Plugin source path: ${destination}`);
  log(`${options.dryRun ? 'Personal marketplace plan' : 'Personal marketplace'}: ${marketplaceAction}`);
  log(`Personal marketplace path: ${marketplacePath}`);
  if (!options.dryRun) {
    const pluginBackup = await installPluginDirectory(source, destination, sourceDigest, pluginPlan);
    const marketplaceBackup = await writeMarketplace(marketplacePath, marketplaceOriginal, marketplacePlan.text);
    if (pluginBackup) log(`backed up previous Plugin: ${pluginBackup}`);
    if (marketplaceBackup) log(`backed up marketplace: ${marketplaceBackup}`);
  }

  const url = codexPluginURL(marketplacePath);
  log(`Codex Plugin page: ${url}`);
  if (!options.dryRun && options.open) {
    const opened = await (dependencies.openURL ?? openCodex)(url);
    if (!opened) log('Could not open Codex automatically. Open the Plugin page above.');
  }
  log("Codex installed copy: separate from this marketplace source; this command does not edit Codex's private Plugin cache.");
  if (options.dryRun) {
    log('Next after applying: select Install or Update on the Plugin page, restart Codex if it is running, then start a new task.');
  } else {
    log('Next: select Install or Update on the Plugin page, restart Codex if it is running, then start a new task from your paper directory.');
  }
  log('Existing remote-latexmk login: preserved if present; the server URL and API token are unchanged.');
  log(`Connection setup (first install or server change): npx --yes --ignore-scripts remote-latexmk@${packageJSON.version} auth login --server SERVER_HOST`);
  log('The login command checks the service, then reads the remote-latexmk API token at a hidden prompt.');
  return {
    destination,
    marketplacePath,
    url,
    pluginAction: pluginPlan.action,
    previousVersion: pluginPlan.previousVersion,
    targetVersion: pluginPlan.targetVersion,
    marketplaceAction,
    marketplaceChanged: marketplacePlan.changed,
  };
}
