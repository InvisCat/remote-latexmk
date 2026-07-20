#!/usr/bin/env node

import { execFile } from 'node:child_process';
import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';

const execFileAsync = promisify(execFile);
const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const versionPattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/;
const historicalPaths = new Set(['CHANGELOG.md']);

function parseVersion(version) {
  const match = version.match(versionPattern);
  if (!match) throw new Error(`invalid semantic version: ${version}`);
  return {
    major: Number(match[1]),
    minor: Number(match[2]),
    patch: Number(match[3]),
    prerelease: match[4] ? match[4].split('.') : [],
  };
}

function compareIdentifiers(left, right) {
  const leftNumber = /^\d+$/.test(left);
  const rightNumber = /^\d+$/.test(right);
  if (leftNumber && rightNumber) return Number(left) - Number(right);
  if (leftNumber !== rightNumber) return leftNumber ? -1 : 1;
  return left.localeCompare(right);
}

export function compareVersions(left, right) {
  const a = parseVersion(left);
  const b = parseVersion(right);
  for (const key of ['major', 'minor', 'patch']) {
    if (a[key] !== b[key]) return a[key] - b[key];
  }
  if (a.prerelease.length === 0 || b.prerelease.length === 0) {
    if (a.prerelease.length === b.prerelease.length) return 0;
    return a.prerelease.length === 0 ? 1 : -1;
  }
  const length = Math.max(a.prerelease.length, b.prerelease.length);
  for (let index = 0; index < length; index += 1) {
    if (a.prerelease[index] === undefined) return -1;
    if (b.prerelease[index] === undefined) return 1;
    const comparison = compareIdentifiers(a.prerelease[index], b.prerelease[index]);
    if (comparison !== 0) return comparison;
  }
  return 0;
}

async function rootManifest() {
  return JSON.parse(await readFile(path.join(repositoryRoot, 'package.json'), 'utf8'));
}

async function trackedFiles() {
  const { stdout } = await execFileAsync('git', ['ls-files', '-z'], {
    cwd: repositoryRoot,
    encoding: 'buffer',
    maxBuffer: 16 * 1024 * 1024,
  });
  return stdout.toString('utf8').split('\0').filter(Boolean);
}

async function readTrackedFile(target) {
  try {
    return await readFile(target);
  } catch (error) {
    if (error?.code === 'ENOENT') return null;
    throw error;
  }
}

async function replaceTrackedVersion(previousVersion, nextVersion) {
  const stale = [];
  for (const relative of await trackedFiles()) {
    if (historicalPaths.has(relative) || relative.startsWith('docs/releases/')) continue;
    const target = path.join(repositoryRoot, relative);
    const content = await readTrackedFile(target);
    if (!content) continue;
    if (content.includes(0)) continue;
    const text = content.toString('utf8');
    if (!text.includes(previousVersion)) continue;
    if (relative === 'README.md') {
      const history = text.indexOf('\n## Changelog');
      if (history < 0) throw new Error('README.md is missing the Changelog boundary');
      const active = text.slice(0, history).replaceAll(previousVersion, nextVersion);
      await writeFile(target, `${active}${text.slice(history)}`);
    } else {
      await writeFile(target, text.replaceAll(previousVersion, nextVersion));
    }
    stale.push(relative);
  }
  return stale;
}

function collectVersionReferences(relative, content) {
  const references = [];
  const patterns = [
    /remote-latexmk@([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)/g,
    /releases\/(?:download|tag)\/v([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)/g,
    /LATEXMK_GHCR_VERSION:-([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)/g,
    /remote-latexmk-(?:server(?:-full)?|client):([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)/g,
  ];
  for (const pattern of patterns) {
    for (const match of content.matchAll(pattern)) references.push({ relative, version: match[1], value: match[0] });
  }
  return references;
}

async function checkVersionSync() {
  const manifest = await rootManifest();
  const version = manifest.version;
  parseVersion(version);
  const errors = [];
  const files = await trackedFiles();

  for (const relative of files.filter((name) => name === 'package.json' || name.endsWith('/package.json'))) {
    const packageManifest = JSON.parse(await readFile(path.join(repositoryRoot, relative), 'utf8'));
    if (packageManifest.version !== version) errors.push(`${relative}: version ${packageManifest.version} != ${version}`);
  }

  for (const relative of [
    'plugins/remote-latexmk/.codex-plugin/plugin.json',
    'plugins/remote-latexmk/.claude-plugin/plugin.json',
  ]) {
    const plugin = JSON.parse(await readFile(path.join(repositoryRoot, relative), 'utf8'));
    if (plugin.version !== version) errors.push(`${relative}: version ${plugin.version} != ${version}`);
  }

  const marketplace = JSON.parse(await readFile(path.join(repositoryRoot, '.claude-plugin/marketplace.json'), 'utf8'));
  if (marketplace.metadata?.version !== version) errors.push(`.claude-plugin/marketplace.json: metadata.version != ${version}`);
  if (marketplace.plugins?.[0]?.version !== version) errors.push(`.claude-plugin/marketplace.json: plugins[0].version != ${version}`);

  for (const relative of files) {
    if (historicalPaths.has(relative) || relative.startsWith('docs/releases/')) continue;
    const content = await readTrackedFile(path.join(repositoryRoot, relative));
    if (!content) continue;
    if (content.includes(0)) continue;
    for (const reference of collectVersionReferences(relative, content.toString('utf8'))) {
      if (reference.version !== version) errors.push(`${reference.relative}: ${reference.value} != ${version}`);
    }
  }

  if (errors.length > 0) throw new Error(`release version is not synchronized:\n${errors.join('\n')}`);
  console.log(`release version ${version} is synchronized`);
}

async function setVersion(nextVersion) {
  parseVersion(nextVersion);
  const previousVersion = (await rootManifest()).version;
  if (compareVersions(nextVersion, previousVersion) <= 0) {
    throw new Error(`new version ${nextVersion} must be greater than ${previousVersion}`);
  }
  const changed = await replaceTrackedVersion(previousVersion, nextVersion);
  if (!changed.includes('package.json')) throw new Error('root package.json version was not updated');
  await execFileAsync(process.execPath, ['scripts/sync-plugin-skills.mjs'], { cwd: repositoryRoot });
  await checkVersionSync();
  console.log(`prepared release ${nextVersion} from ${previousVersion}`);
}

async function main(args) {
  const [command = 'check', value] = args;
  if (command === 'check') return checkVersionSync();
  if (command === 'set') {
    if (!value) throw new Error('usage: release-version.mjs set VERSION');
    return setVersion(value);
  }
  if (command === 'assert-newer') {
    const previous = args[2];
    if (!value || !previous) throw new Error('usage: release-version.mjs assert-newer VERSION PREVIOUS_VERSION');
    if (compareVersions(value, previous) <= 0) throw new Error(`${value} must be greater than ${previous}`);
    return;
  }
  throw new Error(`unknown command: ${command}`);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main(process.argv.slice(2)).catch((error) => {
    console.error(`release-version: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 2;
  });
}

export { checkVersionSync, parseVersion, setVersion };
