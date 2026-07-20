#!/usr/bin/env node

import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, mkdtemp, readFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';

const execFileAsync = promisify(execFile);

export function releaseTag(version) {
  return version.includes('-') ? 'next' : 'latest';
}

async function npm(args, options = {}) {
  return execFileAsync('npm', args, {
    encoding: 'utf8',
    maxBuffer: 16 * 1024 * 1024,
    ...options,
  });
}

async function remoteIntegrity(name, version) {
  try {
    const { stdout } = await npm(['view', `${name}@${version}`, 'dist.integrity', '--json']);
    return JSON.parse(stdout);
  } catch (error) {
    if (String(error.stderr).includes('E404')) return '';
    throw error;
  }
}

async function packageFingerprint(archive, files) {
  const digest = createHash('sha256');
  const entries = files
    .map(({ path: filePath, size, mode }) => ({ path: filePath, size, mode }))
    .sort((left, right) => left.path.localeCompare(right.path));
  for (const entry of entries) {
    digest.update(`${entry.path}\0${entry.size}\0${entry.mode}\0`);
    const { stdout } = await execFileAsync('tar', ['-xOf', archive, `package/${entry.path}`], {
      encoding: 'buffer',
      maxBuffer: 64 * 1024 * 1024,
    });
    digest.update(stdout);
  }
  return digest.digest('hex');
}

async function publishPackage(directory) {
  const manifest = JSON.parse(await readFile(path.join(directory, 'package.json'), 'utf8'));
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-publish-'));
  try {
    const localDirectory = path.join(temp, 'local');
    await mkdir(localDirectory);
    const { stdout } = await npm(['pack', directory, '--json', '--pack-destination', localDirectory]);
    const [packed] = JSON.parse(stdout);
    if (!packed?.filename || !packed?.integrity) throw new Error(`npm pack returned no integrity for ${manifest.name}`);
    const publishedIntegrity = await remoteIntegrity(manifest.name, manifest.version);
    if (publishedIntegrity) {
      const remoteDirectory = path.join(temp, 'remote');
      await mkdir(remoteDirectory);
      const remote = await npm([
        'pack', `${manifest.name}@${manifest.version}`,
        '--json',
        '--pack-destination', remoteDirectory,
      ]);
      const [remotePacked] = JSON.parse(remote.stdout);
      const localFingerprint = await packageFingerprint(path.join(localDirectory, packed.filename), packed.files);
      const remoteFingerprint = await packageFingerprint(path.join(remoteDirectory, remotePacked.filename), remotePacked.files);
      if (localFingerprint !== remoteFingerprint) {
        throw new Error(`${manifest.name}@${manifest.version} exists with different package contents`);
      }
      console.log(`${manifest.name}@${manifest.version} already exists with matching package contents; skipping`);
      return;
    }
    const published = await npm([
      'publish', path.join(localDirectory, packed.filename),
      '--access', 'public',
      '--provenance',
      '--tag', releaseTag(manifest.version),
    ]);
    process.stdout.write(published.stdout);
    process.stderr.write(published.stderr);
  } finally {
    await rm(temp, { recursive: true, force: true });
  }
}

async function main(args) {
  if (args.length === 0) throw new Error('usage: publish-packages.mjs PACKAGE_DIR...');
  for (const directory of args) await publishPackage(path.resolve(directory));
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main(process.argv.slice(2)).catch((error) => {
    console.error(`publish-packages: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 2;
  });
}

export { packageFingerprint, publishPackage, remoteIntegrity };
